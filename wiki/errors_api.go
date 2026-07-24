package wiki

import "fmt"

// apiErrorAdvice holds recovery guidance for a known MediaWiki API error code.
type apiErrorAdvice struct {
	suggestion   string
	alternatives []string
}

// knownAPIErrorAdvice maps MediaWiki API error codes to recovery guidance.
var knownAPIErrorAdvice = map[string]apiErrorAdvice{
	"nosuchpageid": {
		suggestion:   "Use mediawiki_resolve_title to find the correct page title",
		alternatives: []string{"mediawiki_search", "mediawiki_list_pages"},
	},
	"missingtitle": {
		suggestion:   "Use mediawiki_resolve_title to find the correct page title",
		alternatives: []string{"mediawiki_search", "mediawiki_list_pages"},
	},
	"protectedpage": {
		suggestion: "This page is protected. Contact an administrator.",
	},
	"blocked": {
		suggestion: "Your account or IP is blocked. Contact an administrator.",
	},
	"ratelimited": {
		suggestion:   "Too many requests. Wait and retry with fewer requests.",
		alternatives: []string{"Use batch operations", "Add delays between requests"},
	},
	"readonly": {
		suggestion: "The wiki is in read-only mode. Try again later.",
	},
	"badtoken": {
		suggestion: "Session expired. The system will automatically refresh credentials.",
	},
	"assertuserfailed": {
		suggestion: "Login session expired. Re-authentication will be attempted automatically.",
	},
	"editconflict": {
		suggestion:   "Another edit was made while you were editing. Fetch the latest version and reapply changes.",
		alternatives: []string{"mediawiki_get_page - get current content", "mediawiki_get_revisions - see recent edits"},
	},
	"spamblacklist": {
		suggestion: "Content contains blocked URLs. Remove external links and try again.",
	},
	"abusefilter-disallowed": {
		suggestion: "Content was blocked by an abuse filter. Review the content for policy violations.",
	},
}

// WrapAPIError wraps a MediaWiki API error with helpful context
func WrapAPIError(code, info, operation string) *WikiError {
	err := &WikiError{
		Code:    code,
		Message: info,
	}

	// Add context based on known error codes
	advice, known := knownAPIErrorAdvice[code]
	if !known {
		err.Suggestion = fmt.Sprintf("Check MediaWiki API documentation for error code '%s'", code)
		return err
	}
	err.Suggestion = advice.suggestion
	err.Alternatives = advice.alternatives
	return err
}

// APIError represents a non-OK HTTP response from the wiki API.
//
// SECURITY (HG-2): the raw response body is never embedded in the error
// message. The body may contain MediaWiki HTML error pages, MITM proxy
// responses, or echoed POST parameters (worst-case lgpassword on the
// login leg if a misconfigured proxy echoes request bodies). The Error()
// method exposes only the HTTP status, code, and a stable status-text
// message; the body is preserved on the struct via BodySnippet (capped
// to APIErrorBodyMax bytes) for diagnostic logging by the SERVER, never
// surfaced to MCP callers.
type APIError struct {
	StatusCode int
	Code       ErrorCode
	// BodySnippet is the truncated, server-only body capture for logging.
	// It is NOT included in Error() output. Length capped at APIErrorBodyMax.
	BodySnippet string
}

// APIErrorBodyMax caps the body snippet retained on APIError for server-side
// logging. Beyond this we trade diagnostic depth for risk of leaking sensitive
// state into log files (some operators ship logs to remote sinks).
const APIErrorBodyMax = 256

func (e *APIError) Error() string {
	// Use http.StatusText via the standard library convention. We import
	// net/http where APIError is constructed (in client.go) so the call
	// site passes a stable description; here we just format what we have.
	return fmt.Sprintf("wiki API error: HTTP %d (%s)", e.StatusCode, e.Code)
}

func (e *APIError) ErrorCode() ErrorCode {
	return e.Code
}

// NewAPIError builds an APIError, classifying the status code into a stable
// ErrorCode and capturing a truncated body snippet for server-side logging.
// statusText should be the http.StatusText(statusCode) value; it is included
// in the human-readable error message but the body is NOT.
func NewAPIError(statusCode int, body []byte) *APIError {
	code := classifyHTTPStatus(statusCode)

	snippet := body
	if len(snippet) > APIErrorBodyMax {
		snippet = snippet[:APIErrorBodyMax]
	}

	return &APIError{
		StatusCode:  statusCode,
		Code:        code,
		BodySnippet: string(snippet),
	}
}

// classifyHTTPStatus maps an HTTP status code to a stable ErrorCode class.
func classifyHTTPStatus(statusCode int) ErrorCode {
	switch {
	case isClientErrorStatus(statusCode):
		return APICodeClientError
	case isServerErrorStatus(statusCode):
		return APICodeServerError
	default:
		return APICodeUnknown
	}
}

// isClientErrorStatus reports whether the status code is in the 4xx range.
func isClientErrorStatus(status int) bool {
	return status >= 400 && status < 500
}

// isServerErrorStatus reports whether the status code is in the 5xx range.
func isServerErrorStatus(status int) bool {
	return status >= 500 && status < 600
}
