package wiki

import (
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Error codes for programmatic error handling
type ErrorCode string

const (
	// SSRF error codes
	SSRFCodePrivateIP    ErrorCode = "SSRF_PRIVATE_IP"
	SSRFCodeDNSError     ErrorCode = "SSRF_DNS_ERROR"
	SSRFCodeRedirect     ErrorCode = "SSRF_REDIRECT_BLOCKED"
	SSRFCodeInvalidURL   ErrorCode = "SSRF_INVALID_URL"

	// Rate limit error codes
	RateLimitCodeExceeded ErrorCode = "RATE_LIMIT_EXCEEDED"
	RateLimitCodeBlocked  ErrorCode = "RATE_LIMIT_BLOCKED"

	// Validation error codes
	ValidationCodeInvalid   ErrorCode = "VALIDATION_INVALID"
	ValidationCodeTooLarge  ErrorCode = "VALIDATION_TOO_LARGE"
	ValidationCodeDangerous ErrorCode = "VALIDATION_DANGEROUS"

	// Authentication error codes
	AuthCodeInvalidCredentials ErrorCode = "AUTH_INVALID_CREDENTIALS"
	AuthCodeTokenExpired       ErrorCode = "AUTH_TOKEN_EXPIRED"
	AuthCodePermissionDenied   ErrorCode = "AUTH_PERMISSION_DENIED"

	// Not found error codes
	NotFoundCodePage     ErrorCode = "NOT_FOUND_PAGE"
	NotFoundCodeCategory ErrorCode = "NOT_FOUND_CATEGORY"
)

// SSRFError represents a blocked SSRF attempt with structured error code
type SSRFError struct {
	Code    ErrorCode
	URL     string
	Reason  string
	Blocked bool
	IP      string // The IP that was blocked (if applicable)
}

func (e *SSRFError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] SSRF protection blocked request", e.Code))

	if e.URL != "" {
		sb.WriteString(fmt.Sprintf("\nURL: %s", e.URL))
	}
	if e.IP != "" {
		sb.WriteString(fmt.Sprintf("\nBlocked IP: %s", e.IP))
	}
	sb.WriteString(fmt.Sprintf("\nReason: %s", e.Reason))

	sb.WriteString("\n\nThis is a security protection against Server-Side Request Forgery (SSRF).")
	sb.WriteString("\nInternal/private network addresses are not allowed for external link checking.")

	return sb.String()
}

// ErrorCode returns the structured error code for programmatic handling
func (e *SSRFError) ErrorCode() ErrorCode {
	return e.Code
}

// ValidationError represents a content or input validation failure with recovery guidance
type ValidationError struct {
	Field      string
	Value      string
	Message    string
	Suggestion string
}

func (e *ValidationError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Validation failed for %s: %s", e.Field, e.Message))
	if e.Value != "" {
		// Truncate long values for display
		displayValue := e.Value
		if len(displayValue) > 100 {
			displayValue = displayValue[:100] + "..."
		}
		sb.WriteString(fmt.Sprintf("\n\nProvided value: %q", displayValue))
	}
	if e.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("\n\nTo fix this:\n%s", e.Suggestion))
	}
	return sb.String()
}

// ContentTooLargeError indicates content exceeds size limits
type ContentTooLargeError struct {
	ContentType string // "page content", "edit content", etc.
	ActualSize  int
	MaxSize     int
	PageTitle   string
}

func (e *ContentTooLargeError) Error() string {
	return fmt.Sprintf(`Content too large: %s is %d bytes (max: %d bytes)

Page: %s
Size: %s / %s

To fix this:
1. If reading: The content will be automatically truncated to %d bytes.
   You can request specific sections instead of the full page.

2. If editing: Split your content into multiple smaller pages, or
   edit specific sections using the 'section' parameter.

3. For very large pages, consider:
   - Breaking into sub-pages (e.g., "Page/Section1", "Page/Section2")
   - Using transclusion to include content from other pages
   - Moving large data tables to separate pages`,
		e.ContentType,
		e.ActualSize,
		e.MaxSize,
		e.PageTitle,
		formatBytes(e.ActualSize),
		formatBytes(e.MaxSize),
		e.MaxSize,
	)
}

// AuthenticationError indicates authentication failures with recovery steps
type AuthenticationError struct {
	Operation  string
	Reason     string
	StatusCode int
}

func (e *AuthenticationError) Error() string {
	var suggestion string
	switch {
	case strings.Contains(e.Reason, "credentials"):
		suggestion = `Check your credentials:
1. Verify MEDIAWIKI_USERNAME is in format "YourUser@BotName"
2. Verify MEDIAWIKI_PASSWORD is the bot password (not your user password)
3. Create a bot password at Special:BotPasswords on your wiki`

	case strings.Contains(e.Reason, "token"):
		suggestion = `Token error - this usually resolves automatically.
If persistent:
1. The server will retry with a fresh token
2. Check if your wiki session has expired
3. Verify your bot password hasn't been revoked`

	case strings.Contains(e.Reason, "permission"):
		suggestion = `Your bot account lacks required permissions.
To fix:
1. Go to Special:BotPasswords on your wiki
2. Edit your bot password grants
3. Ensure 'Edit existing pages' and 'Create pages' are enabled`

	default:
		suggestion = `Check your wiki connection and credentials.
1. Verify MEDIAWIKI_URL points to a valid wiki API
2. Test the URL in a browser: <URL>?action=query&meta=siteinfo&format=json
3. Check if the wiki requires authentication for reading`
	}

	return fmt.Sprintf(`Authentication failed for %s: %s

%s`, e.Operation, e.Reason, suggestion)
}

// RateLimitError indicates rate limiting with retry guidance
type RateLimitError struct {
	Operation   string
	RetryAfter  int // seconds
	Limit       int
	Current     int
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf(`Rate limit exceeded for %s

Current: %d requests
Limit:   %d concurrent requests
Retry:   Wait %d seconds before retrying

The server automatically queues requests and retries with exponential backoff.
If you see this error frequently:
1. Reduce the number of concurrent operations
2. Add delays between batch operations
3. Use batch endpoints (e.g., GetExternalLinksBatch) instead of individual calls`,
		e.Operation,
		e.Current,
		e.Limit,
		e.RetryAfter,
	)
}

// DangerousContentError indicates potentially harmful content was blocked
type DangerousContentError struct {
	ContentType string
	Pattern     string
	Location    string
	Suggestion  string
}

func (e *DangerousContentError) Error() string {
	return fmt.Sprintf(`Potentially dangerous content detected in %s

Pattern found: %s
Location: %s

This content was blocked for security reasons.

%s`, e.ContentType, e.Pattern, e.Location, e.Suggestion)
}

// PageNotFoundError provides helpful suggestions for missing pages
type PageNotFoundError struct {
	Title      string
	Suggestion string
}

func (e *PageNotFoundError) Error() string {
	suggestion := e.Suggestion
	if suggestion == "" {
		suggestion = fmt.Sprintf(`The page "%s" does not exist.

Possible causes:
1. The page title is misspelled
2. The page was deleted or moved
3. The page exists in a different namespace

To find the correct page:
1. Use mediawiki_search to search for similar titles
2. Check Special:AllPages on your wiki
3. Verify the exact title including capitalization`, e.Title)
	}
	return fmt.Sprintf(`Page not found: %s

%s`, e.Title, suggestion)
}

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
