package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// GetPage retrieves page content
// Handles title normalization automatically (case, underscores, whitespace)
func (c *Client) GetPage(ctx context.Context, args GetPageArgs) (PageContent, error) {
	if args.Title == "" {
		return PageContent{}, fmt.Errorf("title is required")
	}

	// Normalize the title to handle case variations
	// MediaWiki normalizes titles internally, but we do it here for better cache hits
	// and to avoid duplicate API calls for "Module overview" vs "Module Overview"
	normalizedTitle := normalizePageTitle(args.Title)

	// Check cache with normalized title
	cacheKey := fmt.Sprintf("page_content:%s", normalizedTitle)
	if cached, ok := c.getCached(cacheKey); ok {
		return cached.(PageContent), nil
	}

	format := args.Format
	if format == "" {
		format = "wikitext"
	}

	var result PageContent
	var err error

	if format == "html" {
		result, err = c.getPageHTML(ctx, normalizedTitle)
	} else {
		result, err = c.getPageWikitext(ctx, normalizedTitle)
	}

	if err != nil {
		return PageContent{}, err
	}

	// Cache the result using the canonical title from API response
	c.setCache(cacheKey, result, "page_content")

	// Also cache under the original title if different (for future lookups)
	if args.Title != normalizedTitle {
		originalCacheKey := fmt.Sprintf("page_content:%s", args.Title)
		c.setCache(originalCacheKey, result, "page_content")
	}

	return result, nil
}

func (c *Client) getPageWikitext(ctx context.Context, title string) (PageContent, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return PageContent{}, fmt.Errorf("authentication required: %w (configure MEDIAWIKI_USERNAME and MEDIAWIKI_PASSWORD)", err)
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", title)
	params.Set("prop", "revisions")
	params.Set("rvprop", "content|ids|timestamp")
	params.Set("rvslots", "main")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return PageContent{}, fmt.Errorf("API request failed: %w", err)
	}

	// Safely extract query object
	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return PageContent{}, fmt.Errorf("unexpected API response: missing 'query' object. This may indicate authentication is required for reading pages")
	}

	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return PageContent{}, fmt.Errorf("unexpected API response: missing 'pages' object")
	}

	for pageID, pageData := range pages {
		page, ok := pageData.(map[string]interface{})
		if !ok {
			continue
		}
		return buildWikitextPageContent(page, pageID, title)
	}

	return PageContent{}, fmt.Errorf("page '%s' not found in API response", title)
}

// buildWikitextPageContent converts a single wikitext page object into a
// PageContent, returning descriptive errors for each response-shape failure.
func buildWikitextPageContent(page map[string]interface{}, pageID, title string) (PageContent, error) {
	if _, missing := page["missing"]; missing {
		return PageContent{}, fmt.Errorf("page '%s' does not exist. Try using mediawiki_resolve_title to find the correct page name", title)
	}

	content, rev, err := extractWikitextRevision(page, title)
	if err != nil {
		return PageContent{}, err
	}

	truncated := false
	if len(content) > CharacterLimit {
		content, truncated = truncateContent(content, CharacterLimit)
	}

	id, _ := strconv.Atoi(pageID)
	pageTitle, _ := page["title"].(string)
	if pageTitle == "" {
		pageTitle = title
	}
	revID := 0
	if rid, ok := rev["revid"].(float64); ok {
		revID = int(rid)
	}
	timestamp, _ := rev["timestamp"].(string)

	result := PageContent{
		Title:     pageTitle,
		PageID:    id,
		Content:   content,
		Format:    "wikitext",
		Revision:  revID,
		Timestamp: timestamp,
		Truncated: truncated,
	}
	if truncated {
		result.Message = "Content was truncated due to size limits. Consider fetching specific sections."
	}
	return result, nil
}

// extractWikitextRevision walks revisions[0].slots.main and returns the content
// string and the revision map, with descriptive errors for each shape failure.
func extractWikitextRevision(page map[string]interface{}, title string) (content string, rev map[string]interface{}, err error) {
	revisions, ok := page["revisions"].([]interface{})
	if !ok || len(revisions) == 0 {
		return "", nil, fmt.Errorf("no revisions found for page '%s'. The page may be empty or protected", title)
	}
	rev, ok = revisions[0].(map[string]interface{})
	if !ok {
		return "", nil, fmt.Errorf("invalid revision data for page '%s'", title)
	}
	slots, ok := rev["slots"].(map[string]interface{})
	if !ok {
		return "", nil, fmt.Errorf("invalid slots data for page '%s'. This may be a MediaWiki version compatibility issue", title)
	}
	main, ok := slots["main"].(map[string]interface{})
	if !ok {
		return "", nil, fmt.Errorf("invalid main slot data for page '%s'", title)
	}

	// MediaWiki API returns content under "*" key, not "content"; some versions
	// use "content" instead.
	content, ok = main["*"].(string)
	if !ok {
		content, ok = main["content"].(string)
		if !ok {
			return "", nil, fmt.Errorf("page '%s' has no content or content is not text", title)
		}
	}
	return content, rev, nil
}

func (c *Client) getPageHTML(ctx context.Context, title string) (PageContent, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return PageContent{}, fmt.Errorf("authentication required: %w (configure MEDIAWIKI_USERNAME and MEDIAWIKI_PASSWORD)", err)
	}

	params := url.Values{}
	params.Set("action", "parse")
	params.Set("page", title)
	params.Set("prop", "text|revid")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return PageContent{}, fmt.Errorf("API request failed: %w", err)
	}

	parse, ok := resp["parse"].(map[string]interface{})
	if !ok {
		return PageContent{}, fmt.Errorf("unexpected API response: missing 'parse' object. Page '%s' may not exist or authentication is required", title)
	}

	text, ok := parse["text"].(map[string]interface{})
	if !ok {
		return PageContent{}, fmt.Errorf("unexpected API response: missing 'text' object for page '%s'", title)
	}

	content, ok := text["*"].(string)
	if !ok {
		return PageContent{}, fmt.Errorf("page '%s' has no HTML content", title)
	}

	// Sanitize HTML to prevent XSS
	content = sanitizeHTML(content)

	truncated := false
	if len(content) > CharacterLimit {
		content, truncated = truncateContent(content, CharacterLimit)
	}

	result := PageContent{
		Title:     htmlPageTitle(parse, title),
		PageID:    intField(parse, "pageid"),
		Content:   content,
		Format:    "html",
		Revision:  intField(parse, "revid"),
		Truncated: truncated,
	}
	if truncated {
		result.Message = "Content was truncated due to size limits."
	}
	return result, nil
}

// htmlPageTitle returns the parse response's title, falling back to the
// requested title.
func htmlPageTitle(parse map[string]interface{}, fallback string) string {
	if t, _ := parse["title"].(string); t != "" {
		return t
	}
	return fallback
}

// intField reads a float64-encoded JSON number field as an int (0 if absent).
func intField(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

// Parse parses wikitext and returns HTML
func (c *Client) Parse(ctx context.Context, args ParseArgs) (ParseResult, error) {
	if args.Wikitext == "" {
		return ParseResult{}, fmt.Errorf("wikitext is required")
	}

	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return ParseResult{}, err
	}

	params := url.Values{}
	params.Set("action", "parse")
	params.Set("text", args.Wikitext)
	params.Set("contentmodel", "wikitext")
	params.Set("prop", "text|categories|links")

	if args.Title != "" {
		params.Set("title", args.Title)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return ParseResult{}, err
	}

	parse, ok := resp["parse"].(map[string]interface{})
	if !ok {
		return ParseResult{}, fmt.Errorf("unexpected API response: missing 'parse' object")
	}
	text, ok := parse["text"].(map[string]interface{})
	if !ok {
		return ParseResult{}, fmt.Errorf("unexpected API response: missing 'text' object")
	}
	htmlContent := getString(text["*"])

	// Sanitize HTML to prevent XSS
	htmlContent = sanitizeHTML(htmlContent)

	truncated := false
	if len(htmlContent) > CharacterLimit {
		htmlContent, truncated = truncateContent(htmlContent, CharacterLimit)
	}

	result := ParseResult{
		HTML:       htmlContent,
		Truncated:  truncated,
		Categories: extractStarValues(parse["categories"]),
		Links:      extractStarValues(parse["links"]),
	}
	if truncated {
		result.Message = "Content was truncated due to size limits."
	}
	return result, nil
}

// extractStarValues pulls the "*" string field from each map entry in a
// MediaWiki list (used for categories and links in parse responses).
func extractStarValues(raw interface{}) []string {
	entries, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	var values []string
	for _, e := range entries {
		if m, ok := e.(map[string]interface{}); ok {
			values = append(values, getString(m["*"]))
		}
	}
	return values
}

// GetPageSummary returns the lead section and key metadata for a page.
// This is much lighter than GetPage for large pages when you only need an overview.
func (c *Client) GetPageSummary(ctx context.Context, args GetPageSummaryArgs) (PageSummaryResult, error) {
	if args.Title == "" {
		return PageSummaryResult{}, fmt.Errorf("title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return PageSummaryResult{}, err
	}

	normalizedTitle := normalizePageTitle(args.Title)

	// Get section 0 (lead/intro) content
	leadResult, err := c.getSectionContent(ctx, normalizedTitle, 0, "wikitext")
	if err != nil {
		return PageSummaryResult{}, fmt.Errorf("failed to get lead section: %w", err)
	}

	// Get page info (metadata)
	info, err := c.GetPageInfo(ctx, PageInfoArgs{Title: normalizedTitle})
	if err != nil {
		return PageSummaryResult{}, fmt.Errorf("failed to get page info: %w", err)
	}

	// Get section list for overview
	sections, err := c.GetSections(ctx, GetSectionsArgs{Title: normalizedTitle})
	sectionNames := make([]string, 0)
	if err == nil {
		for _, s := range sections.Sections {
			sectionNames = append(sectionNames, s.Title)
		}
	}

	result := PageSummaryResult{
		Title:        info.Title,
		PageID:       info.PageID,
		LeadContent:  leadResult.SectionContent,
		Format:       "wikitext",
		Length:       info.Length,
		Revision:     info.LastRevision,
		LastEdited:   info.Touched,
		Categories:   info.Categories,
		SectionCount: len(sectionNames),
		Sections:     sectionNames,
		Redirect:     info.Redirect,
		RedirectTo:   info.RedirectTo,
	}

	if info.Length > CharacterLimit {
		result.Message = "This is a large page. Use mediawiki_get_sections with a section number to read specific sections."
	}

	return result, nil
}
