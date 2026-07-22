package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// EditPage creates or edits a page
func (c *Client) EditPage(ctx context.Context, args EditPageArgs) (EditResult, error) {
	if err := validateEditArgs(args); err != nil {
		return EditResult{}, err
	}

	editResult, err := c.performEdit(ctx, args)
	if err != nil && strings.Contains(err.Error(), "badtoken") {
		c.invalidateCSRFToken()
		editResult, err = c.performEdit(ctx, args)
	}
	if err != nil {
		return EditResult{}, err
	}
	return editResult, nil
}

// validateEditArgs checks required fields and content safety for an edit.
func validateEditArgs(args EditPageArgs) error {
	if args.Title == "" {
		return &ValidationError{
			Field:   "title",
			Message: "page title is required",
			Suggestion: `Provide a title for the page you want to edit.

Example:
  Title: "My Page"
  Title: "Category:My Category"
  Title: "User:Username/Subpage"`,
		}
	}
	if args.Content == "" {
		return &ValidationError{
			Field:   "content",
			Message: "page content is required",
			Suggestion: `Provide the wikitext content for the page.

Example:
  Content: "== Section ==\nThis is the page content."

If you want to clear a page, use a single space or redirect instead.`,
		}
	}
	if err := ValidateContentSize(args.Content, args.Title, MaxEditSize); err != nil {
		return err
	}
	return ValidateWikitextContent(args.Content, args.Title)
}

// performEdit executes a single edit attempt with a fresh CSRF token.
// buildEditAPIParams builds the form parameters for an edit API call.
func buildEditAPIParams(args EditPageArgs, token string) url.Values {
	params := url.Values{}
	params.Set("action", "edit")
	params.Set("title", args.Title)
	params.Set("text", args.Content)
	params.Set("token", token)
	if args.Summary != "" {
		params.Set("summary", args.Summary)
	}
	if args.Minor {
		params.Set("minor", "1")
	}
	if args.Bot {
		params.Set("bot", "1")
	}
	if args.Section != "" {
		params.Set("section", args.Section)
	}
	if args.CaptchaID != "" {
		params.Set("captchaid", args.CaptchaID)
	}
	if args.CaptchaWord != "" {
		params.Set("captchaword", args.CaptchaWord)
	}
	if args.BaseTimestamp != "" {
		params.Set("basetimestamp", args.BaseTimestamp)
	}
	return params
}

// editResultFromAPI converts a successful edit API response into an EditResult.
// It uses ctx to fetch (and cache) site info for building a pretty page URL;
// any failure to obtain site info is non-fatal — the index.php?title= form
// is used as a fallback.
func (c *Client) editResultFromAPI(ctx context.Context, edit map[string]interface{}) EditResult {
	r := EditResult{
		Success:    true,
		Title:      getString(edit["title"]),
		PageID:     getInt(edit["pageid"]),
		RevisionID: getInt(edit["newrevid"]),
		NewPage:    edit["new"] != nil,
		Message:    "Page edited successfully",
	}
	if r.NewPage {
		r.Message = "Page created successfully"
	}
	r.PageURL = c.pageURL(ctx, r.Title)
	return r
}

func (c *Client) performEdit(ctx context.Context, args EditPageArgs) (EditResult, error) {
	token, err := c.getCSRFToken(ctx)
	if err != nil {
		return EditResult{}, fmt.Errorf("authentication failed: %w", err)
	}

	resp, err := c.apiRequest(ctx, buildEditAPIParams(args, token))
	if err != nil {
		return EditResult{}, err
	}

	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		return EditResult{}, fmt.Errorf("%s: %s", getString(errInfo["code"]), getString(errInfo["info"]))
	}
	edit, ok := resp["edit"].(map[string]interface{})
	if !ok {
		return EditResult{}, fmt.Errorf("unexpected API response: missing 'edit' object")
	}

	if status := getString(edit["result"]); status != "Success" {
		return c.failedEditResult(args, edit, status), nil
	}

	editResult := c.editResultFromAPI(ctx, edit)
	op := AuditOpEdit
	if editResult.NewPage {
		op = AuditOpCreate
	}
	c.logAudit(c.buildAuditEntry(
		op, editResult.Title, args.Content, args.Summary,
		args.Minor, args.Bot, true, editResult.PageID, editResult.RevisionID, "",
	))
	return editResult, nil
}

// failedEditResult builds the EditResult (and audit entry) for a non-Success
// edit API response, including any CAPTCHA challenge details.
func (c *Client) failedEditResult(args EditPageArgs, edit map[string]interface{}, status string) EditResult {
	msg := fmt.Sprintf("Edit failed: %s", status)
	if info := getString(edit["info"]); info != "" {
		msg += fmt.Sprintf(" - %s", info)
	}
	var captchaType, captchaID, captchaQuestion string
	if captcha := getMap(edit["captcha"]); captcha != nil {
		captchaType = getString(captcha["type"])
		captchaID = getString(captcha["id"])
		captchaQuestion = getString(captcha["question"])
		msg += fmt.Sprintf(" (CAPTCHA: %s)", captchaType)
	}
	c.logAudit(c.buildAuditEntry(
		AuditOpEdit, args.Title, args.Content, args.Summary,
		args.Minor, args.Bot, false, 0, 0, msg,
	))
	return EditResult{
		Success:         false,
		Title:           args.Title,
		Message:         msg,
		CaptchaType:     captchaType,
		CaptchaID:       captchaID,
		CaptchaQuestion: captchaQuestion,
	}
}

// buildEditRevisionInfo creates revision info and undo instructions for an edit
func (c *Client) buildEditRevisionInfo(title string, oldRevision, newRevision int) (*EditRevisionInfo, *UndoInfo) {
	if oldRevision == 0 || newRevision == 0 {
		return nil, nil
	}

	// Derive wiki base URL from API URL (replace api.php with index.php)
	wikiBaseURL := strings.TrimSuffix(c.config.BaseURL, "api.php") + "index.php"

	// Build diff URL
	diffURL := fmt.Sprintf("%s?diff=%d&oldid=%d", wikiBaseURL, newRevision, oldRevision)

	// Build undo URL
	encodedTitle := url.QueryEscape(strings.ReplaceAll(title, " ", "_"))
	undoURL := fmt.Sprintf("%s?title=%s&action=edit&undoafter=%d&undo=%d", wikiBaseURL, encodedTitle, oldRevision, newRevision)

	// Build undo instruction
	undoInstruction := fmt.Sprintf("To undo: use wiki URL or revert to revision %d", oldRevision)

	return &EditRevisionInfo{
			OldRevision: int64(oldRevision),
			NewRevision: int64(newRevision),
			DiffURL:     diffURL,
		}, &UndoInfo{
			Instruction: undoInstruction,
			WikiURL:     undoURL,
		}
}

// pageURL builds the human-readable page URL for the given title.
//
// It tries to use the wiki's pretty URL form (e.g. /wiki/Foo_Bar) derived
// from siteinfo's Server and ArticlePath (cached after the first call).
// When siteinfo is unavailable, it falls back to the universal
// index.php?title= form derived from the configured API endpoint.
//
// MediaWiki's `server` field in siteinfo can be returned in three forms:
//   - absolute:   "https://wiki.example.com"
//   - scheme-less: "//wiki.example.com"  (some installations)
//   - protocol-relative with a port:  "//wiki.example.com:443"
//
// In the scheme-less case we borrow the scheme from the configured API
// endpoint so the printed URL is clickable from a terminal.
//
// Returns an empty string if the title is empty or the API URL is
// unconfigured.
func (c *Client) pageURL(ctx context.Context, title string) string {
	if title == "" || c.config.BaseURL == "" {
		return ""
	}

	pathTitle := strings.ReplaceAll(title, " ", "_")

	if info, err := c.GetWikiInfo(ctx, WikiInfoArgs{}); err == nil && info.Server != "" && info.ArticlePath != "" {
		if strings.Contains(info.ArticlePath, "$1") {
			server := ensureServerScheme(info.Server, c.config.BaseURL)
			// Path-escape the title, then unescape slashes so subpage
			// slashes survive (e.g. User:Alice/Sandbox stays as
			// User:Alice/Sandbox, not User:Alice%2FSandbox).
			escaped := strings.ReplaceAll(url.PathEscape(pathTitle), "%2F", "/")
			return server + strings.Replace(info.ArticlePath, "$1", escaped, 1)
		}
	}

	wikiBaseURL := strings.TrimSuffix(c.config.BaseURL, "api.php")
	if !strings.HasSuffix(wikiBaseURL, "/") {
		wikiBaseURL += "/"
	}
	wikiBaseURL += "index.php"
	return fmt.Sprintf("%s?title=%s", wikiBaseURL, url.QueryEscape(pathTitle))
}

// ensureServerScheme returns server with a scheme. If server is scheme-relative
// (e.g. "//wiki.example.com") the scheme from apiBase is prepended. Otherwise
// server is returned unchanged.
func ensureServerScheme(server, apiBase string) string {
	if !strings.HasPrefix(server, "//") {
		return server
	}
	if u, err := url.Parse(apiBase); err == nil && u.Scheme != "" {
		return u.Scheme + ":" + server
	}
	return server
}

// extractNormalizedTitleMap reads the "normalized" array from a MediaWiki query
// response and returns a map of normalized→original title. MediaWiki normalizes
// titles before lookup, so the original input title may not appear in the
// "pages" map.
// performMove executes a single move attempt with a fresh CSRF token.
