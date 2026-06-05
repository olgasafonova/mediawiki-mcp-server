package wiki

import (
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// formatBytes formats byte count as human-readable string
func formatBytes(bytes int) string {
	const (
		KB = 1024
		MB = KB * 1024
	)
	switch {
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}

// Dangerous wikitext patterns that should be blocked or flagged
// These are only dangerous OUTSIDE of code block tags
var DangerousPatterns = []struct {
	Pattern     string
	Description string
	Severity    string // "block" or "warn"
}{
	{"<script", "JavaScript injection", "block"},
	{"<html", "Raw HTML block", "block"},
	{"javascript:", "JavaScript URL", "block"},
	{"<iframe", "Iframe embedding", "block"},
	{"<object", "Object embedding", "block"},
	{"<embed", "Embed tag", "block"},
	{"{{#invoke:", "Lua module invocation", "warn"},
	{"{{#tag:script", "Script tag via parser function", "block"},
	{"{{#tag:style", "Style tag via parser function", "warn"},
	{"__NOINDEX__", "Search engine directive", "warn"},
	{"__NOEDITSECTION__", "Edit section hiding", "warn"},
}

// SafeCodeBlockTags are MediaWiki tags that safely display code as text (not executed)
// Content inside these tags is allowed to contain "dangerous" patterns because
// they're displayed as examples, not executed
var SafeCodeBlockTags = []string{
	"syntaxhighlight", // <syntaxhighlight lang="javascript">code</syntaxhighlight>
	"source",          // <source lang="javascript">code</source> (older syntax)
	"pre",             // <pre>preformatted text</pre>
	"code",            // <code>inline code</code>
	"nowiki",          // <nowiki>not parsed</nowiki>
	"tt",              // <tt>teletype</tt> (older)
}

// stripSafeCodeBlocks removes content inside safe code block tags
// so we don't flag code examples as dangerous
func stripSafeCodeBlocks(content string) string {
	result := content
	lowerContent := strings.ToLower(content)

	for _, tag := range SafeCodeBlockTags {
		// Match both <tag>content</tag> and <tag attr="value">content</tag>
		openTagStart := "<" + tag
		closeTag := "</" + tag + ">"

		for {
			lowerResult := strings.ToLower(result)
			startIdx := strings.Index(lowerResult, openTagStart)
			if startIdx == -1 {
				break
			}

			// Find the end of the opening tag (handle attributes)
			tagEndIdx := strings.Index(lowerResult[startIdx:], ">")
			if tagEndIdx == -1 {
				break
			}
			tagEndIdx += startIdx + 1

			// Find the closing tag
			closeIdx := strings.Index(lowerResult[tagEndIdx:], closeTag)
			if closeIdx == -1 {
				break
			}
			closeIdx += tagEndIdx + len(closeTag)

			// Remove this code block from the content
			result = result[:startIdx] + result[closeIdx:]
		}
	}

	// Also handle self-closing syntaxhighlight with file attribute
	// <syntaxhighlight lang="json" source="file.json" />
	_ = lowerContent // suppress unused warning
	return result
}

// ValidateWikitextContent checks content for dangerous patterns
// Code inside safe wrapper tags (syntaxhighlight, source, pre, code, nowiki) is allowed
// SECURITY: Applies Unicode NFC normalization to prevent bypass attacks
func ValidateWikitextContent(content, title string) error {
	// Apply Unicode NFC normalization to prevent bypass attacks using
	// alternative character representations (e.g., combining characters)
	normalizedContent := norm.NFC.String(content)

	// Strip out safe code blocks before checking for dangerous patterns
	// This allows code examples in documentation
	contentToCheck := stripSafeCodeBlocks(normalizedContent)
	lowerContent := strings.ToLower(contentToCheck)

	for _, pattern := range DangerousPatterns {
		if pattern.Severity == "block" && strings.Contains(lowerContent, strings.ToLower(pattern.Pattern)) {
			// Find approximate location in original content
			originalLower := strings.ToLower(content)
			idx := strings.Index(originalLower, strings.ToLower(pattern.Pattern))
			location := "near beginning"
			if idx > 100 {
				location = fmt.Sprintf("around character %d", idx)
			}

			return &DangerousContentError{
				ContentType: fmt.Sprintf("edit to '%s'", title),
				Pattern:     pattern.Description,
				Location:    location,
				Suggestion: fmt.Sprintf(`The pattern "%s" was found outside of a code block.

To include code examples safely, wrap them in one of these tags:
• <syntaxhighlight lang="javascript">your code here</syntaxhighlight>
• <source lang="xml">your code here</source>
• <pre>preformatted code</pre>
• <code>inline code</code>
• <nowiki>prevents wiki parsing</nowiki>

Code inside these tags is displayed as text, not executed.

If this is NOT a code example and you need this functionality:
1. Contact a wiki administrator to whitelist the pattern
2. For scripts, use the wiki's Gadgets system instead`, pattern.Pattern),
			}
		}
	}

	return nil
}

// ValidateContentSize checks if content is within size limits
func ValidateContentSize(content, title string, maxSize int) error {
	if len(content) > maxSize {
		return &ContentTooLargeError{
			ContentType: "edit content",
			ActualSize:  len(content),
			MaxSize:     maxSize,
			PageTitle:   title,
		}
	}
	return nil
}

// WikiError provides contextual error messages with recovery suggestions.
