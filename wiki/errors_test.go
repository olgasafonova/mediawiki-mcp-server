package wiki

import (
	"strings"
	"testing"
)

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Field:      "title",
		Value:      "test value",
		Message:    "is required",
		Suggestion: "Provide a valid title",
	}

	errStr := err.Error()

	if !strings.Contains(errStr, "title") {
		t.Error("Error should contain field name")
	}
	if !strings.Contains(errStr, "is required") {
		t.Error("Error should contain message")
	}
	if !strings.Contains(errStr, "test value") {
		t.Error("Error should contain value")
	}
	if !strings.Contains(errStr, "To fix this") {
		t.Error("Error should contain suggestion header")
	}
}

func TestValidationError_TruncatesLongValues(t *testing.T) {
	longValue := strings.Repeat("a", 200)
	err := &ValidationError{
		Field:   "content",
		Value:   longValue,
		Message: "too long",
	}

	errStr := err.Error()

	// Should truncate to ~100 chars + "..."
	if strings.Contains(errStr, longValue) {
		t.Error("Should truncate long values")
	}
	if !strings.Contains(errStr, "...") {
		t.Error("Should indicate truncation with ...")
	}
}

func TestContentTooLargeError_Error(t *testing.T) {
	err := &ContentTooLargeError{
		ContentType: "page content",
		ActualSize:  200000,
		MaxSize:     150000,
		PageTitle:   "Test Page",
	}

	errStr := err.Error()

	if !strings.Contains(errStr, "Content too large") {
		t.Error("Should indicate content is too large")
	}
	if !strings.Contains(errStr, "200000") {
		t.Error("Should show actual size")
	}
	if !strings.Contains(errStr, "150000") {
		t.Error("Should show max size")
	}
	if !strings.Contains(errStr, "Test Page") {
		t.Error("Should show page title")
	}
	if !strings.Contains(errStr, "To fix this") {
		t.Error("Should contain recovery suggestions")
	}
}

func TestAuthenticationError_Error(t *testing.T) {
	tests := []struct {
		name           string
		reason         string
		expectContains string
	}{
		{
			"Credentials error",
			"invalid credentials",
			"MEDIAWIKI_USERNAME",
		},
		{
			"Token error",
			"token expired",
			"retry with a fresh token",
		},
		{
			"Permission error",
			"permission denied",
			"bot password grants",
		},
		{
			"Generic error",
			"connection failed",
			"Check your wiki connection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &AuthenticationError{
				Operation: "edit",
				Reason:    tt.reason,
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tt.expectContains) {
				t.Errorf("Expected error to contain %q, got: %s", tt.expectContains, errStr)
			}
		})
	}
}

func TestDangerousContentError_Error(t *testing.T) {
	err := &DangerousContentError{
		ContentType: "edit to 'Test Page'",
		Pattern:     "script injection",
		Location:    "around character 100",
		Suggestion:  "Use MediaWiki's built-in features",
	}

	errStr := err.Error()

	if !strings.Contains(errStr, "dangerous content") {
		t.Error("Should indicate dangerous content")
	}
	if !strings.Contains(errStr, "script injection") {
		t.Error("Should show pattern")
	}
	if !strings.Contains(errStr, "around character 100") {
		t.Error("Should show location")
	}
}

func TestPageNotFoundError_Error(t *testing.T) {
	err := &PageNotFoundError{
		Title: "Missing Page",
	}

	errStr := err.Error()

	if !strings.Contains(errStr, "Page not found") {
		t.Error("Should indicate page not found")
	}
	if !strings.Contains(errStr, "Missing Page") {
		t.Error("Should show page title")
	}
	if !strings.Contains(errStr, "mediawiki_search") {
		t.Error("Should suggest search tool")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int
		expected string
	}{
		{500, "500 bytes"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %q, expected %q", tt.bytes, result, tt.expected)
		}
	}
}

func TestValidateWikitextContent(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectError bool
		errorType   string
	}{
		// Basic safe content
		{"Clean content", "== Heading ==\nNormal content", false, ""},
		{"Normal template", "{{Template|param=value}}", false, ""},
		{"Wikilinks", "[[Page|Link text]]", false, ""},
		{"External links", "[https://example.com Example]", false, ""},
		{"Lua invoke (warning only)", "{{#invoke:Module|function}}", false, ""},

		// Dangerous patterns OUTSIDE code blocks - should be blocked
		{"Script tag outside", "<script>alert('xss')</script>", true, "script"},
		{"HTML tag outside", "<html><body>test</body></html>", true, "HTML"},
		{"JavaScript URL outside", "[[Link|javascript:alert()]]", true, "JavaScript"},
		{"Iframe outside", "<iframe src='evil.com'></iframe>", true, "Iframe"},
		{"Object tag outside", "<object data='flash.swf'></object>", true, "Object"},
		{"Embed tag outside", "<embed src='flash.swf'></embed>", true, "Embed"},
		{"Case insensitive", "<SCRIPT>alert()</SCRIPT>", true, "script"},
		{"Mixed case", "<ScRiPt>alert()</ScRiPt>", true, "script"},

		// Code examples INSIDE safe tags - should be ALLOWED
		{"Script in syntaxhighlight", "<syntaxhighlight lang=\"javascript\"><script>alert('example')</script></syntaxhighlight>", false, ""},
		{"Script in source tag", "<source lang=\"javascript\"><script>alert('example')</script></source>", false, ""},
		{"Script in pre tag", "<pre><script>alert('example')</script></pre>", false, ""},
		{"Script in code tag", "<code><script>alert('example')</script></code>", false, ""},
		{"Script in nowiki", "<nowiki><script>alert('example')</script></nowiki>", false, ""},
		{"HTML in syntaxhighlight", "<syntaxhighlight lang=\"html\"><html><body>example</body></html></syntaxhighlight>", false, ""},
		{"JavaScript URL in code", "<code>javascript:void(0)</code>", false, ""},
		{"Iframe in pre", "<pre><iframe src=\"example.com\"></iframe></pre>", false, ""},

		// Code with attributes in tags
		{"Syntaxhighlight with attrs", "<syntaxhighlight lang=\"xml\" line=\"1\"><script>test</script></syntaxhighlight>", false, ""},

		// Mixed content - safe in code block, dangerous outside
		{"Mixed safe and dangerous", "<pre><script>safe</script></pre>\n<script>dangerous</script>", true, "script"},

		// JSON/XML examples (common in technical docs)
		{"JSON example", "<syntaxhighlight lang=\"json\">{\"script\": \"value\"}</syntaxhighlight>", false, ""},
		{"XML example", "<source lang=\"xml\"><html><head></head></html></source>", false, ""},
		{"Shell example", "<pre>curl javascript:something</pre>", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWikitextContent(tt.content, "Test Page")
			if tt.expectError && err == nil {
				t.Errorf("Expected error for %q", tt.name)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for %q: %v", tt.name, err)
			}
			if tt.expectError && err != nil {
				if _, ok := err.(*DangerousContentError); !ok {
					t.Errorf("Expected DangerousContentError, got %T", err)
				}
			}
		})
	}
}

func TestStripSafeCodeBlocks(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"No code blocks", "normal text", "normal text"},
		{"Simple syntaxhighlight", "before<syntaxhighlight>code</syntaxhighlight>after", "beforeafter"},
		{"Syntaxhighlight with lang", "before<syntaxhighlight lang=\"js\">code</syntaxhighlight>after", "beforeafter"},
		{"Pre tag", "text<pre>preformatted</pre>more", "textmore"},
		{"Code tag", "inline <code>code</code> here", "inline  here"},
		{"Nowiki tag", "text<nowiki>raw</nowiki>end", "textend"},
		{"Multiple blocks", "<pre>a</pre>middle<code>b</code>end", "middleend"},
		{"Nested text preserved", "keep<syntaxhighlight>remove</syntaxhighlight>this", "keepthis"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripSafeCodeBlocks(tt.input)
			if result != tt.expected {
				t.Errorf("stripSafeCodeBlocks(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateContentSize(t *testing.T) {
	tests := []struct {
		name        string
		contentSize int
		maxSize     int
		expectError bool
	}{
		{"Under limit", 1000, 2000, false},
		{"At limit", 2000, 2000, false},
		{"Over limit", 3000, 2000, true},
		{"Way over limit", 100000, 2000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := strings.Repeat("a", tt.contentSize)
			err := ValidateContentSize(content, "Test Page", tt.maxSize)
			if tt.expectError && err == nil {
				t.Error("Expected error")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tt.expectError && err != nil {
				if _, ok := err.(*ContentTooLargeError); !ok {
					t.Errorf("Expected ContentTooLargeError, got %T", err)
				}
			}
		})
	}
}

func TestConfigError_Error(t *testing.T) {
	err := &ConfigError{
		Field:      "MEDIAWIKI_URL",
		Message:    "is required",
		Suggestion: "Set the environment variable",
	}

	errStr := err.Error()

	if !strings.Contains(errStr, "MEDIAWIKI_URL") {
		t.Error("Should contain field name")
	}
	if !strings.Contains(errStr, "is required") {
		t.Error("Should contain message")
	}
	if !strings.Contains(errStr, "To fix this") {
		t.Error("Should contain suggestion header")
	}
}

func TestValidateWikiURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
		errorMsg    string
	}{
		{"Valid HTTPS", "https://wiki.example.com/api.php", false, ""},
		{"Valid HTTPS with path", "https://wiki.example.com/w/api.php", false, ""},
		{"HTTP rejected", "http://wiki.example.com/api.php", true, "HTTPS"},
		{"Missing api.php", "https://wiki.example.com/", true, "api.php"},
		{"Invalid URL no scheme", "not-a-url", true, "HTTPS"}, // Go parses with empty scheme
		{"Empty scheme", "://wiki.example.com/api.php", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWikiURL(tt.url)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for URL %q", tt.url)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for URL %q: %v", tt.url, err)
			}
			if tt.expectError && err != nil && tt.errorMsg != "" {
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain %q, got: %v", tt.errorMsg, err)
				}
			}
		})
	}
}
