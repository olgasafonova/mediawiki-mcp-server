package wiki

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Search searches for pages matching the query
func (c *Client) Search(ctx context.Context, args SearchArgs) (SearchResult, error) {
	if args.Query == "" {
		return SearchResult{}, fmt.Errorf("query is required")
	}

	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return SearchResult{}, err
	}

	limit := normalizeLimit(args.Limit, 20, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "search")
	params.Set("srsearch", args.Query)
	params.Set("srlimit", strconv.Itoa(limit))
	params.Set("srprop", "snippet|size|timestamp")

	if args.Offset > 0 {
		params.Set("sroffset", strconv.Itoa(args.Offset))
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return SearchResult{}, err
	}

	query := resp["query"].(map[string]interface{})
	searchInfo := query["searchinfo"].(map[string]interface{})
	totalHits := int(searchInfo["totalhits"].(float64))

	searchResults := query["search"].([]interface{})
	results := make([]SearchHit, 0, len(searchResults))

	for _, sr := range searchResults {
		item := sr.(map[string]interface{})
		hit := SearchHit{
			PageID:  int(item["pageid"].(float64)),
			Title:   item["title"].(string),
			Snippet: stripHTMLTags(item["snippet"].(string)),
			Size:    int(item["size"].(float64)),
		}
		results = append(results, hit)
	}

	result := SearchResult{
		Query:     args.Query,
		TotalHits: totalHits,
		Results:   results,
		HasMore:   args.Offset+len(results) < totalHits,
	}

	if result.HasMore {
		result.NextOffset = args.Offset + len(results)
	}

	return result, nil
}

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

		// Check if page exists
		if _, missing := page["missing"]; missing {
			// Try to suggest similar pages
			return PageContent{}, fmt.Errorf("page '%s' does not exist. Try using mediawiki_resolve_title to find the correct page name", title)
		}

		revisions, ok := page["revisions"].([]interface{})
		if !ok || len(revisions) == 0 {
			return PageContent{}, fmt.Errorf("no revisions found for page '%s'. The page may be empty or protected", title)
		}

		rev, ok := revisions[0].(map[string]interface{})
		if !ok {
			return PageContent{}, fmt.Errorf("invalid revision data for page '%s'", title)
		}

		slots, ok := rev["slots"].(map[string]interface{})
		if !ok {
			return PageContent{}, fmt.Errorf("invalid slots data for page '%s'. This may be a MediaWiki version compatibility issue", title)
		}

		main, ok := slots["main"].(map[string]interface{})
		if !ok {
			return PageContent{}, fmt.Errorf("invalid main slot data for page '%s'", title)
		}

		// MediaWiki API returns content under "*" key, not "content"
		content, ok := main["*"].(string)
		if !ok {
			// Some versions might use "content" instead
			content, ok = main["content"].(string)
			if !ok {
				return PageContent{}, fmt.Errorf("page '%s' has no content or content is not text", title)
			}
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

	return PageContent{}, fmt.Errorf("page '%s' not found in API response", title)
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

	pageTitle, _ := parse["title"].(string)
	if pageTitle == "" {
		pageTitle = title
	}

	pageID := 0
	if pid, ok := parse["pageid"].(float64); ok {
		pageID = int(pid)
	}

	revID := 0
	if rid, ok := parse["revid"].(float64); ok {
		revID = int(rid)
	}

	result := PageContent{
		Title:     pageTitle,
		PageID:    pageID,
		Content:   content,
		Format:    "html",
		Revision:  revID,
		Truncated: truncated,
	}

	if truncated {
		result.Message = "Content was truncated due to size limits."
	}

	return result, nil
}

// ListPages lists pages in the wiki
func (c *Client) ListPages(ctx context.Context, args ListPagesArgs) (ListPagesResult, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return ListPagesResult{}, err
	}

	limit := normalizeLimit(args.Limit, DefaultLimit, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "allpages")
	params.Set("aplimit", strconv.Itoa(limit))

	if args.Prefix != "" {
		params.Set("apprefix", args.Prefix)
	}

	if args.Namespace >= 0 {
		params.Set("apnamespace", strconv.Itoa(args.Namespace))
	}

	if args.ContinueFrom != "" {
		params.Set("apcontinue", args.ContinueFrom)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return ListPagesResult{}, err
	}

	query := resp["query"].(map[string]interface{})
	allpages := query["allpages"].([]interface{})

	pages := make([]PageSummary, 0, len(allpages))
	for _, p := range allpages {
		page := p.(map[string]interface{})
		pages = append(pages, PageSummary{
			PageID: int(page["pageid"].(float64)),
			Title:  page["title"].(string),
		})
	}

	result := ListPagesResult{
		Pages:      pages,
		TotalCount: len(pages),
	}

	// Check for continuation
	if cont, ok := resp["continue"].(map[string]interface{}); ok {
		if apcontinue, ok := cont["apcontinue"].(string); ok {
			result.HasMore = true
			result.ContinueFrom = apcontinue
		}
	}

	return result, nil
}

// ListCategories lists categories in the wiki
func (c *Client) ListCategories(ctx context.Context, args ListCategoriesArgs) (ListCategoriesResult, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return ListCategoriesResult{}, err
	}

	limit := normalizeLimit(args.Limit, DefaultLimit, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "allcategories")
	params.Set("aclimit", strconv.Itoa(limit))
	params.Set("acprop", "size")

	if args.Prefix != "" {
		params.Set("acprefix", args.Prefix)
	}

	if args.ContinueFrom != "" {
		params.Set("accontinue", args.ContinueFrom)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return ListCategoriesResult{}, err
	}

	query := resp["query"].(map[string]interface{})
	allcats := query["allcategories"].([]interface{})

	categories := make([]CategoryInfo, 0, len(allcats))
	for _, cat := range allcats {
		c := cat.(map[string]interface{})
		members := 0
		if size, ok := c["size"].(float64); ok {
			members = int(size)
		}
		categories = append(categories, CategoryInfo{
			Title:   c["*"].(string),
			Members: members,
		})
	}

	result := ListCategoriesResult{
		Categories: categories,
	}

	// Check for continuation
	if cont, ok := resp["continue"].(map[string]interface{}); ok {
		if accontinue, ok := cont["accontinue"].(string); ok {
			result.HasMore = true
			result.ContinueFrom = accontinue
		}
	}

	return result, nil
}

// GetCategoryMembers gets pages in a category
func (c *Client) GetCategoryMembers(ctx context.Context, args CategoryMembersArgs) (CategoryMembersResult, error) {
	if args.Category == "" {
		return CategoryMembersResult{}, fmt.Errorf("category is required")
	}

	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CategoryMembersResult{}, err
	}

	category := normalizeCategoryName(args.Category)
	limit := normalizeLimit(args.Limit, DefaultLimit, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "categorymembers")
	params.Set("cmtitle", category)
	params.Set("cmlimit", strconv.Itoa(limit))

	if args.Type != "" {
		params.Set("cmtype", args.Type)
	}

	if args.ContinueFrom != "" {
		params.Set("cmcontinue", args.ContinueFrom)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return CategoryMembersResult{}, err
	}

	query := resp["query"].(map[string]interface{})
	members := query["categorymembers"].([]interface{})

	pages := make([]PageSummary, 0, len(members))
	for _, m := range members {
		member := m.(map[string]interface{})
		pages = append(pages, PageSummary{
			PageID: int(member["pageid"].(float64)),
			Title:  member["title"].(string),
		})
	}

	result := CategoryMembersResult{
		Category: category,
		Members:  pages,
	}

	// Check for continuation
	if cont, ok := resp["continue"].(map[string]interface{}); ok {
		if cmcontinue, ok := cont["cmcontinue"].(string); ok {
			result.HasMore = true
			result.ContinueFrom = cmcontinue
		}
	}

	return result, nil
}

// GetPageInfo gets metadata about a page
// Handles title normalization automatically
func (c *Client) GetPageInfo(ctx context.Context, args PageInfoArgs) (PageInfo, error) {
	if args.Title == "" {
		return PageInfo{}, fmt.Errorf("title is required")
	}

	// Normalize the title for consistent lookups
	normalizedTitle := normalizePageTitle(args.Title)

	// Check cache first
	cacheKey := fmt.Sprintf("page_info:%s", normalizedTitle)
	if cached, ok := c.getCached(cacheKey); ok {
		return cached.(PageInfo), nil
	}

	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return PageInfo{}, err
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", normalizedTitle)
	params.Set("prop", "info|categories|links")
	params.Set("inprop", "protection|url")
	params.Set("cllimit", "50")
	params.Set("pllimit", "max")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return PageInfo{}, err
	}

	query := resp["query"].(map[string]interface{})
	pages := query["pages"].(map[string]interface{})

	for _, pageData := range pages {
		page := pageData.(map[string]interface{})

		// Check if page exists
		if _, missing := page["missing"]; missing {
			return PageInfo{
				Title:  args.Title,
				Exists: false,
			}, nil
		}

		info := PageInfo{
			Title:        page["title"].(string),
			PageID:       int(page["pageid"].(float64)),
			Namespace:    int(page["ns"].(float64)),
			ContentModel: getString(page, "contentmodel"),
			PageLanguage: getString(page, "pagelanguage"),
			Length:       int(page["length"].(float64)),
			Touched:      getString(page, "touched"),
			LastRevision: int(page["lastrevid"].(float64)),
			Exists:       true,
		}

		// Categories
		if cats, ok := page["categories"].([]interface{}); ok {
			for _, cat := range cats {
				c := cat.(map[string]interface{})
				info.Categories = append(info.Categories, c["title"].(string))
			}
		}

		// Links count
		if links, ok := page["links"].([]interface{}); ok {
			info.Links = len(links)
		}

		// Redirect
		if _, isRedirect := page["redirect"]; isRedirect {
			info.Redirect = true
		}

		// Protection
		if protection, ok := page["protection"].([]interface{}); ok {
			for _, p := range protection {
				prot := p.(map[string]interface{})
				info.Protection = append(info.Protection, fmt.Sprintf("%s: %s", prot["type"], prot["level"]))
			}
		}

		// Cache the result
		c.setCache(cacheKey, info, "page_info")

		return info, nil
	}

	return PageInfo{}, fmt.Errorf("page '%s' not found", normalizedTitle)
}

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
	result := getString(edit, "result")

	if result != "Success" {
		return EditResult{
			Success: false,
			Title:   args.Title,
			Message: fmt.Sprintf("Edit failed: %s", result),
		}, nil
	}

	editResult := EditResult{
		Success:    true,
		Title:      getString(edit, "title"),
		PageID:     int(edit["pageid"].(float64)),
		RevisionID: int(edit["newrevid"].(float64)),
		NewPage:    edit["new"] != nil,
		Message:    "Page edited successfully",
	}

	if editResult.NewPage {
		editResult.Message = "Page created successfully"
	}

	return editResult, nil
}

// GetRecentChanges gets recent changes
func (c *Client) GetRecentChanges(ctx context.Context, args RecentChangesArgs) (RecentChangesResult, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return RecentChangesResult{}, err
	}

	limit := normalizeLimit(args.Limit, DefaultLimit, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "recentchanges")
	params.Set("rclimit", strconv.Itoa(limit))
	params.Set("rcprop", "title|ids|sizes|flags|user|timestamp|comment")

	if args.Namespace >= 0 {
		params.Set("rcnamespace", strconv.Itoa(args.Namespace))
	}

	if args.Type != "" {
		params.Set("rctype", args.Type)
	}

	if args.ContinueFrom != "" {
		params.Set("rccontinue", args.ContinueFrom)
	}

	if args.Start != "" {
		params.Set("rcstart", args.Start)
	}

	if args.End != "" {
		params.Set("rcend", args.End)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return RecentChangesResult{}, err
	}

	query := resp["query"].(map[string]interface{})
	rcList := query["recentchanges"].([]interface{})

	changes := make([]RecentChange, 0, len(rcList))
	for _, rc := range rcList {
		change := rc.(map[string]interface{})

		ts, _ := time.Parse(time.RFC3339, change["timestamp"].(string))

		changes = append(changes, RecentChange{
			Type:       getString(change, "type"),
			Title:      getString(change, "title"),
			PageID:     getInt(change, "pageid"),
			RevisionID: getInt(change, "revid"),
			User:       getString(change, "user"),
			Timestamp:  ts,
			Comment:    getString(change, "comment"),
			SizeDiff:   getInt(change, "newlen") - getInt(change, "oldlen"),
			New:        change["new"] != nil,
			Minor:      change["minor"] != nil,
			Bot:        change["bot"] != nil,
		})
	}

	result := RecentChangesResult{
		Changes: changes,
	}

	// Check for continuation
	if cont, ok := resp["continue"].(map[string]interface{}); ok {
		if rccontinue, ok := cont["rccontinue"].(string); ok {
			result.HasMore = true
			result.ContinueFrom = rccontinue
		}
	}

	return result, nil
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

	parse := resp["parse"].(map[string]interface{})
	text := parse["text"].(map[string]interface{})
	htmlContent := text["*"].(string)

	// Sanitize HTML to prevent XSS
	htmlContent = sanitizeHTML(htmlContent)

	truncated := false
	if len(htmlContent) > CharacterLimit {
		htmlContent, truncated = truncateContent(htmlContent, CharacterLimit)
	}

	result := ParseResult{
		HTML:      htmlContent,
		Truncated: truncated,
	}

	// Categories
	if cats, ok := parse["categories"].([]interface{}); ok {
		for _, cat := range cats {
			c := cat.(map[string]interface{})
			result.Categories = append(result.Categories, c["*"].(string))
		}
	}

	// Links
	if links, ok := parse["links"].([]interface{}); ok {
		for _, link := range links {
			l := link.(map[string]interface{})
			result.Links = append(result.Links, l["*"].(string))
		}
	}

	if truncated {
		result.Message = "Content was truncated due to size limits."
	}

	return result, nil
}

// GetWikiInfo gets information about the wiki
func (c *Client) GetWikiInfo(ctx context.Context, args WikiInfoArgs) (WikiInfo, error) {
	// Check cache first
	cacheKey := "wiki_info"
	if cached, ok := c.getCached(cacheKey); ok {
		return cached.(WikiInfo), nil
	}

	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return WikiInfo{}, err
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("meta", "siteinfo")
	params.Set("siprop", "general|statistics")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return WikiInfo{}, err
	}

	query := resp["query"].(map[string]interface{})
	general := query["general"].(map[string]interface{})

	info := WikiInfo{
		SiteName:    getString(general, "sitename"),
		MainPage:    getString(general, "mainpage"),
		Base:        getString(general, "base"),
		Generator:   getString(general, "generator"),
		PHPVersion:  getString(general, "phpversion"),
		Language:    getString(general, "lang"),
		ArticlePath: getString(general, "articlepath"),
		Server:      getString(general, "server"),
		Timezone:    getString(general, "timezone"),
		WriteAPI:    general["writeapi"] != nil,
	}

	// Statistics
	if stats, ok := query["statistics"].(map[string]interface{}); ok {
		info.Statistics = &WikiStats{
			Pages:       getInt(stats, "pages"),
			Articles:    getInt(stats, "articles"),
			Edits:       getInt(stats, "edits"),
			Images:      getInt(stats, "images"),
			Users:       getInt(stats, "users"),
			ActiveUsers: getInt(stats, "activeusers"),
			Admins:      getInt(stats, "admins"),
		}
	}

	// Cache the result
	c.setCache(cacheKey, info, "wiki_info")

	return info, nil
}

// Helper functions

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

func stripHTMLTags(s string) string {
	// Decode HTML entities
	s = html.UnescapeString(s)
	// Remove HTML tags
	s = htmlTagRegex.ReplaceAllString(s, "")
	// Clean up whitespace
	s = strings.TrimSpace(s)
	return s
}

// GetExternalLinks retrieves external links from a wiki page
func (c *Client) GetExternalLinks(ctx context.Context, args GetExternalLinksArgs) (ExternalLinksResult, error) {
	if args.Title == "" {
		return ExternalLinksResult{}, fmt.Errorf("title is required")
	}

	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return ExternalLinksResult{}, err
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", args.Title)
	params.Set("prop", "extlinks")
	params.Set("ellimit", "500")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return ExternalLinksResult{}, err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return ExternalLinksResult{}, fmt.Errorf("unexpected response format")
	}

	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return ExternalLinksResult{}, fmt.Errorf("no pages in response")
	}

	links := make([]ExternalLink, 0) // Initialize as empty slice, not nil, to avoid JSON null
	var pageTitle string

	for _, pageData := range pages {
		page := pageData.(map[string]interface{})

		// Check if page exists
		if _, missing := page["missing"]; missing {
			return ExternalLinksResult{}, fmt.Errorf("page '%s' does not exist", args.Title)
		}

		pageTitle = page["title"].(string)

		if extlinks, ok := page["extlinks"].([]interface{}); ok {
			for _, el := range extlinks {
				link := el.(map[string]interface{})
				linkURL := getString(link, "*")
				if linkURL == "" {
					linkURL = getString(link, "url")
				}
				if linkURL != "" {
					protocol := ""
					if u, err := url.Parse(linkURL); err == nil {
						protocol = u.Scheme
					}
					links = append(links, ExternalLink{
						URL:      linkURL,
						Protocol: protocol,
					})
				}
			}
		}
		break
	}

	return ExternalLinksResult{
		Title: pageTitle,
		Links: links,
		Count: len(links),
	}, nil
}

// GetExternalLinksBatch retrieves external links from multiple wiki pages
// Optimized to process pages in parallel using goroutines
func (c *Client) GetExternalLinksBatch(ctx context.Context, args GetExternalLinksBatchArgs) (ExternalLinksBatchResult, error) {
	if len(args.Titles) == 0 {
		return ExternalLinksBatchResult{}, fmt.Errorf("at least one title is required")
	}

	// Limit batch size to prevent overwhelming the API
	maxBatch := 10
	if len(args.Titles) > maxBatch {
		args.Titles = args.Titles[:maxBatch]
	}

	// Process pages in parallel
	type pageResult struct {
		index int
		data  PageExternalLinks
	}

	results := make(chan pageResult, len(args.Titles))
	var wg sync.WaitGroup

	for i, title := range args.Titles {
		wg.Add(1)
		go func(idx int, t string) {
			defer wg.Done()

			pageLinks, err := c.GetExternalLinks(ctx, GetExternalLinksArgs{Title: t})
			if err != nil {
				results <- pageResult{
					index: idx,
					data: PageExternalLinks{
						Title: t,
						Links: make([]ExternalLink, 0),
						Error: err.Error(),
					},
				}
				return
			}

			results <- pageResult{
				index: idx,
				data: PageExternalLinks{
					Title: pageLinks.Title,
					Links: pageLinks.Links,
					Count: pageLinks.Count,
				},
			}
		}(i, title)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results maintaining order
	pageResults := make([]PageExternalLinks, len(args.Titles))
	totalLinks := 0

	for pr := range results {
		pageResults[pr.index] = pr.data
		totalLinks += pr.data.Count
	}

	return ExternalLinksBatchResult{
		Pages:      pageResults,
		TotalLinks: totalLinks,
	}, nil
}

// CheckLinks checks if URLs are accessible (broken link detection)
func (c *Client) CheckLinks(ctx context.Context, args CheckLinksArgs) (CheckLinksResult, error) {
	if len(args.URLs) == 0 {
		return CheckLinksResult{}, fmt.Errorf("at least one URL is required")
	}

	// Limit URLs to prevent abuse
	maxURLs := 20
	if len(args.URLs) > maxURLs {
		args.URLs = args.URLs[:maxURLs]
	}

	// Set timeout (default 10s, max 30s)
	timeout := 10
	if args.Timeout > 0 && args.Timeout <= 30 {
		timeout = args.Timeout
	}

	httpClient := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow up to 5 redirects
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	result := CheckLinksResult{
		Results:    make([]LinkCheckResult, 0, len(args.URLs)),
		TotalLinks: len(args.URLs),
	}

	// Use a semaphore to limit concurrent checks
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, linkURL := range args.URLs {
		wg.Add(1)
		go func(checkURL string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			linkResult := LinkCheckResult{URL: checkURL}

			// Validate URL format
			parsedURL, err := url.Parse(checkURL)
			if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
				linkResult.Status = "invalid_url"
				linkResult.Error = "Invalid URL format"
				linkResult.Broken = true
			} else {
				// Make HEAD request first (faster)
				req, _ := http.NewRequestWithContext(ctx, "HEAD", checkURL, nil)
				req.Header.Set("User-Agent", "MediaWiki-MCP-LinkChecker/1.0")

				resp, err := httpClient.Do(req)
				if err != nil {
					// Try GET if HEAD fails
					req, _ = http.NewRequestWithContext(ctx, "GET", checkURL, nil)
					req.Header.Set("User-Agent", "MediaWiki-MCP-LinkChecker/1.0")
					resp, err = httpClient.Do(req)
				}

				if err != nil {
					linkResult.Status = "error"
					linkResult.Error = err.Error()
					linkResult.Broken = true
				} else {
					_ = resp.Body.Close() // Error ignored; we only need the status
					linkResult.StatusCode = resp.StatusCode
					linkResult.Status = resp.Status

					// Consider 4xx and 5xx as broken (except 403 which might be access denied)
					if resp.StatusCode >= 400 {
						linkResult.Broken = true
					}
				}
			}

			mu.Lock()
			result.Results = append(result.Results, linkResult)
			if linkResult.Broken {
				result.BrokenCount++
			} else {
				result.ValidCount++
			}
			mu.Unlock()
		}(linkURL)
	}

	wg.Wait()
	return result, nil
}

// CheckTerminology checks pages for terminology inconsistencies based on a wiki glossary
func (c *Client) CheckTerminology(ctx context.Context, args CheckTerminologyArgs) (CheckTerminologyResult, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CheckTerminologyResult{}, err
	}

	// Default glossary page
	glossaryPage := args.GlossaryPage
	if glossaryPage == "" {
		glossaryPage = "Brand Terminology Glossary"
	}

	// Load glossary from wiki
	glossary, err := c.loadGlossary(ctx, glossaryPage)
	if err != nil {
		return CheckTerminologyResult{}, fmt.Errorf("failed to load glossary from '%s': %w", glossaryPage, err)
	}

	if len(glossary) == 0 {
		return CheckTerminologyResult{}, fmt.Errorf("no terms found in glossary page '%s'", glossaryPage)
	}

	// Get pages to check
	var pagesToCheck []string
	limit := normalizeLimit(args.Limit, 10, 50)

	if len(args.Pages) > 0 {
		pagesToCheck = args.Pages
		if len(pagesToCheck) > limit {
			pagesToCheck = pagesToCheck[:limit]
		}
	} else if args.Category != "" {
		// Get pages from category
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
			Category: args.Category,
			Limit:    limit,
		})
		if err != nil {
			return CheckTerminologyResult{}, fmt.Errorf("failed to get category members: %w", err)
		}
		for _, p := range catResult.Members {
			pagesToCheck = append(pagesToCheck, p.Title)
		}
	} else {
		return CheckTerminologyResult{}, fmt.Errorf("either 'pages' or 'category' must be specified")
	}

	result := CheckTerminologyResult{
		GlossaryPage: glossaryPage,
		TermsLoaded:  len(glossary),
		Pages:        make([]PageTerminologyResult, 0, len(pagesToCheck)),
	}

	// Check each page
	for _, pageTitle := range pagesToCheck {
		pageResult := c.checkPageTerminology(ctx, pageTitle, glossary)
		result.Pages = append(result.Pages, pageResult)
		result.IssuesFound += pageResult.IssueCount
	}

	result.PagesChecked = len(result.Pages)
	return result, nil
}

// loadGlossary parses a wiki table to extract glossary terms
func (c *Client) loadGlossary(ctx context.Context, glossaryPage string) ([]GlossaryTerm, error) {
	page, err := c.GetPage(ctx, GetPageArgs{Title: glossaryPage, Format: "wikitext"})
	if err != nil {
		return nil, err
	}

	return parseWikiTableGlossary(page.Content), nil
}

// parseWikiTableGlossary extracts terms from wikitable format
func parseWikiTableGlossary(content string) []GlossaryTerm {
	var terms []GlossaryTerm

	// Match wiki tables with class containing "mcp-glossary" or any wikitable
	// Format: {| class="wikitable..." ... |}
	tableRegex := regexp.MustCompile(`(?s)\{\|[^\n]*class="[^"]*(?:mcp-glossary|wikitable)[^"]*"[^\n]*\n(.*?)\|\}`)
	tables := tableRegex.FindAllStringSubmatch(content, -1)

	for _, table := range tables {
		if len(table) < 2 {
			continue
		}

		tableContent := table[1]

		// Split into rows (|-) and process each
		rows := strings.Split(tableContent, "|-")
		for _, row := range rows {
			row = strings.TrimSpace(row)
			if row == "" || strings.HasPrefix(row, "!") {
				// Skip empty rows and header rows
				continue
			}

			// Parse cells (|| separator or | at line start)
			cells := parseTableRow(row)
			if len(cells) >= 2 {
				term := GlossaryTerm{
					Incorrect: strings.TrimSpace(cells[0]),
					Correct:   strings.TrimSpace(cells[1]),
				}

				// Skip if incorrect is empty or equals correct
				if term.Incorrect == "" || term.Incorrect == term.Correct {
					continue
				}

				if len(cells) >= 3 {
					term.Pattern = strings.TrimSpace(cells[2])
				}
				if len(cells) >= 4 {
					term.Notes = strings.TrimSpace(cells[3])
				}
				terms = append(terms, term)
			}
		}
	}

	return terms
}

// parseTableRow extracts cells from a wiki table row
func parseTableRow(row string) []string {
	var cells []string
	lines := strings.Split(row, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}

		// Remove leading | if present
		line = strings.TrimPrefix(line, "|")

		// Split by || for multiple cells on one line
		parts := strings.Split(line, "||")
		for _, part := range parts {
			cell := strings.TrimSpace(part)
			if cell != "" {
				cells = append(cells, cell)
			}
		}
	}

	return cells
}

// checkPageTerminology checks a single page against the glossary
func (c *Client) checkPageTerminology(ctx context.Context, title string, glossary []GlossaryTerm) PageTerminologyResult {
	result := PageTerminologyResult{
		Title:  title,
		Issues: make([]TerminologyIssue, 0),
	}

	page, err := c.GetPage(ctx, GetPageArgs{Title: title, Format: "wikitext"})
	if err != nil {
		result.Error = err.Error()
		return result
	}

	lines := strings.Split(page.Content, "\n")

	for lineNum, line := range lines {
		for _, term := range glossary {
			// Use regex pattern if specified, otherwise literal match
			var re *regexp.Regexp
			var err error

			if term.Pattern != "" {
				re, err = regexp.Compile("(?i)" + term.Pattern)
			} else {
				// Escape special regex characters and do case-insensitive match
				escaped := regexp.QuoteMeta(term.Incorrect)
				re, err = regexp.Compile("(?i)" + escaped)
			}

			if err != nil {
				continue
			}

			matches := re.FindAllStringIndex(line, -1)
			for _, match := range matches {
				// Extract the actual matched text
				matchedText := line[match[0]:match[1]]

				// Skip if the matched text is actually the correct form
				if strings.EqualFold(matchedText, term.Correct) {
					continue
				}

				// Get context (surrounding text)
				context := extractContext(line, match[0], match[1], 40)

				result.Issues = append(result.Issues, TerminologyIssue{
					Incorrect: matchedText,
					Correct:   term.Correct,
					Line:      lineNum + 1,
					Context:   context,
					Notes:     term.Notes,
				})
			}
		}
	}

	result.IssueCount = len(result.Issues)
	return result
}

// extractContext extracts surrounding text for context
func extractContext(line string, start, end, contextLen int) string {
	// Calculate bounds
	ctxStart := start - contextLen
	if ctxStart < 0 {
		ctxStart = 0
	}
	ctxEnd := end + contextLen
	if ctxEnd > len(line) {
		ctxEnd = len(line)
	}

	context := line[ctxStart:ctxEnd]

	// Add ellipsis if truncated
	if ctxStart > 0 {
		context = "..." + context
	}
	if ctxEnd < len(line) {
		context = context + "..."
	}

	return context
}

// CheckTranslations checks if pages exist in all specified languages
func (c *Client) CheckTranslations(ctx context.Context, args CheckTranslationsArgs) (CheckTranslationsResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CheckTranslationsResult{}, err
	}

	if len(args.Languages) == 0 {
		return CheckTranslationsResult{}, fmt.Errorf("at least one language is required")
	}

	// Default pattern
	pattern := args.Pattern
	if pattern == "" {
		pattern = "subpage"
	}
	if pattern != "subpage" && pattern != "suffix" && pattern != "prefix" {
		return CheckTranslationsResult{}, fmt.Errorf("invalid pattern: %s (use 'subpage', 'suffix', or 'prefix')", pattern)
	}

	// Get base pages to check
	var basePages []string
	limit := normalizeLimit(args.Limit, 20, 100)

	if len(args.BasePages) > 0 {
		basePages = args.BasePages
		if len(basePages) > limit {
			basePages = basePages[:limit]
		}
	} else if args.Category != "" {
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
			Category: args.Category,
			Limit:    limit,
		})
		if err != nil {
			return CheckTranslationsResult{}, fmt.Errorf("failed to get category members: %w", err)
		}
		for _, p := range catResult.Members {
			basePages = append(basePages, p.Title)
		}
	} else {
		return CheckTranslationsResult{}, fmt.Errorf("either 'base_pages' or 'category' must be specified")
	}

	result := CheckTranslationsResult{
		LanguagesChecked: args.Languages,
		Pattern:          pattern,
		Pages:            make([]PageTranslationResult, 0, len(basePages)),
	}

	// Check each base page for all languages
	for _, basePage := range basePages {
		pageResult := PageTranslationResult{
			BasePage:     basePage,
			Translations: make(map[string]TranslationStatus),
			Complete:     true,
		}

		for _, lang := range args.Languages {
			// Build page title based on pattern
			var langPage string
			switch pattern {
			case "subpage":
				langPage = fmt.Sprintf("%s/%s", basePage, lang)
			case "suffix":
				langPage = fmt.Sprintf("%s (%s)", basePage, lang)
			case "prefix":
				langPage = fmt.Sprintf("%s:%s", lang, basePage)
			}

			// Check if page exists
			info, err := c.GetPageInfo(ctx, PageInfoArgs{Title: langPage})
			status := TranslationStatus{
				PageTitle: langPage,
			}

			if err == nil && info.Exists {
				status.Exists = true
				status.PageID = info.PageID
				status.Length = info.Length
			} else {
				status.Exists = false
				pageResult.MissingLangs = append(pageResult.MissingLangs, lang)
				pageResult.Complete = false
				result.MissingCount++
			}

			pageResult.Translations[lang] = status
		}

		result.Pages = append(result.Pages, pageResult)
	}

	result.PagesChecked = len(result.Pages)
	return result, nil
}

// FindBrokenInternalLinks finds internal wiki links that point to non-existent pages
// Optimized to use batch page existence checks instead of individual API calls
func (c *Client) FindBrokenInternalLinks(ctx context.Context, args FindBrokenInternalLinksArgs) (FindBrokenInternalLinksResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return FindBrokenInternalLinksResult{}, err
	}

	// Get pages to check
	var pagesToCheck []string
	limit := normalizeLimit(args.Limit, 20, 100)

	if len(args.Pages) > 0 {
		pagesToCheck = args.Pages
		if len(pagesToCheck) > limit {
			pagesToCheck = pagesToCheck[:limit]
		}
	} else if args.Category != "" {
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
			Category: args.Category,
			Limit:    limit,
		})
		if err != nil {
			return FindBrokenInternalLinksResult{}, fmt.Errorf("failed to get category members: %w", err)
		}
		for _, p := range catResult.Members {
			pagesToCheck = append(pagesToCheck, p.Title)
		}
	} else {
		return FindBrokenInternalLinksResult{}, fmt.Errorf("either 'pages' or 'category' must be specified")
	}

	result := FindBrokenInternalLinksResult{
		Pages: make([]PageBrokenLinksResult, 0, len(pagesToCheck)),
	}

	// Regex to match internal wiki links: [[Target]] or [[Target|Display]]
	linkRegex := regexp.MustCompile(`\[\[([^\]|#]+)(?:[|#][^\]]*)?]]`)

	// First pass: collect all unique link targets from all pages
	type linkLocation struct {
		pageTitle string
		target    string
		line      int
		context   string
	}
	var allLinkLocations []linkLocation
	allTargets := make(map[string]bool)
	pageContents := make(map[string]string)

	for _, pageTitle := range pagesToCheck {
		page, err := c.GetPage(ctx, GetPageArgs{Title: pageTitle, Format: "wikitext"})
		if err != nil {
			result.Pages = append(result.Pages, PageBrokenLinksResult{
				Title:       pageTitle,
				BrokenLinks: make([]BrokenLink, 0),
				Error:       err.Error(),
			})
			continue
		}
		pageContents[pageTitle] = page.Content

		lines := strings.Split(page.Content, "\n")
		for lineNum, line := range lines {
			matches := linkRegex.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) < 2 {
					continue
				}

				target := strings.TrimSpace(match[1])

				// Skip special prefixes (categories, files, interwiki)
				lowerTarget := strings.ToLower(target)
				if strings.HasPrefix(lowerTarget, "category:") ||
					strings.HasPrefix(lowerTarget, "file:") ||
					strings.HasPrefix(lowerTarget, "image:") ||
					strings.HasPrefix(lowerTarget, ":") ||
					strings.HasPrefix(lowerTarget, "http") {
					continue
				}

				allTargets[target] = true
				allLinkLocations = append(allLinkLocations, linkLocation{
					pageTitle: pageTitle,
					target:    target,
					line:      lineNum + 1,
					context:   extractContext(line, strings.Index(line, match[0]), strings.Index(line, match[0])+len(match[0]), 30),
				})
			}
		}
	}

	// Second pass: batch check all targets at once
	targetList := make([]string, 0, len(allTargets))
	for target := range allTargets {
		targetList = append(targetList, target)
	}

	existenceMap, err := c.checkPagesExist(ctx, targetList)
	if err != nil {
		return FindBrokenInternalLinksResult{}, fmt.Errorf("failed to check page existence: %w", err)
	}

	// Third pass: build results using the existence map
	pageResults := make(map[string]*PageBrokenLinksResult)
	for _, pageTitle := range pagesToCheck {
		if _, hasContent := pageContents[pageTitle]; hasContent {
			pageResults[pageTitle] = &PageBrokenLinksResult{
				Title:       pageTitle,
				BrokenLinks: make([]BrokenLink, 0),
			}
		}
	}

	seenLinks := make(map[string]map[string]bool) // page -> target -> seen
	for _, loc := range allLinkLocations {
		if pageResults[loc.pageTitle] == nil {
			continue
		}

		// Deduplicate links within the same page
		if seenLinks[loc.pageTitle] == nil {
			seenLinks[loc.pageTitle] = make(map[string]bool)
		}
		if seenLinks[loc.pageTitle][loc.target] {
			continue
		}
		seenLinks[loc.pageTitle][loc.target] = true

		// Check if the target exists
		if exists, ok := existenceMap[loc.target]; !ok || !exists {
			pageResults[loc.pageTitle].BrokenLinks = append(pageResults[loc.pageTitle].BrokenLinks, BrokenLink{
				Target:  loc.target,
				Line:    loc.line,
				Context: loc.context,
			})
		}
	}

	// Finalize results
	for _, pageTitle := range pagesToCheck {
		if pr, ok := pageResults[pageTitle]; ok {
			pr.BrokenCount = len(pr.BrokenLinks)
			result.BrokenCount += pr.BrokenCount
			result.Pages = append(result.Pages, *pr)
		}
	}

	// Add pages that had errors (already added above)
	for _, pr := range result.Pages {
		if pr.Error != "" {
			continue // Already counted
		}
	}

	result.PagesChecked = len(result.Pages)
	return result, nil
}

// FindOrphanedPages finds pages that have no incoming links from other pages
func (c *Client) FindOrphanedPages(ctx context.Context, args FindOrphanedPagesArgs) (FindOrphanedPagesResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return FindOrphanedPagesResult{}, err
	}

	limit := normalizeLimit(args.Limit, 50, 200)

	// Use the API's lonelypages query (pages with no links to them)
	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "querypage")
	params.Set("qppage", "Lonelypages")
	params.Set("qplimit", strconv.Itoa(limit))

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return FindOrphanedPagesResult{}, err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return FindOrphanedPagesResult{}, fmt.Errorf("unexpected response format")
	}

	querypage, ok := query["querypage"].(map[string]interface{})
	if !ok {
		return FindOrphanedPagesResult{}, fmt.Errorf("querypage not found in response")
	}

	results, ok := querypage["results"].([]interface{})
	if !ok {
		return FindOrphanedPagesResult{}, fmt.Errorf("results not found in querypage")
	}

	orphaned := make([]OrphanedPage, 0)
	for _, r := range results {
		page := r.(map[string]interface{})

		// Filter by namespace if specified
		ns := getInt(page, "ns")
		if args.Namespace >= 0 && ns != args.Namespace {
			continue
		}

		title := getString(page, "title")

		// Filter by prefix if specified
		if args.Prefix != "" && !strings.HasPrefix(title, args.Prefix) {
			continue
		}

		orphaned = append(orphaned, OrphanedPage{
			Title:  title,
			PageID: getInt(page, "value"),
		})
	}

	return FindOrphanedPagesResult{
		OrphanedPages: orphaned,
		TotalChecked:  len(results),
		OrphanedCount: len(orphaned),
	}, nil
}

// GetBacklinks returns pages that link to the specified page ("What links here")
func (c *Client) GetBacklinks(ctx context.Context, args GetBacklinksArgs) (GetBacklinksResult, error) {
	if args.Title == "" {
		return GetBacklinksResult{}, fmt.Errorf("title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetBacklinksResult{}, err
	}

	limit := normalizeLimit(args.Limit, 50, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "backlinks")
	params.Set("bltitle", args.Title)
	params.Set("bllimit", strconv.Itoa(limit))

	if args.Namespace >= 0 {
		params.Set("blnamespace", strconv.Itoa(args.Namespace))
	}

	if !args.Redirect {
		params.Set("blfilterredir", "nonredirects")
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetBacklinksResult{}, err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return GetBacklinksResult{}, fmt.Errorf("unexpected response format")
	}

	backlinks, ok := query["backlinks"].([]interface{})
	if !ok {
		return GetBacklinksResult{Title: args.Title, Backlinks: make([]BacklinkInfo, 0)}, nil
	}

	result := GetBacklinksResult{
		Title:     args.Title,
		Backlinks: make([]BacklinkInfo, 0, len(backlinks)),
	}

	for _, bl := range backlinks {
		link := bl.(map[string]interface{})
		info := BacklinkInfo{
			PageID:    getInt(link, "pageid"),
			Title:     getString(link, "title"),
			Namespace: getInt(link, "ns"),
		}
		if _, isRedirect := link["redirect"]; isRedirect {
			info.IsRedirect = true
		}
		result.Backlinks = append(result.Backlinks, info)
	}

	result.Count = len(result.Backlinks)

	// Check for continuation
	if _, ok := resp["continue"]; ok {
		result.HasMore = true
	}

	return result, nil
}

// GetRevisions returns the revision history for a page
func (c *Client) GetRevisions(ctx context.Context, args GetRevisionsArgs) (GetRevisionsResult, error) {
	if args.Title == "" {
		return GetRevisionsResult{}, fmt.Errorf("title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetRevisionsResult{}, err
	}

	limit := normalizeLimit(args.Limit, 20, 100)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", args.Title)
	params.Set("prop", "revisions")
	params.Set("rvprop", "ids|timestamp|user|size|comment|flags")
	params.Set("rvlimit", strconv.Itoa(limit))

	if args.Start != "" {
		params.Set("rvstart", args.Start)
	}
	if args.End != "" {
		params.Set("rvend", args.End)
	}
	if args.User != "" {
		params.Set("rvuser", args.User)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetRevisionsResult{}, err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return GetRevisionsResult{}, fmt.Errorf("unexpected response format")
	}

	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return GetRevisionsResult{}, fmt.Errorf("pages not found in response")
	}

	result := GetRevisionsResult{
		Title:     args.Title,
		Revisions: make([]RevisionInfo, 0),
	}

	for pageIDStr, pageData := range pages {
		pageID, _ := strconv.Atoi(pageIDStr)
		if pageID < 0 {
			return GetRevisionsResult{}, fmt.Errorf("page '%s' not found", args.Title)
		}

		page := pageData.(map[string]interface{})
		result.PageID = pageID
		result.Title = getString(page, "title")

		revisions, ok := page["revisions"].([]interface{})
		if !ok {
			return result, nil
		}

		var prevSize int
		for i, rev := range revisions {
			r := rev.(map[string]interface{})
			info := RevisionInfo{
				RevID:     getInt(r, "revid"),
				ParentID:  getInt(r, "parentid"),
				User:      getString(r, "user"),
				Timestamp: getString(r, "timestamp"),
				Size:      getInt(r, "size"),
				Comment:   getString(r, "comment"),
			}

			if _, isMinor := r["minor"]; isMinor {
				info.Minor = true
			}

			// Calculate size diff
			if i == 0 {
				prevSize = info.Size
			} else {
				info.SizeDiff = info.Size - prevSize
				prevSize = info.Size
			}

			result.Revisions = append(result.Revisions, info)
		}

		break // Only process first page
	}

	result.Count = len(result.Revisions)

	// Check for continuation
	if _, ok := resp["continue"]; ok {
		result.HasMore = true
	}

	return result, nil
}

// CompareRevisions compares two revisions and returns the diff
func (c *Client) CompareRevisions(ctx context.Context, args CompareRevisionsArgs) (CompareRevisionsResult, error) {
	if args.FromRev == 0 && args.FromTitle == "" {
		return CompareRevisionsResult{}, fmt.Errorf("either from_rev or from_title is required")
	}
	if args.ToRev == 0 && args.ToTitle == "" {
		return CompareRevisionsResult{}, fmt.Errorf("either to_rev or to_title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CompareRevisionsResult{}, err
	}

	params := url.Values{}
	params.Set("action", "compare")

	if args.FromRev > 0 {
		params.Set("fromrev", strconv.Itoa(args.FromRev))
	} else {
		params.Set("fromtitle", args.FromTitle)
	}

	if args.ToRev > 0 {
		params.Set("torev", strconv.Itoa(args.ToRev))
	} else {
		params.Set("totitle", args.ToTitle)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return CompareRevisionsResult{}, err
	}

	compare, ok := resp["compare"].(map[string]interface{})
	if !ok {
		return CompareRevisionsResult{}, fmt.Errorf("compare not found in response")
	}

	result := CompareRevisionsResult{
		FromTitle:     getString(compare, "fromtitle"),
		FromRevID:     getInt(compare, "fromrevid"),
		ToTitle:       getString(compare, "totitle"),
		ToRevID:       getInt(compare, "torevid"),
		Diff:          getString(compare, "*"),
		FromUser:      getString(compare, "fromuser"),
		ToUser:        getString(compare, "touser"),
		FromTimestamp: getString(compare, "fromtimestamp"),
		ToTimestamp:   getString(compare, "totimestamp"),
	}

	// Clean up the diff HTML for readability
	if result.Diff != "" {
		result.Diff = sanitizeHTML(result.Diff)
	}

	return result, nil
}

// GetUserContributions returns the contributions (edits) made by a user
func (c *Client) GetUserContributions(ctx context.Context, args GetUserContributionsArgs) (GetUserContributionsResult, error) {
	if args.User == "" {
		return GetUserContributionsResult{}, fmt.Errorf("user is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetUserContributionsResult{}, err
	}

	limit := normalizeLimit(args.Limit, 50, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "usercontribs")
	params.Set("ucuser", args.User)
	params.Set("ucprop", "ids|title|timestamp|comment|size|sizediff|flags")
	params.Set("uclimit", strconv.Itoa(limit))

	if args.Namespace >= 0 {
		params.Set("ucnamespace", strconv.Itoa(args.Namespace))
	}
	if args.Start != "" {
		params.Set("ucstart", args.Start)
	}
	if args.End != "" {
		params.Set("ucend", args.End)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetUserContributionsResult{}, err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return GetUserContributionsResult{}, fmt.Errorf("unexpected response format")
	}

	contribs, ok := query["usercontribs"].([]interface{})
	if !ok {
		return GetUserContributionsResult{User: args.User, Contributions: make([]UserContribution, 0)}, nil
	}

	result := GetUserContributionsResult{
		User:          args.User,
		Contributions: make([]UserContribution, 0, len(contribs)),
	}

	for _, c := range contribs {
		contrib := c.(map[string]interface{})
		uc := UserContribution{
			PageID:    getInt(contrib, "pageid"),
			Title:     getString(contrib, "title"),
			Namespace: getInt(contrib, "ns"),
			RevID:     getInt(contrib, "revid"),
			ParentID:  getInt(contrib, "parentid"),
			Timestamp: getString(contrib, "timestamp"),
			Comment:   getString(contrib, "comment"),
			Size:      getInt(contrib, "size"),
			SizeDiff:  getInt(contrib, "sizediff"),
		}

		if _, isMinor := contrib["minor"]; isMinor {
			uc.Minor = true
		}
		if _, isNew := contrib["new"]; isNew {
			uc.New = true
		}

		result.Contributions = append(result.Contributions, uc)
	}

	result.Count = len(result.Contributions)

	// Check for continuation
	if _, ok := resp["continue"]; ok {
		result.HasMore = true
	}

	return result, nil
}

// ========== Simple Edit Tools ==========

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

// SearchInPage searches for text within a specific page
func (c *Client) SearchInPage(ctx context.Context, args SearchInPageArgs) (SearchInPageResult, error) {
	if args.Title == "" {
		return SearchInPageResult{}, fmt.Errorf("title is required")
	}
	if args.Query == "" {
		return SearchInPageResult{}, fmt.Errorf("query is required")
	}

	// Get page content
	page, err := c.GetPage(ctx, GetPageArgs{Title: args.Title, Format: "wikitext"})
	if err != nil {
		return SearchInPageResult{}, fmt.Errorf("failed to get page: %w", err)
	}

	result := SearchInPageResult{
		Title:   page.Title,
		Query:   args.Query,
		Matches: make([]PageMatch, 0),
	}

	// Build regex
	var re *regexp.Regexp
	if args.UseRegex {
		re, err = regexp.Compile("(?i)" + args.Query)
		if err != nil {
			return SearchInPageResult{}, fmt.Errorf("invalid regex: %w", err)
		}
	} else {
		re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(args.Query))
	}

	contextLines := args.ContextLines
	if contextLines <= 0 {
		contextLines = 2
	}

	lines := strings.Split(page.Content, "\n")

	for lineNum, line := range lines {
		matches := re.FindAllStringIndex(line, -1)
		for _, match := range matches {
			// Build context from surrounding lines
			startLine := lineNum - contextLines
			if startLine < 0 {
				startLine = 0
			}
			endLine := lineNum + contextLines + 1
			if endLine > len(lines) {
				endLine = len(lines)
			}

			contextStr := strings.Join(lines[startLine:endLine], "\n")

			result.Matches = append(result.Matches, PageMatch{
				Line:    lineNum + 1,
				Column:  match[0] + 1,
				Text:    line[match[0]:match[1]],
				Context: contextStr,
			})
		}
	}

	result.MatchCount = len(result.Matches)
	return result, nil
}

// ResolveTitle tries to find the correct page title with fuzzy matching
func (c *Client) ResolveTitle(ctx context.Context, args ResolveTitleArgs) (ResolveTitleResult, error) {
	if args.Title == "" {
		return ResolveTitleResult{}, fmt.Errorf("title is required")
	}

	maxResults := args.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}

	result := ResolveTitleResult{
		Suggestions: make([]TitleSuggestion, 0),
	}

	// First try exact match with normalization
	normalizedTitle := normalizePageTitle(args.Title)
	info, err := c.GetPageInfo(ctx, PageInfoArgs{Title: normalizedTitle})
	if err == nil && info.Exists {
		result.ExactMatch = true
		result.ResolvedTitle = info.Title
		result.PageID = info.PageID
		result.Message = "Exact match found"
		return result, nil
	}

	// Try case-insensitive search
	searchResult, err := c.Search(ctx, SearchArgs{
		Query: args.Title,
		Limit: maxResults * 2, // Get more to filter
	})
	if err != nil {
		return ResolveTitleResult{}, fmt.Errorf("search failed: %w", err)
	}

	// Calculate similarity and rank results
	titleLower := strings.ToLower(args.Title)
	for _, hit := range searchResult.Results {
		hitLower := strings.ToLower(hit.Title)

		// Calculate simple similarity score
		similarity := calculateSimilarity(titleLower, hitLower)

		result.Suggestions = append(result.Suggestions, TitleSuggestion{
			Title:      hit.Title,
			PageID:     hit.PageID,
			Similarity: similarity,
		})
	}

	// Sort by similarity (descending)
	for i := 0; i < len(result.Suggestions)-1; i++ {
		for j := i + 1; j < len(result.Suggestions); j++ {
			if result.Suggestions[j].Similarity > result.Suggestions[i].Similarity {
				result.Suggestions[i], result.Suggestions[j] = result.Suggestions[j], result.Suggestions[i]
			}
		}
	}

	// Limit results
	if len(result.Suggestions) > maxResults {
		result.Suggestions = result.Suggestions[:maxResults]
	}

	if len(result.Suggestions) > 0 && result.Suggestions[0].Similarity > 0.8 {
		result.ResolvedTitle = result.Suggestions[0].Title
		result.PageID = result.Suggestions[0].PageID
		result.Message = fmt.Sprintf("Did you mean '%s'?", result.Suggestions[0].Title)
	} else if len(result.Suggestions) > 0 {
		result.Message = fmt.Sprintf("Page '%s' not found. Similar pages found.", args.Title)
	} else {
		result.Message = fmt.Sprintf("Page '%s' not found. No similar pages.", args.Title)
	}

	return result, nil
}

// Helper function to truncate string for display
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// Helper function to calculate string similarity (Jaccard-like)
func calculateSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	// Split into words
	words1 := strings.Fields(s1)
	words2 := strings.Fields(s2)

	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	// Count common words
	set1 := make(map[string]bool)
	for _, w := range words1 {
		set1[w] = true
	}

	common := 0
	for _, w := range words2 {
		if set1[w] {
			common++
		}
	}

	// Jaccard similarity
	union := len(words1) + len(words2) - common
	if union == 0 {
		return 0.0
	}

	return float64(common) / float64(union)
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
				from := getString(norm, "from")
				to := getString(norm, "to")
				normalized[to] = from
			}
		}

		// Check each page in the response
		for _, pageData := range pages {
			page := pageData.(map[string]interface{})
			title := getString(page, "title")

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

// ListUsers lists wiki users, optionally filtered by group
func (c *Client) ListUsers(ctx context.Context, args ListUsersArgs) (ListUsersResult, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return ListUsersResult{}, err
	}

	limit := normalizeLimit(args.Limit, DefaultLimit, MaxLimit)

	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "allusers")
	params.Set("aulimit", strconv.Itoa(limit))
	params.Set("auprop", "groups|editcount|registration")

	if args.Group != "" {
		params.Set("augroup", args.Group)
	}

	if args.ActiveOnly {
		params.Set("auactiveusers", "1")
	}

	if args.ContinueFrom != "" {
		params.Set("aufrom", args.ContinueFrom)
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return ListUsersResult{}, err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return ListUsersResult{}, fmt.Errorf("unexpected response format: missing query")
	}

	allusers, ok := query["allusers"].([]interface{})
	if !ok {
		return ListUsersResult{}, fmt.Errorf("unexpected response format: missing allusers")
	}

	users := make([]UserInfo, 0, len(allusers))
	for _, u := range allusers {
		user := u.(map[string]interface{})

		userInfo := UserInfo{
			UserID:       getInt(user, "userid"),
			Name:         getString(user, "name"),
			EditCount:    getInt(user, "editcount"),
			Registration: getString(user, "registration"),
		}

		// Extract groups
		if groups, ok := user["groups"].([]interface{}); ok {
			for _, g := range groups {
				if groupName, ok := g.(string); ok {
					userInfo.Groups = append(userInfo.Groups, groupName)
				}
			}
		}

		users = append(users, userInfo)
	}

	result := ListUsersResult{
		Users:      users,
		TotalCount: len(users),
		Group:      args.Group,
	}

	// Check for continuation
	if cont, ok := resp["continue"].(map[string]interface{}); ok {
		if aufrom, ok := cont["aufrom"].(string); ok {
			result.HasMore = true
			result.ContinueFrom = aufrom
		}
	}

	return result, nil
}
