package wiki

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
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
		return c.failedEditResult(args, edit, status), nil
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

	newLine, replaceCount := op.rewriteLine(line, len(matches))
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

// rewriteLine performs the replacement on a line and reports how many
// occurrences were replaced. In all-mode every match is replaced; otherwise
// only the first match is.
func (op findReplaceOp) rewriteLine(line string, matchCount int) (newLine string, replaceCount int) {
	if op.all {
		newLine = op.re.ReplaceAllString(line, op.replace)
		if newLine != line {
			replaceCount = matchCount
		}
		return newLine, replaceCount
	}

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
	return newLine, replaceCount
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

// findReplaceSaveInput bundles everything saveFindReplaceEdit needs to write a
// find/replace edit back to the wiki.
type findReplaceSaveInput struct {
	page       PageContent
	newContent string
	args       FindReplaceArgs
}

// saveFindReplaceEdit writes the rewritten content back to the wiki and records
// the resulting revision metadata on the result.
func (c *Client) saveFindReplaceEdit(ctx context.Context, in findReplaceSaveInput, result *FindReplaceResult) error {
	summary := in.args.Summary
	if summary == "" {
		summary = fmt.Sprintf("Replaced '%s' with '%s'", truncateString(in.args.Find, 30), truncateString(in.args.Replace, 30))
	}
	oldRevision := in.page.Revision
	editResult, err := c.EditPage(ctx, EditPageArgs{
		Title:   in.page.Title,
		Content: in.newContent,
		Summary: summary,
		Minor:   in.args.Minor,
	})
	if err != nil {
		return fmt.Errorf("failed to save changes: %w", err)
	}
	result.Success = editResult.Success
	result.RevisionID = editResult.RevisionID
	result.Message = fmt.Sprintf("Replaced %d occurrence(s)", result.ReplaceCount)
	result.Revision, result.Undo = c.buildEditRevisionInfo(in.page.Title, oldRevision, editResult.RevisionID)
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

	if err := c.saveFindReplaceEdit(ctx, findReplaceSaveInput{page: page, newContent: newContent, args: args}, &result); err != nil {
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
		result.add(c.processBulkReplacePage(ctx, pageTitle, args, summary))
	}

	result.PagesProcessed = len(result.Results)
	result.Message = bulkReplaceMessage(args.Preview, result.PagesModified, result.TotalChanges)
	return result, nil
}

// add records one page result, updating modified/total counters.
func (r *BulkReplaceResult) add(pageResult PageReplaceResult) {
	if pageResult.Error == "" && pageResult.ReplaceCount > 0 {
		r.PagesModified++
		r.TotalChanges += pageResult.ReplaceCount
	}
	r.Results = append(r.Results, pageResult)
}

// bulkReplaceMessage renders the preview/applied summary line.
func bulkReplaceMessage(preview bool, pagesModified, totalChanges int) string {
	if preview {
		return fmt.Sprintf("Preview: %d pages would be modified with %d total changes", pagesModified, totalChanges)
	}
	return fmt.Sprintf("Modified %d pages with %d total changes", pagesModified, totalChanges)
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
// performMove executes a single move attempt with a fresh CSRF token.
