package wiki

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

func truncateContent(content string, limit int) (string, bool) {
	if len(content) <= limit {
		return content, false
	}

	truncationMsg := fmt.Sprintf(`

---
[CONTENT TRUNCATED]
Showing: %d of %d characters (%.1f%% of full content)

To get the full content:
1. Request specific sections using the 'section' parameter
2. Use mediawiki_get_page_info to check the full page size first
3. For very large pages, consider fetching in chunks`,
		limit, len(content), float64(limit)/float64(len(content))*100)

	return content[:limit] + truncationMsg, true
}

// normalizeLimit ensures limit is within bounds
func normalizeLimit(limit, defaultVal, maxVal int) int {
	if limit <= 0 {
		return defaultVal
	}
	if limit > maxVal {
		return maxVal
	}
	return limit
}

// normalizeCategoryName ensures category name has proper prefix
func normalizeCategoryName(name string) string {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "Category:") {
		name = "Category:" + name
	}
	return name
}

// NormalizeUnicode applies NFC normalization to a string
// This prevents Unicode-based bypass attacks where attackers use alternative
// representations of characters (e.g., combining characters, different encodings)
// to evade security checks. NFC (Canonical Decomposition, followed by Canonical Composition)
// is the standard normalization form used by MediaWiki.
func NormalizeUnicode(s string) string {
	return norm.NFC.String(s)
}

// normalizePageTitle normalizes a page title to MediaWiki conventions:
// - Applies Unicode NFC normalization
// - Trims whitespace
// - Replaces underscores with spaces
// - Capitalizes the first letter of the title and namespace prefix
// This helps handle case variations like "Module overview" vs "Module Overview"
func normalizePageTitle(title string) string {
	// Apply Unicode normalization first to prevent bypass attacks
	title = NormalizeUnicode(title)
	title = strings.TrimSpace(title)
	if title == "" {
		return title
	}

	// Replace underscores with spaces (MediaWiki convention)
	title = strings.ReplaceAll(title, "_", " ")

	// Collapse multiple spaces
	for strings.Contains(title, "  ") {
		title = strings.ReplaceAll(title, "  ", " ")
	}

	// Capitalize first letter (MediaWiki default behavior)
	if colonIdx := strings.Index(title, ":"); colonIdx > 0 {
		// Has namespace prefix - capitalize both the prefix and the page title
		prefix := title[:colonIdx]
		rest := title[colonIdx+1:]

		// Capitalize the namespace prefix
		prefix = strings.ToUpper(string(prefix[0])) + prefix[1:]

		// Capitalize the first letter after the colon
		if len(rest) > 0 {
			rest = strings.ToUpper(string(rest[0])) + rest[1:]
		}
		return prefix + ":" + rest
	}

	// No namespace prefix - capitalize first letter
	return strings.ToUpper(string(title[0])) + title[1:]
}

// HTML sanitization patterns for XSS prevention - optimized with combined regexes
// Note: Go's regexp doesn't support backreferences, so we use separate patterns for each tag
var (
	// Patterns for dangerous tags with content (separate patterns since Go doesn't support backrefs)
	scriptTagRegex = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleTagRegex  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	iframeTagRegex = regexp.MustCompile(`(?is)<iframe[^>]*>.*?</iframe>`)
	objectTagRegex = regexp.MustCompile(`(?is)<object[^>]*>.*?</object>`)
	embedTagRegex  = regexp.MustCompile(`(?is)<embed[^>]*>.*?</embed>`)
	appletTagRegex = regexp.MustCompile(`(?is)<applet[^>]*>.*?</applet>`)
	formTagRegex   = regexp.MustCompile(`(?is)<form[^>]*>.*?</form>`)

	// Combined pattern for self-closing dangerous tags (single pass for 3 tag types)
	dangerousSelfClosingTagsRegex = regexp.MustCompile(`(?is)<(?:meta|link|base)[^>]*>`)

	// Remove event handler attributes (onclick, onerror, onload, etc.)
	eventHandlerRegex = regexp.MustCompile(`(?i)\s+on\w+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]*)`)

	// Combined pattern for dangerous URL schemes (javascript: and data:)
	dangerousURLRegex = regexp.MustCompile(`(?i)(href|src|action)\s*=\s*["']?\s*(?:javascript|data):[^"'>\s]*["']?`)

	// Remove style attributes that could contain expressions
	styleAttrRegex = regexp.MustCompile(`(?i)\s+style\s*=\s*(?:"[^"]*"|'[^']*')`)

	// Remove HTML comments (including MediaWiki cache comments like <!-- NewPP limit report -->)
	htmlCommentRegex = regexp.MustCompile(`(?s)<!--.*?-->`)
)

// sanitizeHTML removes potentially dangerous HTML elements and attributes
// to prevent XSS attacks when HTML content is displayed by clients
func sanitizeHTML(html string) string {
	// Remove dangerous tags with content
	html = scriptTagRegex.ReplaceAllString(html, "")
	html = styleTagRegex.ReplaceAllString(html, "")
	html = iframeTagRegex.ReplaceAllString(html, "")
	html = objectTagRegex.ReplaceAllString(html, "")
	html = embedTagRegex.ReplaceAllString(html, "")
	html = appletTagRegex.ReplaceAllString(html, "")
	html = formTagRegex.ReplaceAllString(html, "")

	// Remove self-closing dangerous tags (meta, link, base)
	html = dangerousSelfClosingTagsRegex.ReplaceAllString(html, "")

	// Remove event handlers
	html = eventHandlerRegex.ReplaceAllString(html, "")

	// Remove dangerous URL schemes (javascript:, data:)
	html = dangerousURLRegex.ReplaceAllString(html, "$1=\"\"")

	// Remove style attributes (can contain CSS expressions)
	html = styleAttrRegex.ReplaceAllString(html, "")

	// Remove HTML comments (reduces token usage by removing MediaWiki cache comments)
	html = htmlCommentRegex.ReplaceAllString(html, "")

	return html
}

// =============================================================================
// Safe Type Assertion Helpers for API Response Parsing
// =============================================================================

// getMap safely extracts a map from an interface, returning nil if not a map
func getMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}

// getString safely extracts a string from an interface, returning empty string if not a string
func getString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// getFloat64 safely extracts a float64 from an interface, returning 0 if not a float64
func getFloat64(v interface{}) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

// getInt safely extracts an int from a float64 interface (JSON numbers are float64)
func getInt(v interface{}) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

// getBool safely extracts a bool from an interface, returning false if not a bool
func getBool(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// getSlice safely extracts a slice from an interface, returning nil if not a slice
func getSlice(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok {
		return s
	}
	return nil
}

// getNestedMap safely navigates nested maps using a path of keys
// Returns nil if any key is missing or value is not a map
func getNestedMap(data map[string]interface{}, keys ...string) map[string]interface{} {
	current := data
	for _, key := range keys {
		if current == nil {
			return nil
		}
		next, ok := current[key].(map[string]interface{})
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

// getNestedString safely extracts a string from a nested path
func getNestedString(data map[string]interface{}, keys ...string) string {
	if len(keys) == 0 {
		return ""
	}
	if len(keys) == 1 {
		return getString(data[keys[0]])
	}
	nested := getNestedMap(data, keys[:len(keys)-1]...)
	if nested == nil {
		return ""
	}
	return getString(nested[keys[len(keys)-1]])
}
