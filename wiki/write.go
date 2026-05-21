package wiki

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// EditPage creates or edits a page
func (c *Client) EditPage(ctx context.Context, args EditPageArgs) (EditResult, error) {
	if args.Title == "" {
		return EditResult{}, &ValidationError{
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
		return EditResult{}, &ValidationError{
			Field:   "content",
			Message: "page content is required",
			Suggestion: `Provide the wikitext content for the page.

Example:
  Content: "== Section ==\nThis is the page content."

If you want to clear a page, use a single space or redirect instead.`,
		}
	}

	// Validate content size
	if err := ValidateContentSize(args.Content, args.Title, MaxEditSize); err != nil {
		return EditResult{}, err
	}

	// Validate content for dangerous patterns
	if err := ValidateWikitextContent(args.Content, args.Title); err != nil {
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
	return params
}

// editResultFromAPI converts a successful edit API response into an EditResult.
func editResultFromAPI(edit map[string]interface{}) EditResult {
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
		c.logAudit(c.buildAuditEntry(
			AuditOpEdit, args.Title, args.Content, args.Summary,
			args.Minor, args.Bot, false, 0, 0,
			fmt.Sprintf("Edit failed: %s", status),
		))
		return EditResult{
			Success: false,
			Title:   args.Title,
			Message: fmt.Sprintf("Edit failed: %s", status),
		}, nil
	}

	editResult := editResultFromAPI(edit)
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

// FindReplace finds and replaces text in a wiki page
// compileFindReplaceRegex validates and compiles the find-replace pattern.
// Literal mode escapes the input; regex mode bounds pattern length to 500 chars.
func compileFindReplaceRegex(find string, useRegex bool) (*regexp.Regexp, error) {
	if !useRegex {
		return regexp.MustCompile(regexp.QuoteMeta(find)), nil
	}
	if len(find) > 500 {
		return nil, fmt.Errorf("regex pattern too long (max 500 characters)")
	}
	re, err := regexp.Compile(find)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}
	return re, nil
}

// findReplaceOp bundles regex + behavior for a single replacement run.
type findReplaceOp struct {
	re      *regexp.Regexp
	replace string
	all     bool
}

// findReplaceLineOutcome captures one line's replacement outcome.
type findReplaceLineOutcome struct {
	newLine      string
	change       *TextChange
	matchCount   int
	replaceCount int
}

// applyFindReplaceToLine computes the replacement for a single line. When
// op.all is false and replacementsDone > 0, the line is only counted; not rewritten.
func applyFindReplaceToLine(line string, lineNum int, op findReplaceOp, replacementsDone int) findReplaceLineOutcome {
	matches := op.re.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return findReplaceLineOutcome{newLine: line}
	}
	out := findReplaceLineOutcome{newLine: line, matchCount: len(matches)}
	if !op.all && replacementsDone > 0 {
		return out
	}

	var newLine string
	var replaceCount int
	if op.all {
		newLine = op.re.ReplaceAllString(line, op.replace)
		if newLine != line {
			replaceCount = len(matches)
		}
	} else {
		replaced := false
		newLine = op.re.ReplaceAllStringFunc(line, func(match string) string {
			if !replaced {
				replaced = true
				return op.replace
			}
			return match
		})
		if replaced {
			replaceCount = 1
		}
	}
	if newLine == line {
		return out
	}
	out.newLine = newLine
	out.replaceCount = replaceCount
	out.change = &TextChange{
		Line:    lineNum + 1,
		Before:  line,
		After:   newLine,
		Context: extractContext(line, matches[0][0], matches[0][1], 40),
	}
	return out
}

// applyFindReplaceToContent runs the line-by-line replacement on the page content.
func applyFindReplaceToContent(content string, op findReplaceOp) (newContent string, changes []TextChange, matchCount, replaceCount int) {
	lines := strings.Split(content, "\n")
	newLines := make([]string, len(lines))
	for i, line := range lines {
		outcome := applyFindReplaceToLine(line, i, op, replaceCount)
		newLines[i] = outcome.newLine
		matchCount += outcome.matchCount
		replaceCount += outcome.replaceCount
		if outcome.change != nil {
			changes = append(changes, *outcome.change)
		}
	}
	return strings.Join(newLines, "\n"), changes, matchCount, replaceCount
}

// saveFindReplaceEdit writes the rewritten content back to the wiki and records
// the resulting revision metadata on the result.
func (c *Client) saveFindReplaceEdit(ctx context.Context, page PageContent, newContent string, args FindReplaceArgs, result *FindReplaceResult) error {
	summary := args.Summary
	if summary == "" {
		summary = fmt.Sprintf("Replaced '%s' with '%s'", truncateString(args.Find, 30), truncateString(args.Replace, 30))
	}
	oldRevision := page.Revision
	editResult, err := c.EditPage(ctx, EditPageArgs{
		Title:   page.Title,
		Content: newContent,
		Summary: summary,
		Minor:   args.Minor,
	})
	if err != nil {
		return fmt.Errorf("failed to save changes: %w", err)
	}
	result.Success = editResult.Success
	result.RevisionID = editResult.RevisionID
	result.Message = fmt.Sprintf("Replaced %d occurrence(s)", result.ReplaceCount)
	result.Revision, result.Undo = c.buildEditRevisionInfo(page.Title, oldRevision, editResult.RevisionID)
	return nil
}

func (c *Client) FindReplace(ctx context.Context, args FindReplaceArgs) (FindReplaceResult, error) {
	if args.Title == "" {
		return FindReplaceResult{}, fmt.Errorf("title is required")
	}
	if args.Find == "" {
		return FindReplaceResult{}, fmt.Errorf("find text is required")
	}

	re, err := compileFindReplaceRegex(args.Find, args.UseRegex)
	if err != nil {
		return FindReplaceResult{}, err
	}

	page, err := c.GetPage(ctx, GetPageArgs{Title: args.Title, Format: "wikitext"})
	if err != nil {
		return FindReplaceResult{}, fmt.Errorf("failed to get page: %w", err)
	}

	op := findReplaceOp{re: re, replace: args.Replace, all: args.All}
	newContent, changes, matchCount, replaceCount := applyFindReplaceToContent(page.Content, op)
	result := FindReplaceResult{
		Title:        page.Title,
		Preview:      args.Preview,
		MatchCount:   matchCount,
		ReplaceCount: replaceCount,
		Changes:      changes,
	}

	if matchCount == 0 {
		result.Message = fmt.Sprintf("No matches found for '%s'", args.Find)
		return result, nil
	}
	if args.Preview {
		result.Success = true
		result.Message = fmt.Sprintf("Preview: %d matches found, %d would be replaced", matchCount, replaceCount)
		return result, nil
	}

	if err := c.saveFindReplaceEdit(ctx, page, newContent, args, &result); err != nil {
		return FindReplaceResult{}, err
	}
	return result, nil
}

// ApplyFormatting applies formatting to text in a wiki page
func (c *Client) ApplyFormatting(ctx context.Context, args ApplyFormattingArgs) (ApplyFormattingResult, error) {
	if args.Title == "" {
		return ApplyFormattingResult{}, fmt.Errorf("title is required")
	}
	if args.Text == "" {
		return ApplyFormattingResult{}, fmt.Errorf("text is required")
	}
	if args.Format == "" {
		return ApplyFormattingResult{}, fmt.Errorf("format is required")
	}

	// Map format to wikitext markup
	formatMap := map[string][2]string{
		"strikethrough": {"<s>", "</s>"},
		"strike":        {"<s>", "</s>"},
		"bold":          {"'''", "'''"},
		"italic":        {"''", "''"},
		"underline":     {"<u>", "</u>"},
		"code":          {"<code>", "</code>"},
		"nowiki":        {"<nowiki>", "</nowiki>"},
	}

	markup, ok := formatMap[strings.ToLower(args.Format)]
	if !ok {
		return ApplyFormattingResult{}, fmt.Errorf("unknown format: %s (use: strikethrough, bold, italic, underline, code, nowiki)", args.Format)
	}

	// Use FindReplace to apply formatting
	replacement := markup[0] + args.Text + markup[1]

	findArgs := FindReplaceArgs{
		Title:   args.Title,
		Find:    args.Text,
		Replace: replacement,
		All:     args.All,
		Preview: args.Preview,
		Minor:   true,
	}

	if args.Summary != "" {
		findArgs.Summary = args.Summary
	} else {
		findArgs.Summary = fmt.Sprintf("Applied %s formatting to '%s'", args.Format, truncateString(args.Text, 30))
	}

	frResult, err := c.FindReplace(ctx, findArgs)
	if err != nil {
		return ApplyFormattingResult{}, err
	}

	return ApplyFormattingResult{
		Success:     frResult.Success,
		Title:       frResult.Title,
		Format:      args.Format,
		MatchCount:  frResult.MatchCount,
		FormatCount: frResult.ReplaceCount,
		Preview:     args.Preview,
		Changes:     frResult.Changes,
		RevisionID:  frResult.RevisionID,
		Revision:    frResult.Revision,
		Undo:        frResult.Undo,
		Message:     frResult.Message,
	}, nil
}

// BulkReplace performs find/replace across multiple pages
// processBulkReplacePage runs a single page replace and projects the result
// into the per-page bulk shape. Errors are captured on the result so one bad
// page doesn't sink the whole bulk operation.
func (c *Client) processBulkReplacePage(ctx context.Context, title string, args BulkReplaceArgs, summary string) PageReplaceResult {
	pageResult := PageReplaceResult{Title: title}
	frResult, err := c.FindReplace(ctx, FindReplaceArgs{
		Title:    title,
		Find:     args.Find,
		Replace:  args.Replace,
		UseRegex: args.UseRegex,
		All:      true,
		Preview:  args.Preview,
		Summary:  summary,
	})
	if err != nil {
		pageResult.Error = err.Error()
		return pageResult
	}
	pageResult.MatchCount = frResult.MatchCount
	pageResult.ReplaceCount = frResult.ReplaceCount
	pageResult.RevisionID = frResult.RevisionID
	pageResult.Revision = frResult.Revision
	pageResult.Undo = frResult.Undo
	if args.Preview {
		pageResult.Changes = frResult.Changes
	}
	return pageResult
}

func (c *Client) BulkReplace(ctx context.Context, args BulkReplaceArgs) (BulkReplaceResult, error) {
	if args.Find == "" {
		return BulkReplaceResult{}, fmt.Errorf("find text is required")
	}

	limit := normalizeLimit(args.Limit, 10, 50)
	pagesToProcess, err := c.collectPagesFromArgs(ctx, args.Pages, args.Category, limit, "pages")
	if err != nil {
		return BulkReplaceResult{}, err
	}

	summary := args.Summary
	if summary == "" {
		summary = fmt.Sprintf("Bulk replace: '%s' → '%s'", truncateString(args.Find, 20), truncateString(args.Replace, 20))
	}

	result := BulkReplaceResult{
		Preview: args.Preview,
		Results: make([]PageReplaceResult, 0, len(pagesToProcess)),
	}

	for _, pageTitle := range pagesToProcess {
		pageResult := c.processBulkReplacePage(ctx, pageTitle, args, summary)
		if pageResult.Error == "" && pageResult.ReplaceCount > 0 {
			result.PagesModified++
			result.TotalChanges += pageResult.ReplaceCount
		}
		result.Results = append(result.Results, pageResult)
	}

	result.PagesProcessed = len(result.Results)
	if args.Preview {
		result.Message = fmt.Sprintf("Preview: %d pages would be modified with %d total changes", result.PagesModified, result.TotalChanges)
	} else {
		result.Message = fmt.Sprintf("Modified %d pages with %d total changes", result.PagesModified, result.TotalChanges)
	}
	return result, nil
}

// truncateString truncates a string for display
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
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

// extractNormalizedTitleMap reads the "normalized" array from a MediaWiki query
// response and returns a map of normalized→original title. MediaWiki normalizes
// titles before lookup, so the original input title may not appear in the
// "pages" map.
func extractNormalizedTitleMap(query map[string]interface{}) map[string]string {
	normalized := make(map[string]string)
	normList, ok := query["normalized"].([]interface{})
	if !ok {
		return normalized
	}
	for _, n := range normList {
		norm, ok := n.(map[string]interface{})
		if !ok {
			continue
		}
		normalized[getString(norm["to"])] = getString(norm["from"])
	}
	return normalized
}

// recordExistenceFromBatchPages walks the "pages" map from a query response
// and writes existence flags into result, including the un-normalized form
// if applicable.
func recordExistenceFromBatchPages(pages map[string]interface{}, normalized map[string]string, result map[string]bool) {
	for _, pageData := range pages {
		page, ok := pageData.(map[string]interface{})
		if !ok {
			continue
		}
		title := getString(page["title"])
		_, missing := page["missing"]
		exists := !missing
		result[title] = exists
		if originalTitle, ok := normalized[title]; ok {
			result[originalTitle] = exists
		}
	}
}

// queryPageExistence runs one batch existence query and merges the outcome
// into result. Returns the API error (network/auth) without aborting on
// missing-from-response cases — those are caught by the caller's fallback loop.
func (c *Client) queryPageExistence(ctx context.Context, batch []string, result map[string]bool) error {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", strings.Join(batch, "|"))
	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return err
	}
	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return nil
	}
	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return nil
	}
	recordExistenceFromBatchPages(pages, extractNormalizedTitleMap(query), result)
	return nil
}

// checkPagesExist checks if multiple pages exist using MediaWiki's multi-value API
// Returns a map of page title -> exists (bool)
// This is much more efficient than calling GetPageInfo for each page individually
func (c *Client) checkPagesExist(ctx context.Context, titles []string) (map[string]bool, error) {
	if len(titles) == 0 {
		return make(map[string]bool), nil
	}

	const maxTitlesPerRequest = 50
	result := make(map[string]bool, len(titles))

	for i := 0; i < len(titles); i += maxTitlesPerRequest {
		end := i + maxTitlesPerRequest
		if end > len(titles) {
			end = len(titles)
		}
		if err := c.queryPageExistence(ctx, titles[i:end], result); err != nil {
			return nil, err
		}
	}

	// Default any titles that didn't appear in any response to non-existent.
	for _, title := range titles {
		if _, ok := result[title]; !ok {
			result[title] = false
		}
	}
	return result, nil
}

// fileTypeFromMIME returns a friendly file type label from a MIME string.
func fileTypeFromMIME(mimeType string) string {
	if mimeType == "application/pdf" {
		return "pdf"
	}
	if strings.HasPrefix(mimeType, "text/") {
		return strings.TrimPrefix(mimeType, "text/")
	}
	return mimeType
}

// extractFileURLFromPageEntry pulls the download URL and file type from one
// entry in the imageinfo response. Returns an error if the entry is malformed,
// represents a missing file, or has no URL.
func extractFileURLFromPageEntry(page map[string]interface{}, filename string) (fileURL, fileType string, err error) {
	if _, missing := page["missing"]; missing {
		return "", "", fmt.Errorf("file '%s' does not exist", filename)
	}
	imageinfo, ok := page["imageinfo"].([]interface{})
	if !ok || len(imageinfo) == 0 {
		return "", "", fmt.Errorf("no file info available for '%s'", filename)
	}
	info, ok := imageinfo[0].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("unexpected API response: invalid imageinfo format for '%s'", filename)
	}
	fileURL = getString(info["url"])
	if fileURL == "" {
		return "", "", fmt.Errorf("no download URL for '%s'", filename)
	}
	return fileURL, fileTypeFromMIME(getString(info["mime"])), nil
}

func (c *Client) getFileURL(ctx context.Context, filename string) (string, string, error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", filename)
	params.Set("prop", "imageinfo")
	params.Set("iiprop", "url|mime|size")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return "", "", err
	}
	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("unexpected response format")
	}
	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("no pages in response")
	}

	for _, pageData := range pages {
		page, ok := pageData.(map[string]interface{})
		if !ok {
			continue
		}
		return extractFileURLFromPageEntry(page, filename)
	}
	return "", "", fmt.Errorf("file '%s' not found", filename)
}

// downloadFile downloads a file from the given URL.
//
// SECURITY: The fileURL is validated against SSRF (private IPs, DNS rebinding,
// redirect bypass) via validateFileURL before any network I/O. The actual
// HTTP request goes through downloadClient, which uses safeDialer +
// CheckRedirect for defense in depth. Callers in production today (only
// SearchInFile) get fileURL from the wiki's own imageinfo API, so the URL is
// already trusted. The validation closes the gap for any future caller that
// might accept user-supplied URLs.
//
// Tests can set c.allowPrivateDownloadForTest to skip validation against
// httptest servers (which bind to 127.0.0.1).
func (c *Client) downloadFile(ctx context.Context, fileURL string) ([]byte, error) {
	httpClient := downloadClient
	if !c.allowPrivateDownloadForTest {
		if err := validateFileURL(fileURL); err != nil {
			return nil, fmt.Errorf("download URL rejected: %w", err)
		}
	} else {
		// Test path: bypass SSRF guards so httptest (loopback) servers work.
		httpClient = c.httpClient
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", c.config.UserAgent)

	// Forward session cookies from the API client's jar onto the download
	// request. Required for MediaWiki instances that gate file access behind
	// a login (the $wgUploadDirectory + img_auth.php pattern common on
	// corporate wikis): the file-serving endpoint refuses anonymous reads
	// even when the bot is authenticated for api.php. The standard cookie jar
	// is domain-scoped (RFC 6265), so cookies set for the wiki host are only
	// forwarded when fileURL is on the same host — no credential leakage to
	// arbitrary external hosts.
	if c.httpClient != nil && c.httpClient.Jar != nil {
		for _, cookie := range c.httpClient.Jar.Cookies(req.URL) {
			req.AddCookie(cookie)
		}
	}

	resp, err := httpClient.Do(req) // #nosec G107 G704 -- URL validated by validateFileURL; request goes through downloadClient (safeDialer + CheckRedirect)
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Surface the known bot-password + img_auth.php gotcha with an
		// actionable message rather than a bare status code. Wikis that gate
		// file access via img_auth.php check user-level auth (UserID/UserName/
		// Token cookies), which the legacy `action=login` bot-password flow
		// doesn't establish — only a BPsession cookie. The session is fine
		// for api.php but the file-serving endpoint rejects it. Tracked as
		// mediawiki-mcp-server-xum.
		if resp.StatusCode == http.StatusForbidden &&
			c.config.HasCredentials() &&
			strings.Contains(req.URL.Path, "/img_auth.php") {
			return nil, fmt.Errorf(
				"download failed with status 403: %s is gated behind img_auth.php and refused the bot-password session. "+
					"Bot-password logins authenticate against api.php but img_auth.php enforces user-level auth this flow does not establish. "+
					"This is a wiki-side limitation, not a CLI bug. Workarounds: download the file manually from the wiki UI, "+
					"or ask a wiki admin to grant additional rights to your bot password at Special:BotPasswords",
				req.URL.Host)
		}
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Limit download size to 50MB
	const maxSize = 50 * 1024 * 1024
	limitedReader := &io.LimitedReader{R: resp.Body, N: maxSize}

	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file data: %w", err)
	}

	if limitedReader.N <= 0 {
		return nil, fmt.Errorf("file exceeds maximum size of 50MB")
	}

	return data, nil
}

// performMove executes a single move attempt with a fresh CSRF token.
func (c *Client) performMove(ctx context.Context, args MovePageArgs) (map[string]interface{}, error) {
	token, err := c.getCSRFToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	params := url.Values{}
	params.Set("action", "move")
	params.Set("from", args.From)
	params.Set("to", args.To)
	params.Set("token", token)

	if args.Reason != "" {
		params.Set("reason", args.Reason)
	}

	if args.NoRedirect {
		params.Set("noredirect", "1")
	}

	// Default: move talk page
	if !args.MoveTalk {
		// MediaWiki moves talk by default, so we only set movetalk=0 if explicitly disabled
	} else {
		params.Set("movetalk", "1")
	}

	if args.MoveSubpages {
		params.Set("movesubpages", "1")
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return nil, err
	}

	// Check for badtoken error so caller can retry
	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		code := getString(errInfo["code"])
		if code == "badtoken" {
			return nil, fmt.Errorf("%s: %s", code, getString(errInfo["info"]))
		}
	}

	return resp, nil
}

// MovePage moves (renames) a wiki page
func (c *Client) MovePage(ctx context.Context, args MovePageArgs) (MovePageResult, error) {
	if args.From == "" {
		return MovePageResult{}, &ValidationError{
			Field:   "from",
			Message: "source page title is required",
		}
	}
	if args.To == "" {
		return MovePageResult{}, &ValidationError{
			Field:   "to",
			Message: "target page title is required",
		}
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return MovePageResult{}, fmt.Errorf("authentication required for page moves: %w", err)
	}

	resp, err := c.performMove(ctx, args)
	if err != nil && strings.Contains(err.Error(), "badtoken") {
		c.invalidateCSRFToken()
		resp, err = c.performMove(ctx, args)
	}
	if err != nil {
		return MovePageResult{}, err
	}

	// Check for errors
	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		return MovePageResult{
			Success: false,
			From:    args.From,
			To:      args.To,
			Message: fmt.Sprintf("Move failed: %s", getString(errInfo["info"])),
		}, nil
	}

	moveData, ok := resp["move"].(map[string]interface{})
	if !ok {
		return MovePageResult{
			Success: false,
			From:    args.From,
			To:      args.To,
			Message: "Unexpected response format",
		}, nil
	}

	result := MovePageResult{
		Success: true,
		From:    getString(moveData["from"]),
		To:      getString(moveData["to"]),
		Reason:  args.Reason,
		Message: fmt.Sprintf("Page moved from '%s' to '%s'", getString(moveData["from"]), getString(moveData["to"])),
	}

	// Check if talk page was moved
	if _, hasTalkFrom := moveData["talkfrom"]; hasTalkFrom {
		result.TalkMoved = true
	}

	// Build redirect URL
	wikiBaseURL := strings.TrimSuffix(c.config.BaseURL, "api.php") + "index.php"
	encodedFrom := url.QueryEscape(strings.ReplaceAll(result.From, " ", "_"))
	result.RedirectURL = fmt.Sprintf("%s?title=%s&redirect=no", wikiBaseURL, encodedFrom)

	// Log the move
	c.logAudit(AuditEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Operation: "move",
		Title:     result.From + " → " + result.To,
		Summary:   args.Reason,
		WikiURL:   c.config.BaseURL,
		Success:   true,
	})

	return result, nil
}

// ManageCategories adds or removes categories from a page
// categoryTagRegex matches "[[Category:Name|sortkey]]" anywhere in wikitext.
var categoryTagRegex = regexp.MustCompile(`\[\[Category:([^\]|]+)(?:\|[^\]]*)?\]\]`)

// parseExistingCategories returns the set of category names already present in
// the wikitext content.
func parseExistingCategories(content string) map[string]bool {
	existing := make(map[string]bool)
	for _, m := range categoryTagRegex.FindAllStringSubmatch(content, -1) {
		existing[strings.TrimSpace(m[1])] = true
	}
	return existing
}

// removeCategoriesFromContent removes the listed categories from the content
// and returns the rewritten content plus the per-category outcome (removed,
// not-found). The existing-set is updated in place.
func removeCategoriesFromContent(content string, toRemove []string, existing map[string]bool) (newContent string, removed, notFound []string) {
	newContent = content
	for _, cat := range toRemove {
		cat = strings.TrimSpace(cat)
		if !existing[cat] {
			notFound = append(notFound, cat)
			continue
		}
		removeRegex := regexp.MustCompile(`\n?\[\[Category:` + regexp.QuoteMeta(cat) + `(?:\|[^\]]*)?\]\]\n?`)
		newContent = removeRegex.ReplaceAllString(newContent, "\n")
		removed = append(removed, cat)
		delete(existing, cat)
	}
	return newContent, removed, notFound
}

// addCategoriesToContent appends category tags missing from the existing-set.
// Categories already present are reported via alreadyPresent.
func addCategoriesToContent(content string, toAdd []string, existing map[string]bool) (newContent string, added, alreadyPresent []string) {
	newContent = content
	for _, cat := range toAdd {
		cat = strings.TrimSpace(cat)
		if existing[cat] {
			alreadyPresent = append(alreadyPresent, cat)
			continue
		}
		newContent = strings.TrimRight(newContent, "\n") + "\n[[Category:" + cat + "]]\n"
		added = append(added, cat)
		existing[cat] = true
	}
	return newContent, added, alreadyPresent
}

// buildCategoryEditSummary composes the default edit summary for category changes.
func buildCategoryEditSummary(added, removed []string) string {
	parts := make([]string, 0, 2)
	if len(added) > 0 {
		parts = append(parts, fmt.Sprintf("Added categories: %s", strings.Join(added, ", ")))
	}
	if len(removed) > 0 {
		parts = append(parts, fmt.Sprintf("Removed categories: %s", strings.Join(removed, ", ")))
	}
	return strings.Join(parts, ". ")
}

func (c *Client) ManageCategories(ctx context.Context, args ManageCategoriesArgs) (ManageCategoriesResult, error) {
	if args.Title == "" {
		return ManageCategoriesResult{}, fmt.Errorf("title is required")
	}
	if len(args.Add) == 0 && len(args.Remove) == 0 {
		return ManageCategoriesResult{}, fmt.Errorf("at least one category to add or remove is required")
	}

	page, err := c.GetPage(ctx, GetPageArgs{Title: args.Title, Format: "wikitext"})
	if err != nil {
		return ManageCategoriesResult{}, fmt.Errorf("failed to get page: %w", err)
	}

	existing := parseExistingCategories(page.Content)
	result := ManageCategoriesResult{
		Title:             page.Title,
		Preview:           args.Preview,
		CurrentCategories: keysOf(existing),
	}

	newContent, removed, notFound := removeCategoriesFromContent(page.Content, args.Remove, existing)
	result.Removed = removed
	result.NotFound = notFound

	newContent, added, alreadyPresent := addCategoriesToContent(newContent, args.Add, existing)
	result.Added = added
	result.AlreadyPresent = alreadyPresent
	result.CurrentCategories = keysOf(existing)

	if len(result.Added) == 0 && len(result.Removed) == 0 {
		result.Success = true
		result.Message = "No changes needed"
		return result, nil
	}
	if args.Preview {
		result.Success = true
		result.Message = fmt.Sprintf("Preview: would add %d and remove %d categories", len(result.Added), len(result.Removed))
		return result, nil
	}

	summary := args.Summary
	if summary == "" {
		summary = buildCategoryEditSummary(result.Added, result.Removed)
	}
	oldRevision := page.Revision
	editResult, err := c.EditPage(ctx, EditPageArgs{
		Title:   page.Title,
		Content: newContent,
		Summary: summary,
		Minor:   true,
	})
	if err != nil {
		return ManageCategoriesResult{}, fmt.Errorf("failed to save changes: %w", err)
	}
	result.Success = editResult.Success
	result.RevisionID = editResult.RevisionID
	result.Message = fmt.Sprintf("Added %d, removed %d categories", len(result.Added), len(result.Removed))
	result.Revision, result.Undo = c.buildEditRevisionInfo(page.Title, oldRevision, editResult.RevisionID)
	return result, nil
}

// keysOf returns the keys of a string-bool map as a slice.
func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
