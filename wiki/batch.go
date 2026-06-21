package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// MaxBatchSize is the maximum number of pages that can be fetched in a single batch
const MaxBatchSize = 50

type pageBuildStatus int

const (
	pageStatusOK pageBuildStatus = iota
	pageStatusMissing
	pageStatusError
)

// extractMainContent navigates revisions[0].slots.main and returns the content
// string plus revision metadata. Returns ("", "", 0, errMsg) on shape errors.
func extractMainContent(page map[string]interface{}) (content, timestamp string, revid int, errMsg string) {
	revisions := getSlice(page["revisions"])
	if len(revisions) == 0 {
		return "", "", 0, "no revisions found"
	}
	rev := getMap(revisions[0])
	if rev == nil {
		return "", "", 0, "invalid revision data"
	}
	slots := getMap(rev["slots"])
	if slots == nil {
		return "", "", 0, "invalid slots data"
	}
	main := getMap(slots["main"])
	if main == nil {
		return "", "", 0, "invalid main slot"
	}
	content = getString(main["*"])
	if content == "" {
		content = getString(main["content"])
	}
	return content, getString(rev["timestamp"]), getInt(rev["revid"]), ""
}

// buildPageContentResult converts one MediaWiki page object into a
// PageContentResult and reports its status (OK, missing, error).
func buildPageContentResult(page map[string]interface{}, format string) (PageContentResult, pageBuildStatus) {
	pageResult := PageContentResult{
		Title:  getString(page["title"]),
		Format: format,
	}
	if _, missing := page["missing"]; missing {
		pageResult.Exists = false
		return pageResult, pageStatusMissing
	}
	pageResult.Exists = true
	pageResult.PageID = getInt(page["pageid"])

	content, timestamp, revid, errMsg := extractMainContent(page)
	if errMsg != "" {
		pageResult.Error = errMsg
		return pageResult, pageStatusError
	}
	pageResult.Content = content
	pageResult.Revision = revid
	pageResult.Timestamp = timestamp
	if len(content) > CharacterLimit {
		truncated, _ := truncateContent(content, CharacterLimit)
		pageResult.Content = truncated
		pageResult.Truncated = true
	}
	return pageResult, pageStatusOK
}

// capBatchTitles validates the title list is non-empty and caps it at
// MaxBatchSize, returning a descriptive error when empty.
func capBatchTitles(titles []string) ([]string, error) {
	if len(titles) == 0 {
		return nil, fmt.Errorf("at least one title is required")
	}
	if len(titles) > MaxBatchSize {
		return titles[:MaxBatchSize], nil
	}
	return titles, nil
}

// normalizeTitles returns the per-title normalized form, preserving order.
func normalizeTitles(titles []string) []string {
	normalized := make([]string, len(titles))
	for i, t := range titles {
		normalized[i] = normalizePageTitle(t)
	}
	return normalized
}

// extractQueryPages pulls the query and pages objects out of an API response,
// returning shape errors labeled with the given context.
func extractQueryPages(resp map[string]interface{}, label string) (query, pages map[string]interface{}, err error) {
	query = getMap(resp["query"])
	if query == nil {
		return nil, nil, fmt.Errorf("%s: missing query", label)
	}
	pages = getMap(query["pages"])
	if pages == nil {
		return nil, nil, fmt.Errorf("%s: missing pages", label)
	}
	return query, pages, nil
}

// GetPagesBatch retrieves content for multiple pages in a single API call.
// This is significantly more efficient than calling GetPage individually.
func (c *Client) GetPagesBatch(ctx context.Context, args GetPagesBatchArgs) (GetPagesBatchResult, error) {
	titles, err := capBatchTitles(args.Titles)
	if err != nil {
		return GetPagesBatchResult{}, err
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

	// MediaWiki API accepts pipe-separated titles
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", strings.Join(normalizeTitles(titles), "|"))
	params.Set("prop", "revisions")
	params.Set("rvprop", "content|ids|timestamp")
	params.Set("rvslots", "main")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetPagesBatchResult{}, fmt.Errorf("API request failed: %w", err)
	}
	query, pages, err := extractQueryPages(resp, "unexpected API response")
	if err != nil {
		return GetPagesBatchResult{}, err
	}

	foundTitles := collectPageContentResults(pages, format, &result)
	applyNormalizedMappings(query, foundTitles)
	return result, nil
}

// collectPageContentResults builds a PageContentResult for each page, updating
// the batch counters, and returns the set of titles seen.
func collectPageContentResults(pages map[string]interface{}, format string, result *GetPagesBatchResult) map[string]bool {
	foundTitles := make(map[string]bool)
	for _, pageData := range pages {
		page := getMap(pageData)
		if page == nil {
			continue
		}
		pageResult, status := buildPageContentResult(page, format)
		switch status {
		case pageStatusMissing:
			result.MissingCount++
		case pageStatusOK:
			result.FoundCount++
		}
		foundTitles[pageResult.Title] = true
		result.Pages = append(result.Pages, pageResult)
	}
	return foundTitles
}

// applyNormalizedMappings propagates found-status across MediaWiki's
// normalized title mappings (from -> to).
func applyNormalizedMappings(query map[string]interface{}, foundTitles map[string]bool) {
	for _, n := range getSlice(query["normalized"]) {
		normMap := getMap(n)
		if normMap == nil {
			continue
		}
		from := getString(normMap["from"])
		to := getString(normMap["to"])
		if from != "" && to != "" {
			foundTitles[from] = foundTitles[to]
		}
	}
}

// GetPagesInfoBatch retrieves metadata for multiple pages in a single API call.
func (c *Client) GetPagesInfoBatch(ctx context.Context, args GetPagesInfoBatchArgs) (GetPagesInfoBatchResult, error) {
	titles, err := capBatchTitles(args.Titles)
	if err != nil {
		return GetPagesInfoBatchResult{}, err
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetPagesInfoBatchResult{}, err
	}

	result := GetPagesInfoBatchResult{
		Pages:      make([]PageInfo, 0, len(titles)),
		TotalCount: len(titles),
	}

	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", strings.Join(normalizeTitles(titles), "|"))
	params.Set("prop", "info|categories")
	params.Set("inprop", "protection|url")
	params.Set("cllimit", "50")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetPagesInfoBatchResult{}, err
	}

	_, pages, err := extractQueryPages(resp, "unexpected response format")
	if err != nil {
		return GetPagesInfoBatchResult{}, err
	}

	collectPageInfos(pages, &result)
	return result, nil
}

// collectPageInfos builds a PageInfo for each page and updates the batch
// exists/missing counters.
func collectPageInfos(pages map[string]interface{}, result *GetPagesInfoBatchResult) {
	for _, pageData := range pages {
		page := getMap(pageData)
		if page == nil {
			continue
		}
		info, exists := buildPageInfo(page)
		if exists {
			result.ExistsCount++
		} else {
			result.MissingCount++
		}
		result.Pages = append(result.Pages, info)
	}
}

// buildPageInfo converts one MediaWiki page metadata object into a PageInfo and
// reports whether the page exists.
func buildPageInfo(page map[string]interface{}) (PageInfo, bool) {
	info := PageInfo{Title: getString(page["title"])}

	if _, missing := page["missing"]; missing {
		info.Exists = false
		return info, false
	}

	info.Exists = true
	info.PageID = getInt(page["pageid"])
	info.Namespace = getInt(page["ns"])
	info.ContentModel = getString(page["contentmodel"])
	info.PageLanguage = getString(page["pagelanguage"])
	info.Length = getInt(page["length"])
	info.Touched = getString(page["touched"])
	info.LastRevision = getInt(page["lastrevid"])
	info.Categories = extractCategoryTitles(page["categories"])
	if _, isRedirect := page["redirect"]; isRedirect {
		info.Redirect = true
	}
	info.Protection = extractProtectionEntries(page["protection"])
	return info, true
}

// extractCategoryTitles returns the category titles from a page's categories
// slice.
func extractCategoryTitles(raw interface{}) []string {
	var titles []string
	for _, cat := range getSlice(raw) {
		if catMap := getMap(cat); catMap != nil {
			titles = append(titles, getString(catMap["title"]))
		}
	}
	return titles
}

// extractProtectionEntries returns "type: level" strings from a page's
// protection slice.
func extractProtectionEntries(raw interface{}) []string {
	var entries []string
	for _, p := range getSlice(raw) {
		if prot := getMap(p); prot != nil {
			entries = append(entries, fmt.Sprintf("%s: %s", getString(prot["type"]), getString(prot["level"])))
		}
	}
	return entries
}

// SearchAndRead searches the wiki and reads the top result(s) in one operation.
// This eliminates the most common two-call pattern: search then get_page.
func (c *Client) SearchAndRead(ctx context.Context, args SearchAndReadArgs) (SearchAndReadResult, error) {
	if args.Query == "" {
		return SearchAndReadResult{}, fmt.Errorf("query is required")
	}

	readCount := clampReadCount(args.ReadCount)

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
	result.Pages = c.readSearchHits(ctx, searchResult.Results[:toRead], format)

	// Step 3: Include remaining search hits as summaries
	if len(searchResult.Results) > toRead {
		result.OtherHits = searchResult.Results[toRead:]
	}

	result.Message = fmt.Sprintf("Read %d of %d results for '%s'", len(result.Pages), searchResult.TotalHits, args.Query)
	return result, nil
}

// clampReadCount bounds the requested read count to the supported 1..5 range.
func clampReadCount(n int) int {
	switch {
	case n <= 0:
		return 1
	case n > 5:
		return 5
	default:
		return n
	}
}

// readSearchHits fetches each hit's page content, falling back to a snippet-only
// entry when a page read fails.
func (c *Client) readSearchHits(ctx context.Context, hits []SearchHit, format string) []SearchAndReadPage {
	pages := make([]SearchAndReadPage, 0, len(hits))
	for _, hit := range hits {
		page, err := c.GetPage(ctx, GetPageArgs{Title: hit.Title, Format: format})
		if err != nil {
			pages = append(pages, SearchAndReadPage{
				Title:   hit.Title,
				PageID:  hit.PageID,
				Snippet: hit.Snippet,
			})
			continue
		}
		pages = append(pages, SearchAndReadPage{
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
	return pages
}
