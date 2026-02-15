package wiki

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
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

	token, err := c.getCSRFToken(ctx)
	if err != nil {
		return EditResult{}, fmt.Errorf("authentication failed: %w", err)
	}

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

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return EditResult{}, err
	}

	edit := resp["edit"].(map[string]interface{})
	result := getString(edit["result"])

	if result != "Success" {
		// Log failed edit attempt
		c.logAudit(c.buildAuditEntry(
			AuditOpEdit, args.Title, args.Content, args.Summary,
			args.Minor, args.Bot, false, 0, 0,
			fmt.Sprintf("Edit failed: %s", result),
		))
		return EditResult{
			Success: false,
			Title:   args.Title,
			Message: fmt.Sprintf("Edit failed: %s", result),
		}, nil
	}

	editResult := EditResult{
		Success:    true,
		Title:      getString(edit["title"]),
		PageID:     int(edit["pageid"].(float64)),
		RevisionID: int(edit["newrevid"].(float64)),
		NewPage:    edit["new"] != nil,
		Message:    "Page edited successfully",
	}

	if editResult.NewPage {
		editResult.Message = "Page created successfully"
	}

	// Log successful edit
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
func (c *Client) FindReplace(ctx context.Context, args FindReplaceArgs) (FindReplaceResult, error) {
	if args.Title == "" {
		return FindReplaceResult{}, fmt.Errorf("title is required")
	}
	if args.Find == "" {
		return FindReplaceResult{}, fmt.Errorf("find text is required")
	}

	// Get current page content
	page, err := c.GetPage(ctx, GetPageArgs{Title: args.Title, Format: "wikitext"})
	if err != nil {
		return FindReplaceResult{}, fmt.Errorf("failed to get page: %w", err)
	}

	result := FindReplaceResult{
		Title:   page.Title,
		Preview: args.Preview,
	}

	// Build regex pattern
	var re *regexp.Regexp
	if args.UseRegex {
		re, err = regexp.Compile(args.Find)
		if err != nil {
			return FindReplaceResult{}, fmt.Errorf("invalid regex pattern: %w", err)
		}
	} else {
		re = regexp.MustCompile(regexp.QuoteMeta(args.Find))
	}

	// Find all matches with line context
	lines := strings.Split(page.Content, "\n")
	var changes []TextChange
	newLines := make([]string, len(lines))
	copy(newLines, lines)

	for lineNum, line := range lines {
		matches := re.FindAllStringIndex(line, -1)
		if len(matches) == 0 {
			continue
		}

		result.MatchCount += len(matches)

		// Apply replacements
		if args.All {
			newLine := re.ReplaceAllString(line, args.Replace)
			if newLine != line {
				changes = append(changes, TextChange{
					Line:    lineNum + 1,
					Before:  line,
					After:   newLine,
					Context: extractContext(line, matches[0][0], matches[0][1], 40),
				})
				newLines[lineNum] = newLine
				result.ReplaceCount += len(matches)
			}
		} else if result.ReplaceCount == 0 {
			// Replace first occurrence only
			newLine := re.ReplaceAllStringFunc(line, func(match string) string {
				if result.ReplaceCount == 0 {
					result.ReplaceCount++
					return args.Replace
				}
				return match
			})
			if newLine != line {
				changes = append(changes, TextChange{
					Line:    lineNum + 1,
					Before:  line,
					After:   newLine,
					Context: extractContext(line, matches[0][0], matches[0][1], 40),
				})
				newLines[lineNum] = newLine
			}
		}
	}

	result.Changes = changes

	if result.MatchCount == 0 {
		result.Message = fmt.Sprintf("No matches found for '%s'", args.Find)
		return result, nil
	}

	if args.Preview {
		result.Success = true
		result.Message = fmt.Sprintf("Preview: %d matches found, %d would be replaced", result.MatchCount, result.ReplaceCount)
		return result, nil
	}

	// Capture old revision before edit
	oldRevision := page.Revision

	// Apply the edit
	newContent := strings.Join(newLines, "\n")
	summary := args.Summary
	if summary == "" {
		summary = fmt.Sprintf("Replaced '%s' with '%s'", truncateString(args.Find, 30), truncateString(args.Replace, 30))
	}

	editResult, err := c.EditPage(ctx, EditPageArgs{
		Title:   page.Title,
		Content: newContent,
		Summary: summary,
		Minor:   args.Minor,
	})
	if err != nil {
		return FindReplaceResult{}, fmt.Errorf("failed to save changes: %w", err)
	}

	result.Success = editResult.Success
	result.RevisionID = editResult.RevisionID
	result.Message = fmt.Sprintf("Replaced %d occurrence(s)", result.ReplaceCount)

	// Add revision info and undo instructions
	result.Revision, result.Undo = c.buildEditRevisionInfo(page.Title, oldRevision, editResult.RevisionID)

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
func (c *Client) BulkReplace(ctx context.Context, args BulkReplaceArgs) (BulkReplaceResult, error) {
	if args.Find == "" {
		return BulkReplaceResult{}, fmt.Errorf("find text is required")
	}

	// Get pages to process
	var pagesToProcess []string
	limit := normalizeLimit(args.Limit, 10, 50)

	if len(args.Pages) > 0 {
		pagesToProcess = args.Pages
		if len(pagesToProcess) > limit {
			pagesToProcess = pagesToProcess[:limit]
		}
	} else if args.Category != "" {
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
			Category: args.Category,
			Limit:    limit,
		})
		if err != nil {
			return BulkReplaceResult{}, fmt.Errorf("failed to get category members: %w", err)
		}
		for _, p := range catResult.Members {
			pagesToProcess = append(pagesToProcess, p.Title)
		}
	} else {
		return BulkReplaceResult{}, fmt.Errorf("either 'pages' or 'category' must be specified")
	}

	result := BulkReplaceResult{
		Preview: args.Preview,
		Results: make([]PageReplaceResult, 0, len(pagesToProcess)),
	}

	summary := args.Summary
	if summary == "" {
		summary = fmt.Sprintf("Bulk replace: '%s' → '%s'", truncateString(args.Find, 20), truncateString(args.Replace, 20))
	}

	for _, pageTitle := range pagesToProcess {
		pageResult := PageReplaceResult{Title: pageTitle}

		frResult, err := c.FindReplace(ctx, FindReplaceArgs{
			Title:    pageTitle,
			Find:     args.Find,
			Replace:  args.Replace,
			UseRegex: args.UseRegex,
			All:      true,
			Preview:  args.Preview,
			Summary:  summary,
		})

		if err != nil {
			pageResult.Error = err.Error()
		} else {
			pageResult.MatchCount = frResult.MatchCount
			pageResult.ReplaceCount = frResult.ReplaceCount
			pageResult.RevisionID = frResult.RevisionID
			pageResult.Revision = frResult.Revision
			pageResult.Undo = frResult.Undo
			if args.Preview {
				pageResult.Changes = frResult.Changes
			}

			if frResult.ReplaceCount > 0 {
				result.PagesModified++
				result.TotalChanges += frResult.ReplaceCount
			}
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

// checkPagesExist checks if multiple pages exist using MediaWiki's multi-value API
// Returns a map of page title -> exists (bool)
// This is much more efficient than calling GetPageInfo for each page individually
func (c *Client) checkPagesExist(ctx context.Context, titles []string) (map[string]bool, error) {
	if len(titles) == 0 {
		return make(map[string]bool), nil
	}

	result := make(map[string]bool, len(titles))

	// MediaWiki API supports up to 50 titles per request
	const maxTitlesPerRequest = 50

	for i := 0; i < len(titles); i += maxTitlesPerRequest {
		end := i + maxTitlesPerRequest
		if end > len(titles) {
			end = len(titles)
		}
		batch := titles[i:end]

		// Join titles with pipe separator for MediaWiki API
		params := url.Values{}
		params.Set("action", "query")
		params.Set("titles", strings.Join(batch, "|"))

		resp, err := c.apiRequest(ctx, params)
		if err != nil {
			return nil, err
		}

		query, ok := resp["query"].(map[string]interface{})
		if !ok {
			continue
		}

		pages, ok := query["pages"].(map[string]interface{})
		if !ok {
			continue
		}

		// Build a normalized title map (MediaWiki may normalize titles)
		normalized := make(map[string]string)
		if normList, ok := query["normalized"].([]interface{}); ok {
			for _, n := range normList {
				norm := n.(map[string]interface{})
				from := getString(norm["from"])
				to := getString(norm["to"])
				normalized[to] = from
			}
		}

		// Check each page in the response
		for _, pageData := range pages {
			page := pageData.(map[string]interface{})
			title := getString(page["title"])

			// Check if page exists (missing key indicates non-existence)
			_, missing := page["missing"]
			exists := !missing

			result[title] = exists

			// Also map the original (unnormalized) title if applicable
			if originalTitle, ok := normalized[title]; ok {
				result[originalTitle] = exists
			}
		}
	}

	// Set any titles we didn't get a response for as non-existent
	for _, title := range titles {
		if _, ok := result[title]; !ok {
			result[title] = false
		}
	}

	return result, nil
}

// UploadFile uploads a file to the wiki
func (c *Client) UploadFile(ctx context.Context, args UploadFileArgs) (UploadFileResult, error) {
	if args.Filename == "" {
		return UploadFileResult{}, fmt.Errorf("filename is required")
	}
	if args.FilePath == "" && args.FileURL == "" {
		return UploadFileResult{}, fmt.Errorf("either file_path or file_url is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return UploadFileResult{}, fmt.Errorf("authentication required for uploads: %w", err)
	}

	// Get CSRF token
	token, err := c.getCSRFToken(ctx)
	if err != nil {
		return UploadFileResult{}, fmt.Errorf("failed to get edit token: %w", err)
	}

	var result UploadFileResult

	// If URL provided, use URL upload
	if args.FileURL != "" {
		result, err = c.uploadFromURL(ctx, args, token)
	} else {
		// For local file upload, we need multipart form
		result, err = c.uploadFromFile(ctx, args, token)
	}

	// Log upload attempt (even if error occurred)
	if err != nil {
		c.logAudit(AuditEntry{
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			Operation:   AuditOpUpload,
			Title:       "File:" + args.Filename,
			ContentHash: hashContent(args.FileURL + args.FilePath), // Hash the source
			ContentSize: 0,
			Summary:     args.Comment,
			WikiURL:     c.config.BaseURL,
			Success:     false,
			Error:       err.Error(),
		})
		return result, err
	}

	// Log upload result
	c.logAudit(AuditEntry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Operation:   AuditOpUpload,
		Title:       "File:" + result.Filename,
		ContentHash: hashContent(args.FileURL + args.FilePath),
		ContentSize: result.Size,
		Summary:     args.Comment,
		WikiURL:     c.config.BaseURL,
		Success:     result.Success,
		Error: func() string {
			if !result.Success {
				return result.Message
			}
			return ""
		}(),
	})

	return result, nil
}

// uploadFromURL uploads a file from a URL
func (c *Client) uploadFromURL(ctx context.Context, args UploadFileArgs, token string) (UploadFileResult, error) {
	params := url.Values{}
	params.Set("action", "upload")
	params.Set("filename", args.Filename)
	params.Set("url", args.FileURL)
	params.Set("token", token)

	if args.Text != "" {
		params.Set("text", args.Text)
	}
	if args.Comment != "" {
		params.Set("comment", args.Comment)
	}
	if args.IgnoreWarnings {
		params.Set("ignorewarnings", "1")
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return UploadFileResult{}, err
	}

	return c.parseUploadResponse(resp, args.Filename)
}

// uploadFromFile uploads a local file using multipart form
func (c *Client) uploadFromFile(ctx context.Context, args UploadFileArgs, token string) (UploadFileResult, error) {
	// Read file
	fileData, err := c.readLocalFile(args.FilePath)
	if err != nil {
		return UploadFileResult{}, fmt.Errorf("failed to read file: %w", err)
	}

	// Create multipart request
	boundary := "----WikiUploadBoundary" + strconv.FormatInt(time.Now().UnixNano(), 36)

	var body strings.Builder
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"action\"\r\n\r\n")
	body.WriteString("upload\r\n")

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"format\"\r\n\r\n")
	body.WriteString("json\r\n")

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"filename\"\r\n\r\n")
	body.WriteString(args.Filename + "\r\n")

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"token\"\r\n\r\n")
	body.WriteString(token + "\r\n")

	if args.Text != "" {
		body.WriteString("--" + boundary + "\r\n")
		body.WriteString("Content-Disposition: form-data; name=\"text\"\r\n\r\n")
		body.WriteString(args.Text + "\r\n")
	}

	if args.Comment != "" {
		body.WriteString("--" + boundary + "\r\n")
		body.WriteString("Content-Disposition: form-data; name=\"comment\"\r\n\r\n")
		body.WriteString(args.Comment + "\r\n")
	}

	if args.IgnoreWarnings {
		body.WriteString("--" + boundary + "\r\n")
		body.WriteString("Content-Disposition: form-data; name=\"ignorewarnings\"\r\n\r\n")
		body.WriteString("1\r\n")
	}

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\n", args.Filename))
	body.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	body.Write(fileData)
	body.WriteString("\r\n")
	body.WriteString("--" + boundary + "--\r\n")

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL, strings.NewReader(body.String()))
	if err != nil {
		return UploadFileResult{}, err
	}

	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	// Use HTTP client to make request
	resp, err := c.httpClient.Do(req) // #nosec G704 -- URL is the configured wiki API endpoint, not user-controlled
	if err != nil {
		return UploadFileResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse JSON response
	var result map[string]interface{}
	if err := c.parseJSONResponse(resp, &result); err != nil {
		return UploadFileResult{}, err
	}

	return c.parseUploadResponse(result, args.Filename)
}

// readLocalFile reads a file from the local filesystem
func (c *Client) readLocalFile(path string) ([]byte, error) {
	// This is a placeholder - in a real implementation you'd read the file
	// For security, MCP servers typically don't have direct filesystem access
	// Instead, the file content should be passed directly or via URL
	return nil, fmt.Errorf("local file upload not supported - use file_url instead")
}

// parseUploadResponse parses the upload API response
func (c *Client) parseUploadResponse(resp map[string]interface{}, filename string) (UploadFileResult, error) {
	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		return UploadFileResult{
			Success:  false,
			Filename: filename,
			Message:  fmt.Sprintf("Upload failed: %s", errInfo["info"]),
		}, nil
	}

	upload, ok := resp["upload"].(map[string]interface{})
	if !ok {
		return UploadFileResult{
			Success:  false,
			Filename: filename,
			Message:  "Unexpected response format",
		}, nil
	}

	result := UploadFileResult{
		Filename: filename,
	}

	status := getString(upload["result"])
	switch status {
	case "Success":
		result.Success = true
		result.Message = "File uploaded successfully"
		if imageinfo, ok := upload["imageinfo"].(map[string]interface{}); ok {
			result.URL = getString(imageinfo["url"])
			result.Size = getInt(imageinfo["size"])
		}
	case "Warning":
		result.Success = false
		result.Message = "Upload has warnings - set ignore_warnings=true to proceed"
		if warnings, ok := upload["warnings"].(map[string]interface{}); ok {
			for k, v := range warnings {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", k, v))
			}
		}
	default:
		result.Success = false
		result.Message = fmt.Sprintf("Upload status: %s", status)
	}

	return result, nil
}

// parseJSONResponse parses a JSON response from http.Response
func (c *Client) parseJSONResponse(resp *http.Response, target interface{}) error {
	// Import would be needed: "encoding/json"
	// For now, return error indicating this needs implementation
	return fmt.Errorf("JSON parsing not implemented for multipart upload")
}

// getFileURL retrieves the download URL and type for a wiki file
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
		page := pageData.(map[string]interface{})

		// Check if file exists
		if _, missing := page["missing"]; missing {
			return "", "", fmt.Errorf("file '%s' does not exist", filename)
		}

		imageinfo, ok := page["imageinfo"].([]interface{})
		if !ok || len(imageinfo) == 0 {
			return "", "", fmt.Errorf("no file info available for '%s'", filename)
		}

		info := imageinfo[0].(map[string]interface{})
		fileURL := getString(info["url"])
		mimeType := getString(info["mime"])

		if fileURL == "" {
			return "", "", fmt.Errorf("no download URL for '%s'", filename)
		}

		// Extract file type from mime or filename
		fileType := mimeType
		if mimeType == "application/pdf" {
			fileType = "pdf"
		} else if strings.HasPrefix(mimeType, "text/") {
			fileType = strings.TrimPrefix(mimeType, "text/")
		}

		return fileURL, fileType, nil
	}

	return "", "", fmt.Errorf("file '%s' not found", filename)
}

// downloadFile downloads a file from the given URL
func (c *Client) downloadFile(ctx context.Context, fileURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req) // #nosec G704 -- fileURL validated by validateFileURL in security.go before reaching here
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
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
