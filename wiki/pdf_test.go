package wiki

import (
	"runtime"
	"testing"
)

func TestCleanPDFText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes excessive whitespace",
			input:    "Hello    world   test",
			expected: "Hello world test",
		},
		{
			name:     "normalizes CRLF to LF",
			input:    "Line1\r\nLine2\r\nLine3",
			expected: "Line1\nLine2\nLine3",
		},
		{
			name:     "normalizes CR to LF",
			input:    "Line1\rLine2\rLine3",
			expected: "Line1\nLine2\nLine3",
		},
		{
			name:     "removes excessive blank lines",
			input:    "Line1\n\n\n\n\nLine2",
			expected: "Line1\n\nLine2",
		},
		{
			name:     "trims surrounding whitespace",
			input:    "   Content here   ",
			expected: "Content here",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "handles only whitespace",
			input:    "   \t  \n  ",
			expected: "",
		},
		{
			name:     "combined transformations",
			input:    "  Hello    world  \r\n\r\n\r\nNext line  ",
			expected: "Hello world \n\nNext line", // trailing space before newline is preserved
		},
		{
			name:     "preserves single newlines",
			input:    "Line1\nLine2\nLine3",
			expected: "Line1\nLine2\nLine3",
		},
		{
			name:     "preserves double newlines",
			input:    "Paragraph1\n\nParagraph2",
			expected: "Paragraph1\n\nParagraph2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanPDFText(tt.input)
			if result != tt.expected {
				t.Errorf("cleanPDFText(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSearchInText(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		query      string
		pageCount  int
		wantCount  int
		checkMatch func([]FileSearchMatch) bool
	}{
		{
			name:      "finds single match",
			text:      "This is a test line with keyword here",
			query:     "keyword",
			pageCount: 1,
			wantCount: 1,
			checkMatch: func(matches []FileSearchMatch) bool {
				return len(matches) == 1 && matches[0].Line == 1
			},
		},
		{
			name:      "finds multiple matches on different lines",
			text:      "First keyword here\nNo match\nSecond keyword here",
			query:     "keyword",
			pageCount: 1,
			wantCount: 2,
		},
		{
			name:      "case insensitive search",
			text:      "KEYWORD keyword Keyword KeyWord",
			query:     "keyword",
			pageCount: 1,
			wantCount: 1, // all on same line
		},
		{
			name:      "no matches",
			text:      "This text has no matching content",
			query:     "xyz123",
			pageCount: 1,
			wantCount: 0,
		},
		{
			name:      "empty text",
			text:      "",
			query:     "keyword",
			pageCount: 1,
			wantCount: 0,
		},
		{
			name:      "empty query matches nothing meaningful",
			text:      "Some content here",
			query:     "",
			pageCount: 1,
			wantCount: 1, // empty string matches every line
		},
		{
			name:      "estimates page numbers for multi-page content",
			text:      "Line 1\nLine 2\nLine 3\nLine 4 keyword\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10",
			query:     "keyword",
			pageCount: 2,
			wantCount: 1,
			checkMatch: func(matches []FileSearchMatch) bool {
				return len(matches) == 1 && matches[0].Page >= 1
			},
		},
		{
			name:      "truncates long context",
			text:      "This is a very long line that contains the keyword somewhere in the middle of it and has lots of content before and after the keyword that needs to be truncated properly for display purposes in the search results which requires cutting off at around 80 characters on each side of the match",
			query:     "keyword",
			pageCount: 1,
			wantCount: 1,
			checkMatch: func(matches []FileSearchMatch) bool {
				return len(matches) == 1 && len(matches[0].Context) <= 250
			},
		},
		{
			name:      "respects 50 match limit",
			text:      generateRepeatedLines("keyword line", 100),
			query:     "keyword",
			pageCount: 1,
			wantCount: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := searchInText(tt.text, tt.query, tt.pageCount)
			if len(matches) != tt.wantCount {
				t.Errorf("searchInText() returned %d matches, want %d", len(matches), tt.wantCount)
			}
			if tt.checkMatch != nil && !tt.checkMatch(matches) {
				t.Errorf("searchInText() match check failed, matches: %+v", matches)
			}
		})
	}
}

func TestGetInstallHint(t *testing.T) {
	hint := getInstallHint()

	// Just verify it returns a non-empty string with platform-appropriate content
	if hint == "" {
		t.Error("getInstallHint() returned empty string")
	}

	// Check it contains expected keywords based on platform
	switch runtime.GOOS {
	case "darwin":
		if hint != "Install with: brew install poppler" {
			t.Errorf("getInstallHint() on darwin = %q, want brew install hint", hint)
		}
	case "linux":
		if hint != "Install with: apt install poppler-utils (Debian/Ubuntu) or yum install poppler-utils (RHEL/CentOS)" {
			t.Errorf("getInstallHint() on linux = %q, want apt/yum install hint", hint)
		}
	case "windows":
		if hint != "Install with: choco install poppler (or download from https://github.com/oschwartz10612/poppler-windows/releases)" {
			t.Errorf("getInstallHint() on windows = %q, want choco install hint", hint)
		}
	default:
		if hint != "Install poppler-utils for your platform" {
			t.Errorf("getInstallHint() on %s = %q, want generic install hint", runtime.GOOS, hint)
		}
	}
}

func TestSearchInText_LineNumbers(t *testing.T) {
	text := "Line 1\nLine 2 with match\nLine 3\nLine 4 with match"
	matches := searchInText(text, "match", 1)

	if len(matches) != 2 {
		t.Fatalf("Expected 2 matches, got %d", len(matches))
	}

	// Line numbers should be 1-indexed
	if matches[0].Line != 2 {
		t.Errorf("First match line = %d, want 2", matches[0].Line)
	}
	if matches[1].Line != 4 {
		t.Errorf("Second match line = %d, want 4", matches[1].Line)
	}
}

func TestSearchInText_PageEstimation(t *testing.T) {
	// Create content that spans multiple "pages"
	lines := make([]string, 100)
	for i := range lines {
		if i == 75 {
			lines[i] = "Line with keyword"
		} else {
			lines[i] = "Regular line content"
		}
	}
	text := ""
	for i, line := range lines {
		if i > 0 {
			text += "\n"
		}
		text += line
	}

	matches := searchInText(text, "keyword", 4)

	if len(matches) != 1 {
		t.Fatalf("Expected 1 match, got %d", len(matches))
	}

	// Match at line 76 (0-indexed 75) in 100 lines with 4 pages
	// Should be around page 3-4
	if matches[0].Page < 1 || matches[0].Page > 4 {
		t.Errorf("Page estimate %d not in expected range 1-4", matches[0].Page)
	}
}

// Helper function to generate repeated lines for testing
func generateRepeatedLines(line string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

func TestSearchInPDF_EmptyData(t *testing.T) {
	matches, found, message, err := SearchInPDF([]byte{}, "test")

	if err != nil {
		t.Fatalf("SearchInPDF failed: %v", err)
	}
	if found {
		t.Error("Expected found = false for empty PDF")
	}
	if message != "Empty PDF data" {
		t.Errorf("Expected 'Empty PDF data' message, got: %s", message)
	}
	if len(matches) != 0 {
		t.Errorf("Expected no matches, got %d", len(matches))
	}
}

func TestIsPdfToTextAvailable(t *testing.T) {
	// This just exercises the function - result depends on whether pdftotext is installed
	_ = isPdfToTextAvailable()
}
