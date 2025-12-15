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
func (c *Client) GetPage(ctx context.Context, args GetPageArgs) (PageContent, error) {
	if args.Title == "" {
		return PageContent{}, fmt.Errorf("title is required")
	}

	format := args.Format
	if format == "" {
		format = "wikitext"
	}

	if format == "html" {
		return c.getPageHTML(ctx, args.Title)
	}

	return c.getPageWikitext(ctx, args.Title)
}

func (c *Client) getPageWikitext(ctx context.Context, title string) (PageContent, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return PageContent{}, err
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", title)
	params.Set("prop", "revisions")
	params.Set("rvprop", "content|ids|timestamp")
	params.Set("rvslots", "main")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return PageContent{}, err
	}

	query := resp["query"].(map[string]interface{})
	pages := query["pages"].(map[string]interface{})

	for pageID, pageData := range pages {
		page := pageData.(map[string]interface{})

		// Check if page exists
		if _, missing := page["missing"]; missing {
			return PageContent{}, fmt.Errorf("page '%s' does not exist", title)
		}

		revisions := page["revisions"].([]interface{})
		if len(revisions) == 0 {
			return PageContent{}, fmt.Errorf("no revisions found for page '%s'", title)
		}

		rev := revisions[0].(map[string]interface{})
		slots := rev["slots"].(map[string]interface{})
		main := slots["main"].(map[string]interface{})
		content := main["content"].(string)

		truncated := false
		if len(content) > CharacterLimit {
			content, truncated = truncateContent(content, CharacterLimit)
		}

		id, _ := strconv.Atoi(pageID)
		result := PageContent{
			Title:     page["title"].(string),
			PageID:    id,
			Content:   content,
			Format:    "wikitext",
			Revision:  int(rev["revid"].(float64)),
			Timestamp: rev["timestamp"].(string),
			Truncated: truncated,
		}

		if truncated {
			result.Message = "Content was truncated due to size limits. Consider fetching specific sections."
		}

		return result, nil
	}

	return PageContent{}, fmt.Errorf("page '%s' not found", title)
}

func (c *Client) getPageHTML(ctx context.Context, title string) (PageContent, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return PageContent{}, err
	}

	params := url.Values{}
	params.Set("action", "parse")
	params.Set("page", title)
	params.Set("prop", "text|revid")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return PageContent{}, err
	}

	parse := resp["parse"].(map[string]interface{})
	text := parse["text"].(map[string]interface{})
	content := text["*"].(string)

	truncated := false
	if len(content) > CharacterLimit {
		content, truncated = truncateContent(content, CharacterLimit)
	}

	result := PageContent{
		Title:     parse["title"].(string),
		PageID:    int(parse["pageid"].(float64)),
		Content:   content,
		Format:    "html",
		Revision:  int(parse["revid"].(float64)),
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
func (c *Client) GetPageInfo(ctx context.Context, args PageInfoArgs) (PageInfo, error) {
	if args.Title == "" {
		return PageInfo{}, fmt.Errorf("title is required")
	}

	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return PageInfo{}, err
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", args.Title)
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

		return info, nil
	}

	return PageInfo{}, fmt.Errorf("page '%s' not found", args.Title)
}

// EditPage creates or edits a page
func (c *Client) EditPage(ctx context.Context, args EditPageArgs) (EditResult, error) {
	if args.Title == "" {
		return EditResult{}, fmt.Errorf("title is required")
	}
	if args.Content == "" {
		return EditResult{}, fmt.Errorf("content is required")
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
func (c *Client) GetExternalLinksBatch(ctx context.Context, args GetExternalLinksBatchArgs) (ExternalLinksBatchResult, error) {
	if len(args.Titles) == 0 {
		return ExternalLinksBatchResult{}, fmt.Errorf("at least one title is required")
	}

	// Limit batch size to prevent overwhelming the API
	maxBatch := 10
	if len(args.Titles) > maxBatch {
		args.Titles = args.Titles[:maxBatch]
	}

	result := ExternalLinksBatchResult{
		Pages: make([]PageExternalLinks, 0, len(args.Titles)),
	}

	// Process pages sequentially to respect rate limiting
	for _, title := range args.Titles {
		pageLinks, err := c.GetExternalLinks(ctx, GetExternalLinksArgs{Title: title})
		if err != nil {
			result.Pages = append(result.Pages, PageExternalLinks{
				Title: title,
				Links: make([]ExternalLink, 0), // Initialize as empty slice to avoid JSON null
				Error: err.Error(),
			})
			continue
		}

		result.Pages = append(result.Pages, PageExternalLinks{
			Title: pageLinks.Title,
			Links: pageLinks.Links,
			Count: pageLinks.Count,
		})
		result.TotalLinks += pageLinks.Count
	}

	return result, nil
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
					resp.Body.Close()
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
		if strings.HasPrefix(line, "|") {
			line = strings.TrimPrefix(line, "|")
		}

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

	for _, pageTitle := range pagesToCheck {
		pageResult := PageBrokenLinksResult{
			Title:       pageTitle,
			BrokenLinks: make([]BrokenLink, 0),
		}

		// Get page content
		page, err := c.GetPage(ctx, GetPageArgs{Title: pageTitle, Format: "wikitext"})
		if err != nil {
			pageResult.Error = err.Error()
			result.Pages = append(result.Pages, pageResult)
			continue
		}

		// Find all internal links
		lines := strings.Split(page.Content, "\n")
		checkedTargets := make(map[string]bool) // Avoid duplicate checks

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

				// Skip if already checked
				if checkedTargets[target] {
					continue
				}
				checkedTargets[target] = true

				// Check if target page exists
				info, err := c.GetPageInfo(ctx, PageInfoArgs{Title: target})
				if err != nil || !info.Exists {
					brokenLink := BrokenLink{
						Target:  target,
						Line:    lineNum + 1,
						Context: extractContext(line, strings.Index(line, match[0]), strings.Index(line, match[0])+len(match[0]), 30),
					}
					pageResult.BrokenLinks = append(pageResult.BrokenLinks, brokenLink)
				}
			}
		}

		pageResult.BrokenCount = len(pageResult.BrokenLinks)
		result.BrokenCount += pageResult.BrokenCount
		result.Pages = append(result.Pages, pageResult)
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
