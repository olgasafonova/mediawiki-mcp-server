package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

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
