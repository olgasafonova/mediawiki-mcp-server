package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
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

// getNamespacePageCount tries to get total page count for a namespace
// Returns 0 if unable to fetch statistics
func (c *Client) getNamespacePageCount(ctx context.Context, namespace int) int {
	// For main namespace (0), we can get statistics from siteinfo
	if namespace == 0 {
		params := url.Values{}
		params.Set("action", "query")
		params.Set("meta", "siteinfo")
		params.Set("siprop", "statistics")

		resp, err := c.apiRequest(ctx, params)
		if err != nil {
			return 0
		}

		query := getMap(resp["query"])
		if query == nil {
			return 0
		}
		stats := getMap(query["statistics"])
		if stats == nil {
			return 0
		}

		// "articles" gives the count of content pages in main namespace
		return getInt(stats["articles"])
	}

	// For other namespaces, we can't efficiently get totals without iterating
	return 0
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

	query := getMap(resp["query"])
	if query == nil {
		return ListPagesResult{}, fmt.Errorf("unexpected response format: missing query")
	}

	allpages := getSlice(query["allpages"])
	pages := make([]PageSummary, 0, len(allpages))
	for _, p := range allpages {
		page := getMap(p)
		if page == nil {
			continue
		}
		pages = append(pages, PageSummary{
			PageID: getInt(page["pageid"]),
			Title:  getString(page["title"]),
		})
	}

	result := ListPagesResult{
		Pages:         pages,
		ReturnedCount: len(pages),
		TotalCount:    len(pages), // Deprecated: kept for backwards compatibility
	}

	// Check for continuation
	if cont := getMap(resp["continue"]); cont != nil {
		if apcontinue := getString(cont["apcontinue"]); apcontinue != "" {
			result.HasMore = true
			result.ContinueFrom = apcontinue
		}
	}

	// Try to get namespace statistics for total estimate (only when no prefix filter)
	if args.Prefix == "" && args.Namespace >= 0 {
		if estimate := c.getNamespacePageCount(ctx, args.Namespace); estimate > 0 {
			result.TotalEstimate = estimate
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
			ContentModel: getString(page["contentmodel"]),
			PageLanguage: getString(page["pagelanguage"]),
			Length:       int(page["length"].(float64)),
			Touched:      getString(page["touched"]),
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
		SiteName:    getString(general["sitename"]),
		MainPage:    getString(general["mainpage"]),
		Base:        getString(general["base"]),
		Generator:   getString(general["generator"]),
		PHPVersion:  getString(general["phpversion"]),
		Language:    getString(general["lang"]),
		ArticlePath: getString(general["articlepath"]),
		Server:      getString(general["server"]),
		Timezone:    getString(general["timezone"]),
		WriteAPI:    general["writeapi"] != nil,
	}

	// Statistics
	if stats, ok := query["statistics"].(map[string]interface{}); ok {
		info.Statistics = &WikiStats{
			Pages:       getInt(stats["pages"]),
			Articles:    getInt(stats["articles"]),
			Edits:       getInt(stats["edits"]),
			Images:      getInt(stats["images"]),
			Users:       getInt(stats["users"]),
			ActiveUsers: getInt(stats["activeusers"]),
			Admins:      getInt(stats["admins"]),
		}
	}

	// Cache the result
	c.setCache(cacheKey, info, "wiki_info")

	return info, nil
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

// calculateSimilarity calculates string similarity (Jaccard-like)
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

// GetSections retrieves section structure and optionally section content from a page
func (c *Client) GetSections(ctx context.Context, args GetSectionsArgs) (GetSectionsResult, error) {
	if args.Title == "" {
		return GetSectionsResult{}, fmt.Errorf("title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetSectionsResult{}, err
	}

	normalizedTitle := normalizePageTitle(args.Title)

	// If section number is specified, retrieve that section's content
	if args.Section > 0 || (args.Section == 0 && args.Format != "") {
		return c.getSectionContent(ctx, normalizedTitle, args.Section, args.Format)
	}

	// Otherwise, list all sections
	cacheKey := fmt.Sprintf("sections:%s", normalizedTitle)
	if cached, ok := c.getCached(cacheKey); ok {
		return cached.(GetSectionsResult), nil
	}

	params := url.Values{}
	params.Set("action", "parse")
	params.Set("page", normalizedTitle)
	params.Set("prop", "sections")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetSectionsResult{}, err
	}

	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		return GetSectionsResult{}, fmt.Errorf("%s", errInfo["info"])
	}

	parse := resp["parse"].(map[string]interface{})
	pageID := int(parse["pageid"].(float64))
	title := parse["title"].(string)

	sectionsRaw := parse["sections"].([]interface{})
	sections := make([]SectionInfo, 0, len(sectionsRaw))

	for _, s := range sectionsRaw {
		sec := s.(map[string]interface{})
		index, _ := strconv.Atoi(sec["index"].(string))
		level, _ := strconv.Atoi(sec["level"].(string))
		lineNum := 0
		if line, ok := sec["line"].(float64); ok {
			lineNum = int(line)
		}

		sections = append(sections, SectionInfo{
			Index:   index,
			Level:   level,
			Title:   stripHTMLTags(sec["line"].(string)),
			Anchor:  sec["anchor"].(string),
			LineNum: lineNum,
		})
	}

	result := GetSectionsResult{
		Title:    title,
		PageID:   pageID,
		Sections: sections,
		Message:  fmt.Sprintf("Found %d sections. Use section parameter to get specific section content.", len(sections)),
	}

	c.setCache(cacheKey, result, "page_content")
	return result, nil
}

// getSectionContent retrieves the content of a specific section
func (c *Client) getSectionContent(ctx context.Context, title string, section int, format string) (GetSectionsResult, error) {
	if format == "" {
		format = "wikitext"
	}

	params := url.Values{}
	params.Set("action", "parse")
	params.Set("page", title)
	params.Set("section", strconv.Itoa(section))

	if format == "html" {
		params.Set("prop", "text|displaytitle")
	} else {
		params.Set("prop", "wikitext|displaytitle")
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetSectionsResult{}, err
	}

	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		return GetSectionsResult{}, fmt.Errorf("%s", errInfo["info"])
	}

	parse := resp["parse"].(map[string]interface{})
	pageID := int(parse["pageid"].(float64))
	pageTitle := parse["title"].(string)

	var content string
	var sectionTitle string

	if format == "html" {
		if text, ok := parse["text"].(map[string]interface{}); ok {
			content = text["*"].(string)
		}
	} else {
		if wikitext, ok := parse["wikitext"].(map[string]interface{}); ok {
			content = wikitext["*"].(string)
		}
	}

	// Extract section title from content if it starts with ==
	if format == "wikitext" && strings.HasPrefix(strings.TrimSpace(content), "==") {
		lines := strings.SplitN(content, "\n", 2)
		if len(lines) > 0 {
			sectionTitle = strings.Trim(lines[0], "= \t")
		}
	}

	return GetSectionsResult{
		Title:          pageTitle,
		PageID:         pageID,
		SectionContent: content,
		SectionTitle:   sectionTitle,
		Format:         format,
	}, nil
}

// GetRelated finds pages related to the given page
func (c *Client) GetRelated(ctx context.Context, args GetRelatedArgs) (GetRelatedResult, error) {
	if args.Title == "" {
		return GetRelatedResult{}, fmt.Errorf("title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetRelatedResult{}, err
	}

	limit := normalizeLimit(args.Limit, 20, 50)
	method := args.Method
	if method == "" {
		method = "categories"
	}

	normalizedTitle := normalizePageTitle(args.Title)
	result := GetRelatedResult{
		Title:  normalizedTitle,
		Method: method,
	}

	relatedMap := make(map[string]*RelatedPage)

	// Get categories for the source page
	if method == "categories" || method == "all" {
		cats, err := c.getPageCategories(ctx, normalizedTitle)
		if err == nil {
			result.Categories = cats

			// Get pages from each category
			for _, cat := range cats {
				if len(relatedMap) >= limit {
					break
				}
				members, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
					Category: cat,
					Limit:    limit,
					Type:     "page",
				})
				if err == nil {
					for _, m := range members.Members {
						if m.Title == normalizedTitle {
							continue
						}
						if existing, ok := relatedMap[m.Title]; ok {
							existing.Categories = append(existing.Categories, cat)
							existing.Score++
						} else {
							relatedMap[m.Title] = &RelatedPage{
								Title:      m.Title,
								PageID:     m.PageID,
								Relation:   "same_category",
								Categories: []string{cat},
								Score:      1,
							}
						}
					}
				}
			}
		}
	}

	// Get pages linked from this page
	if method == "links" || method == "all" {
		links, err := c.getPageLinks(ctx, normalizedTitle, limit)
		if err == nil {
			for _, link := range links {
				if existing, ok := relatedMap[link.Title]; ok {
					existing.Relation = "linked_and_categorized"
					existing.Score += 2
				} else {
					relatedMap[link.Title] = &RelatedPage{
						Title:    link.Title,
						PageID:   link.PageID,
						Relation: "linked_from",
						Score:    2,
					}
				}
			}
		}
	}

	// Get pages that link to this page
	if method == "backlinks" || method == "all" {
		backlinks, err := c.GetBacklinks(ctx, GetBacklinksArgs{
			Title: normalizedTitle,
			Limit: limit,
		})
		if err == nil {
			for _, bl := range backlinks.Backlinks {
				if existing, ok := relatedMap[bl.Title]; ok {
					existing.Relation = "bidirectional_link"
					existing.Score += 3
				} else {
					relatedMap[bl.Title] = &RelatedPage{
						Title:    bl.Title,
						PageID:   bl.PageID,
						Relation: "links_to",
						Score:    1,
					}
				}
			}
		}
	}

	// Convert map to slice and sort by score
	related := make([]RelatedPage, 0, len(relatedMap))
	for _, rp := range relatedMap {
		related = append(related, *rp)
	}

	// Sort by score descending
	for i := 0; i < len(related)-1; i++ {
		for j := i + 1; j < len(related); j++ {
			if related[j].Score > related[i].Score {
				related[i], related[j] = related[j], related[i]
			}
		}
	}

	// Limit results
	if len(related) > limit {
		related = related[:limit]
	}

	result.RelatedPages = related
	result.Count = len(related)

	return result, nil
}

// getPageCategories gets categories for a page
func (c *Client) getPageCategories(ctx context.Context, title string) ([]string, error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", title)
	params.Set("prop", "categories")
	params.Set("cllimit", "50")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return nil, err
	}

	query := resp["query"].(map[string]interface{})
	pages := query["pages"].(map[string]interface{})

	var categories []string
	for _, p := range pages {
		page := p.(map[string]interface{})
		if cats, ok := page["categories"].([]interface{}); ok {
			for _, cat := range cats {
				c := cat.(map[string]interface{})
				catTitle := c["title"].(string)
				// Remove "Category:" prefix
				catTitle = strings.TrimPrefix(catTitle, "Category:")
				categories = append(categories, catTitle)
			}
		}
	}

	return categories, nil
}

// getPageLinks gets outgoing links from a page
func (c *Client) getPageLinks(ctx context.Context, title string, limit int) ([]PageSummary, error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", title)
	params.Set("prop", "links")
	params.Set("pllimit", strconv.Itoa(limit))
	params.Set("plnamespace", "0") // Main namespace only

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return nil, err
	}

	query := resp["query"].(map[string]interface{})
	pages := query["pages"].(map[string]interface{})

	var links []PageSummary
	for _, p := range pages {
		page := p.(map[string]interface{})
		if linksList, ok := page["links"].([]interface{}); ok {
			for _, l := range linksList {
				link := l.(map[string]interface{})
				links = append(links, PageSummary{
					Title: link["title"].(string),
				})
			}
		}
	}

	return links, nil
}

// GetImages retrieves images/files used on a page
func (c *Client) GetImages(ctx context.Context, args GetImagesArgs) (GetImagesResult, error) {
	if args.Title == "" {
		return GetImagesResult{}, fmt.Errorf("title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetImagesResult{}, err
	}

	limit := normalizeLimit(args.Limit, 50, MaxLimit)
	normalizedTitle := normalizePageTitle(args.Title)

	// First get list of images on the page
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", normalizedTitle)
	params.Set("prop", "images")
	params.Set("imlimit", strconv.Itoa(limit))

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetImagesResult{}, err
	}

	query := resp["query"].(map[string]interface{})
	pages := query["pages"].(map[string]interface{})

	var imageTitles []string
	for _, p := range pages {
		page := p.(map[string]interface{})
		if images, ok := page["images"].([]interface{}); ok {
			for _, img := range images {
				i := img.(map[string]interface{})
				imageTitles = append(imageTitles, i["title"].(string))
			}
		}
	}

	if len(imageTitles) == 0 {
		return GetImagesResult{
			Title:  normalizedTitle,
			Images: []ImageInfo{},
			Count:  0,
		}, nil
	}

	// Get image info (URLs, dimensions) for each image
	images, err := c.getImageInfo(ctx, imageTitles)
	if err != nil {
		// Return basic info without URLs if imageinfo fails
		basicImages := make([]ImageInfo, 0, len(imageTitles))
		for _, t := range imageTitles {
			basicImages = append(basicImages, ImageInfo{Title: t})
		}
		return GetImagesResult{
			Title:  normalizedTitle,
			Images: basicImages,
			Count:  len(basicImages),
		}, nil
	}

	return GetImagesResult{
		Title:  normalizedTitle,
		Images: images,
		Count:  len(images),
	}, nil
}

// getImageInfo retrieves detailed info for images
func (c *Client) getImageInfo(ctx context.Context, titles []string) ([]ImageInfo, error) {
	if len(titles) == 0 {
		return nil, nil
	}

	// Batch in groups of 50
	batchSize := 50
	var allImages []ImageInfo

	for i := 0; i < len(titles); i += batchSize {
		end := i + batchSize
		if end > len(titles) {
			end = len(titles)
		}
		batch := titles[i:end]

		params := url.Values{}
		params.Set("action", "query")
		params.Set("titles", strings.Join(batch, "|"))
		params.Set("prop", "imageinfo")
		params.Set("iiprop", "url|size|mime")
		params.Set("iiurlwidth", "300") // Get thumbnail URL

		resp, err := c.apiRequest(ctx, params)
		if err != nil {
			continue
		}

		query := resp["query"].(map[string]interface{})
		pages := query["pages"].(map[string]interface{})

		for _, p := range pages {
			page := p.(map[string]interface{})
			title := getString(page["title"])

			imgInfo := ImageInfo{Title: title}

			if imageinfo, ok := page["imageinfo"].([]interface{}); ok && len(imageinfo) > 0 {
				info := imageinfo[0].(map[string]interface{})
				imgInfo.URL = getString(info["url"])
				imgInfo.ThumbURL = getString(info["thumburl"])
				imgInfo.Width = getInt(info["width"])
				imgInfo.Height = getInt(info["height"])
				imgInfo.Size = getInt(info["size"])
				imgInfo.MimeType = getString(info["mime"])
			}

			allImages = append(allImages, imgInfo)
		}
	}

	return allImages, nil
}

// MaxBatchSize is the maximum number of pages that can be fetched in a single batch
const MaxBatchSize = 50

// GetPagesBatch retrieves content for multiple pages in a single API call.
// This is significantly more efficient than calling GetPage individually.
func (c *Client) GetPagesBatch(ctx context.Context, args GetPagesBatchArgs) (GetPagesBatchResult, error) {
	if len(args.Titles) == 0 {
		return GetPagesBatchResult{}, fmt.Errorf("at least one title is required")
	}

	// Enforce batch size limit
	titles := args.Titles
	if len(titles) > MaxBatchSize {
		titles = titles[:MaxBatchSize]
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetPagesBatchResult{}, fmt.Errorf("authentication required: %w", err)
	}

	format := args.Format
	if format == "" {
		format = "wikitext"
	}

	result := GetPagesBatchResult{
		Pages:      make([]PageContentResult, 0, len(titles)),
		TotalCount: len(titles),
	}

	// Normalize all titles
	normalizedTitles := make([]string, len(titles))
	titleMap := make(map[string]string) // normalized -> original
	for i, t := range titles {
		normalized := normalizePageTitle(t)
		normalizedTitles[i] = normalized
		titleMap[normalized] = t
	}

	// MediaWiki API accepts pipe-separated titles
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", strings.Join(normalizedTitles, "|"))
	params.Set("prop", "revisions")
	params.Set("rvprop", "content|ids|timestamp")
	params.Set("rvslots", "main")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetPagesBatchResult{}, fmt.Errorf("API request failed: %w", err)
	}

	query := getMap(resp["query"])
	if query == nil {
		return GetPagesBatchResult{}, fmt.Errorf("unexpected API response: missing 'query' object")
	}

	pages := getMap(query["pages"])
	if pages == nil {
		return GetPagesBatchResult{}, fmt.Errorf("unexpected API response: missing 'pages' object")
	}

	// Track which pages we found
	foundTitles := make(map[string]bool)

	for _, pageData := range pages {
		page := getMap(pageData)
		if page == nil {
			continue
		}

		pageTitle := getString(page["title"])
		pageResult := PageContentResult{
			Title:  pageTitle,
			Format: format,
		}

		// Check if page is missing
		if _, missing := page["missing"]; missing {
			pageResult.Exists = false
			result.MissingCount++
			result.Pages = append(result.Pages, pageResult)
			foundTitles[pageTitle] = true
			continue
		}

		pageResult.Exists = true
		pageResult.PageID = getInt(page["pageid"])
		result.FoundCount++
		foundTitles[pageTitle] = true

		revisions := getSlice(page["revisions"])
		if len(revisions) == 0 {
			pageResult.Error = "no revisions found"
			result.Pages = append(result.Pages, pageResult)
			continue
		}

		rev := getMap(revisions[0])
		if rev == nil {
			pageResult.Error = "invalid revision data"
			result.Pages = append(result.Pages, pageResult)
			continue
		}

		slots := getMap(rev["slots"])
		if slots == nil {
			pageResult.Error = "invalid slots data"
			result.Pages = append(result.Pages, pageResult)
			continue
		}

		main := getMap(slots["main"])
		if main == nil {
			pageResult.Error = "invalid main slot"
			result.Pages = append(result.Pages, pageResult)
			continue
		}

		content := getString(main["*"])
		if content == "" {
			content = getString(main["content"])
		}

		pageResult.Content = content
		pageResult.Revision = getInt(rev["revid"])
		pageResult.Timestamp = getString(rev["timestamp"])

		// Truncate if necessary
		if len(content) > CharacterLimit {
			truncated, _ := truncateContent(content, CharacterLimit)
			pageResult.Content = truncated
			pageResult.Truncated = true
		}

		result.Pages = append(result.Pages, pageResult)
	}

	// Handle normalized titles that weren't found in response
	if normalized := getMap(query["normalized"]); normalized != nil {
		// MediaWiki returns normalized mappings
		for _, n := range getSlice(query["normalized"]) {
			normMap := getMap(n)
			if normMap != nil {
				from := getString(normMap["from"])
				to := getString(normMap["to"])
				if from != "" && to != "" {
					foundTitles[from] = foundTitles[to]
				}
			}
		}
	}

	return result, nil
}

// GetPagesInfoBatch retrieves metadata for multiple pages in a single API call.
func (c *Client) GetPagesInfoBatch(ctx context.Context, args GetPagesInfoBatchArgs) (GetPagesInfoBatchResult, error) {
	if len(args.Titles) == 0 {
		return GetPagesInfoBatchResult{}, fmt.Errorf("at least one title is required")
	}

	// Enforce batch size limit
	titles := args.Titles
	if len(titles) > MaxBatchSize {
		titles = titles[:MaxBatchSize]
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetPagesInfoBatchResult{}, err
	}

	result := GetPagesInfoBatchResult{
		Pages:      make([]PageInfo, 0, len(titles)),
		TotalCount: len(titles),
	}

	// Normalize all titles
	normalizedTitles := make([]string, len(titles))
	for i, t := range titles {
		normalizedTitles[i] = normalizePageTitle(t)
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", strings.Join(normalizedTitles, "|"))
	params.Set("prop", "info|categories")
	params.Set("inprop", "protection|url")
	params.Set("cllimit", "50")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetPagesInfoBatchResult{}, err
	}

	query := getMap(resp["query"])
	if query == nil {
		return GetPagesInfoBatchResult{}, fmt.Errorf("unexpected response format: missing query")
	}

	pages := getMap(query["pages"])
	if pages == nil {
		return GetPagesInfoBatchResult{}, fmt.Errorf("unexpected response format: missing pages")
	}

	for _, pageData := range pages {
		page := getMap(pageData)
		if page == nil {
			continue
		}

		title := getString(page["title"])
		info := PageInfo{
			Title: title,
		}

		// Check if page exists
		if _, missing := page["missing"]; missing {
			info.Exists = false
			result.MissingCount++
			result.Pages = append(result.Pages, info)
			continue
		}

		info.Exists = true
		info.PageID = getInt(page["pageid"])
		info.Namespace = getInt(page["ns"])
		info.ContentModel = getString(page["contentmodel"])
		info.PageLanguage = getString(page["pagelanguage"])
		info.Length = getInt(page["length"])
		info.Touched = getString(page["touched"])
		info.LastRevision = getInt(page["lastrevid"])
		result.ExistsCount++

		// Categories
		if cats := getSlice(page["categories"]); cats != nil {
			for _, cat := range cats {
				catMap := getMap(cat)
				if catMap != nil {
					info.Categories = append(info.Categories, getString(catMap["title"]))
				}
			}
		}

		// Redirect
		if _, isRedirect := page["redirect"]; isRedirect {
			info.Redirect = true
		}

		// Protection
		if protection := getSlice(page["protection"]); protection != nil {
			for _, p := range protection {
				prot := getMap(p)
				if prot != nil {
					protType := getString(prot["type"])
					protLevel := getString(prot["level"])
					info.Protection = append(info.Protection, fmt.Sprintf("%s: %s", protType, protLevel))
				}
			}
		}

		result.Pages = append(result.Pages, info)
	}

	return result, nil
}

// SearchAndRead searches the wiki and reads the top result(s) in one operation.
// This eliminates the most common two-call pattern: search then get_page.
func (c *Client) SearchAndRead(ctx context.Context, args SearchAndReadArgs) (SearchAndReadResult, error) {
	if args.Query == "" {
		return SearchAndReadResult{}, fmt.Errorf("query is required")
	}

	readCount := args.ReadCount
	if readCount <= 0 {
		readCount = 1
	}
	if readCount > 5 {
		readCount = 5
	}

	format := args.Format
	if format == "" {
		format = "wikitext"
	}

	// Step 1: Search
	searchResult, err := c.Search(ctx, SearchArgs{
		Query: args.Query,
		Limit: 10,
	})
	if err != nil {
		return SearchAndReadResult{}, fmt.Errorf("search failed: %w", err)
	}

	result := SearchAndReadResult{
		Query:     args.Query,
		TotalHits: searchResult.TotalHits,
	}

	if len(searchResult.Results) == 0 {
		result.Message = fmt.Sprintf("No pages found for '%s'", args.Query)
		return result, nil
	}

	// Step 2: Read top N results
	toRead := readCount
	if toRead > len(searchResult.Results) {
		toRead = len(searchResult.Results)
	}

	result.Pages = make([]SearchAndReadPage, 0, toRead)
	for i := 0; i < toRead; i++ {
		hit := searchResult.Results[i]

		page, err := c.GetPage(ctx, GetPageArgs{Title: hit.Title, Format: format})
		if err != nil {
			// Include the hit but without content
			result.Pages = append(result.Pages, SearchAndReadPage{
				Title:   hit.Title,
				PageID:  hit.PageID,
				Snippet: hit.Snippet,
			})
			continue
		}

		result.Pages = append(result.Pages, SearchAndReadPage{
			Title:     page.Title,
			PageID:    page.PageID,
			Snippet:   hit.Snippet,
			Content:   page.Content,
			Format:    page.Format,
			Revision:  page.Revision,
			Timestamp: page.Timestamp,
			Truncated: page.Truncated,
		})
	}

	// Step 3: Include remaining search hits as summaries
	if len(searchResult.Results) > toRead {
		result.OtherHits = searchResult.Results[toRead:]
	}

	result.Message = fmt.Sprintf("Read %d of %d results for '%s'", len(result.Pages), searchResult.TotalHits, args.Query)
	return result, nil
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
