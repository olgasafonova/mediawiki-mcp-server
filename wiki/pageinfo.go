package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

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

	params := buildListPagesParams(args)
	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return ListPagesResult{}, err
	}

	query := getMap(resp["query"])
	if query == nil {
		return ListPagesResult{}, fmt.Errorf("unexpected response format: missing query")
	}

	pages := parsePageSummaries(getSlice(query["allpages"]))
	result := ListPagesResult{
		Pages:         pages,
		ReturnedCount: len(pages),
		TotalCount:    len(pages), // Deprecated: kept for backwards compatibility
	}
	applyContinuation(resp, &result)

	// Try to get namespace statistics for total estimate (only when no prefix filter)
	if args.Prefix == "" && args.Namespace >= 0 {
		if estimate := c.getNamespacePageCount(ctx, args.Namespace); estimate > 0 {
			result.TotalEstimate = estimate
		}
	}

	return result, nil
}

// buildListPagesParams assembles the allpages query parameters from args.
func buildListPagesParams(args ListPagesArgs) url.Values {
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
	return params
}

// parsePageSummaries converts allpages entries into PageSummary values.
func parsePageSummaries(allpages []interface{}) []PageSummary {
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
	return pages
}

// applyContinuation sets HasMore/ContinueFrom from the response's continue
// block, if present.
func applyContinuation(resp map[string]interface{}, result *ListPagesResult) {
	cont := getMap(resp["continue"])
	if cont == nil {
		return
	}
	if apcontinue := getString(cont["apcontinue"]); apcontinue != "" {
		result.HasMore = true
		result.ContinueFrom = apcontinue
	}
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

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return PageInfo{}, fmt.Errorf("unexpected API response: missing 'query' object")
	}
	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return PageInfo{}, fmt.Errorf("unexpected API response: missing 'pages' object")
	}

	info, found := firstPageInfo(pages, args.Title)
	if !found {
		return PageInfo{}, fmt.Errorf("page '%s' not found", normalizedTitle)
	}
	if info.Exists {
		c.setCache(cacheKey, info, "page_info")
	}
	return info, nil
}

// firstPageInfo returns the PageInfo for the first valid page object in the
// response map. found is false when no usable page object is present. A page
// flagged missing yields a non-existent PageInfo carrying the requested title.
func firstPageInfo(pages map[string]interface{}, requestedTitle string) (info PageInfo, found bool) {
	for _, pageData := range pages {
		page, ok := pageData.(map[string]interface{})
		if !ok {
			continue
		}
		if _, missing := page["missing"]; missing {
			return PageInfo{Title: requestedTitle, Exists: false}, true
		}
		return buildDetailedPageInfo(page), true
	}
	return PageInfo{}, false
}

// buildDetailedPageInfo converts an existing page's metadata object into a
// PageInfo, including categories, link count, redirect flag, and protection.
func buildDetailedPageInfo(page map[string]interface{}) PageInfo {
	info := PageInfo{
		Title:        getString(page["title"]),
		PageID:       getInt(page["pageid"]),
		Namespace:    getInt(page["ns"]),
		ContentModel: getString(page["contentmodel"]),
		PageLanguage: getString(page["pagelanguage"]),
		Length:       getInt(page["length"]),
		Touched:      getString(page["touched"]),
		LastRevision: getInt(page["lastrevid"]),
		Exists:       true,
	}

	info.Categories = extractCategoryTitles(page["categories"])
	if links, ok := page["links"].([]interface{}); ok {
		info.Links = len(links)
	}
	if _, isRedirect := page["redirect"]; isRedirect {
		info.Redirect = true
	}
	info.Protection = extractProtectionEntries(page["protection"])
	return info
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

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return WikiInfo{}, fmt.Errorf("unexpected API response: missing 'query' object")
	}
	general, ok := query["general"].(map[string]interface{})
	if !ok {
		return WikiInfo{}, fmt.Errorf("unexpected API response: missing 'general' object")
	}

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

	result.Suggestions = rankTitleSuggestions(args.Title, searchResult.Results)
	if len(result.Suggestions) > maxResults {
		result.Suggestions = result.Suggestions[:maxResults]
	}
	applyResolveMessage(args.Title, &result)
	return result, nil
}

// rankTitleSuggestions scores each search hit against the query title and
// returns suggestions sorted by descending similarity.
func rankTitleSuggestions(title string, hits []SearchHit) []TitleSuggestion {
	titleLower := strings.ToLower(title)
	suggestions := make([]TitleSuggestion, 0, len(hits))
	for _, hit := range hits {
		suggestions = append(suggestions, TitleSuggestion{
			Title:      hit.Title,
			PageID:     hit.PageID,
			Similarity: calculateSimilarity(titleLower, strings.ToLower(hit.Title)),
		})
	}

	// Sort by similarity (descending)
	for i := 0; i < len(suggestions)-1; i++ {
		for j := i + 1; j < len(suggestions); j++ {
			if suggestions[j].Similarity > suggestions[i].Similarity {
				suggestions[i], suggestions[j] = suggestions[j], suggestions[i]
			}
		}
	}
	return suggestions
}

// applyResolveMessage sets ResolvedTitle/PageID/Message based on the best
// suggestion's similarity.
func applyResolveMessage(title string, result *ResolveTitleResult) {
	switch {
	case len(result.Suggestions) > 0 && result.Suggestions[0].Similarity > 0.8:
		result.ResolvedTitle = result.Suggestions[0].Title
		result.PageID = result.Suggestions[0].PageID
		result.Message = fmt.Sprintf("Did you mean '%s'?", result.Suggestions[0].Title)
	case len(result.Suggestions) > 0:
		result.Message = fmt.Sprintf("Page '%s' not found. Similar pages found.", title)
	default:
		result.Message = fmt.Sprintf("Page '%s' not found. No similar pages.", title)
	}
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
