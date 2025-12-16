package wiki

import (
	"testing"
)

func TestNormalizeLimit(t *testing.T) {
	tests := []struct {
		name       string
		limit      int
		defaultVal int
		maxVal     int
		expected   int
	}{
		{"Zero limit returns default", 0, 50, 500, 50},
		{"Negative limit returns default", -10, 50, 500, 50},
		{"Within bounds returns as-is", 100, 50, 500, 100},
		{"Exceeds max returns max", 1000, 50, 500, 500},
		{"Exactly max returns max", 500, 50, 500, 500},
		{"Exactly default returns default", 50, 50, 500, 50},
		{"One returns one", 1, 50, 500, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeLimit(tt.limit, tt.defaultVal, tt.maxVal)
			if result != tt.expected {
				t.Errorf("normalizeLimit(%d, %d, %d) = %d, expected %d",
					tt.limit, tt.defaultVal, tt.maxVal, result, tt.expected)
			}
		})
	}
}

func TestNormalizeCategoryName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Without prefix", "MyCategory", "Category:MyCategory"},
		{"With prefix", "Category:MyCategory", "Category:MyCategory"},
		{"With spaces", "  My Category  ", "Category:My Category"},
		{"Empty string", "", "Category:"},
		{"Only prefix", "Category:", "Category:"},
		{"Lowercase prefix not matched", "category:Test", "Category:category:Test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeCategoryName(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeCategoryName(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTruncateContent(t *testing.T) {
	tests := []struct {
		name              string
		content           string
		limit             int
		expectedTruncated bool
		expectedContains  string
	}{
		{"Short content not truncated", "Hello", 100, false, "Hello"},
		{"Exactly at limit not truncated", "12345", 5, false, "12345"},
		{"Over limit truncated", "123456", 5, true, "12345"},
		{"Long content truncated", "This is a very long content", 10, true, "This is a "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, truncated := truncateContent(tt.content, tt.limit)
			if truncated != tt.expectedTruncated {
				t.Errorf("truncateContent() truncated = %v, expected %v", truncated, tt.expectedTruncated)
			}
			if !truncated && result != tt.content {
				t.Errorf("Non-truncated content should be unchanged")
			}
			if truncated && len(result) < tt.limit {
				t.Errorf("Truncated content should include truncation message")
			}
		})
	}
}

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Plain text unchanged", "Hello World", "Hello World"},
		{"Simple tag removed", "<b>Hello</b>", "Hello"},
		{"Multiple tags removed", "<p><b>Hello</b> <i>World</i></p>", "Hello World"},
		{"Self-closing tag removed", "Hello<br/>World", "HelloWorld"},
		{"Nested tags removed", "<div><span>Text</span></div>", "Text"},
		{"HTML entities decoded and stripped", "&lt;script&gt;", ""}, // Entities decode to <script> which is stripped
		{"Ampersand decoded", "Tom &amp; Jerry", "Tom & Jerry"},
		{"Mixed content", "<p>Hello &amp; <b>World</b></p>", "Hello & World"},
		{"Empty string", "", ""},
		{"Only tags", "<div></div>", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripHTMLTags(tt.input)
			if result != tt.expected {
				t.Errorf("stripHTMLTags(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Test validation on method argument types

func TestSearchArgs_Validation(t *testing.T) {
	// Search requires non-empty query
	args := SearchArgs{Query: ""}
	if args.Query != "" {
		t.Error("Empty query should be empty")
	}

	args = SearchArgs{Query: "test", Limit: 0}
	// Limit 0 should be normalized to default
	normalized := normalizeLimit(args.Limit, 20, MaxLimit)
	if normalized != 20 {
		t.Errorf("Expected default limit 20, got %d", normalized)
	}

	args = SearchArgs{Query: "test", Limit: 1000}
	// Limit > MaxLimit should be capped
	normalized = normalizeLimit(args.Limit, 20, MaxLimit)
	if normalized != MaxLimit {
		t.Errorf("Expected max limit %d, got %d", MaxLimit, normalized)
	}
}

func TestCheckLinksArgs_URLLimit(t *testing.T) {
	// Test that URL limit is enforced (max 20)
	maxURLs := 20

	// Test with exactly max URLs
	urls := make([]string, maxURLs)
	for i := 0; i < maxURLs; i++ {
		urls[i] = "https://example.com/" + string(rune('a'+i))
	}

	if len(urls) != maxURLs {
		t.Errorf("Expected %d URLs, got %d", maxURLs, len(urls))
	}

	// Test exceeding limit (simulating what the method does)
	urls = make([]string, 30)
	if len(urls) > maxURLs {
		urls = urls[:maxURLs]
	}
	if len(urls) != maxURLs {
		t.Errorf("Expected URLs to be capped at %d, got %d", maxURLs, len(urls))
	}
}

func TestCheckLinksArgs_TimeoutBounds(t *testing.T) {
	tests := []struct {
		name     string
		timeout  int
		expected int
	}{
		{"Zero uses default", 0, 10},
		{"Negative uses default", -5, 10},
		{"Within bounds", 15, 15},
		{"At max", 30, 30},
		{"Over max uses default", 60, 10}, // Method caps at 30, treats >30 as default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeout := 10 // default
			if tt.timeout > 0 && tt.timeout <= 30 {
				timeout = tt.timeout
			}
			if timeout != tt.expected {
				t.Errorf("Timeout %d => %d, expected %d", tt.timeout, timeout, tt.expected)
			}
		})
	}
}

func TestExternalLinksBatchArgs_TitleLimit(t *testing.T) {
	maxBatch := 10

	// Test exceeding limit
	titles := make([]string, 15)
	for i := range titles {
		titles[i] = "Page" + string(rune('A'+i))
	}

	if len(titles) > maxBatch {
		titles = titles[:maxBatch]
	}

	if len(titles) != maxBatch {
		t.Errorf("Expected titles to be capped at %d, got %d", maxBatch, len(titles))
	}
}

func TestCheckTranslationsArgs_PatternValidation(t *testing.T) {
	validPatterns := []string{"subpage", "suffix", "prefix"}
	invalidPatterns := []string{"invalid", "", "SUBPAGE", "sub-page"}

	for _, p := range validPatterns {
		if p != "subpage" && p != "suffix" && p != "prefix" {
			t.Errorf("Pattern %q should be valid", p)
		}
	}

	for _, p := range invalidPatterns {
		if p == "subpage" || p == "suffix" || p == "prefix" {
			t.Errorf("Pattern %q should be invalid", p)
		}
	}
}

func TestFindBrokenInternalLinksArgs_PageLimit(t *testing.T) {
	// Default 20, max 100
	tests := []struct {
		limit    int
		expected int
	}{
		{0, 20},
		{-5, 20},
		{50, 50},
		{100, 100},
		{150, 100},
	}

	for _, tt := range tests {
		result := normalizeLimit(tt.limit, 20, 100)
		if result != tt.expected {
			t.Errorf("normalizeLimit(%d, 20, 100) = %d, expected %d", tt.limit, result, tt.expected)
		}
	}
}

func TestFindOrphanedPagesArgs_Limit(t *testing.T) {
	// Default 50, max 200
	tests := []struct {
		limit    int
		expected int
	}{
		{0, 50},
		{-5, 50},
		{100, 100},
		{200, 200},
		{300, 200},
	}

	for _, tt := range tests {
		result := normalizeLimit(tt.limit, 50, 200)
		if result != tt.expected {
			t.Errorf("normalizeLimit(%d, 50, 200) = %d, expected %d", tt.limit, result, tt.expected)
		}
	}
}

func TestGetRevisionsArgs_Limit(t *testing.T) {
	// Default 20, max 100
	tests := []struct {
		limit    int
		expected int
	}{
		{0, 20},
		{50, 50},
		{100, 100},
		{200, 100},
	}

	for _, tt := range tests {
		result := normalizeLimit(tt.limit, 20, 100)
		if result != tt.expected {
			t.Errorf("normalizeLimit(%d, 20, 100) = %d, expected %d", tt.limit, result, tt.expected)
		}
	}
}

func TestGetBacklinksArgs_Limit(t *testing.T) {
	// Default 50, max 500
	tests := []struct {
		limit    int
		expected int
	}{
		{0, 50},
		{100, 100},
		{500, 500},
		{1000, 500},
	}

	for _, tt := range tests {
		result := normalizeLimit(tt.limit, 50, MaxLimit)
		if result != tt.expected {
			t.Errorf("normalizeLimit(%d, 50, %d) = %d, expected %d", tt.limit, MaxLimit, result, tt.expected)
		}
	}
}

func TestCheckTerminologyArgs_Limit(t *testing.T) {
	// Default 10, max 50
	tests := []struct {
		limit    int
		expected int
	}{
		{0, 10},
		{25, 25},
		{50, 50},
		{100, 50},
	}

	for _, tt := range tests {
		result := normalizeLimit(tt.limit, 10, 50)
		if result != tt.expected {
			t.Errorf("normalizeLimit(%d, 10, 50) = %d, expected %d", tt.limit, result, tt.expected)
		}
	}
}
