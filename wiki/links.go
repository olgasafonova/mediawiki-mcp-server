package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"sync"
)

// GetExternalLinks retrieves external links from a wiki page
// extractExternalLinkFromEntry pulls the URL+scheme from one extlinks entry.
// Returns ok=false if the entry is malformed or has no URL.
func extractExternalLinkFromEntry(entry interface{}) (ExternalLink, bool) {
	link, ok := entry.(map[string]interface{})
	if !ok {
		return ExternalLink{}, false
	}
	linkURL := getString(link["*"])
	if linkURL == "" {
		linkURL = getString(link["url"])
	}
	if linkURL == "" {
		return ExternalLink{}, false
	}
	protocol := ""
	if u, err := url.Parse(linkURL); err == nil {
		protocol = u.Scheme
	}
	return ExternalLink{URL: linkURL, Protocol: protocol}, true
}

// extractExternalLinksFromPage walks one page's extlinks list and returns the
// ExternalLink slice. Returns the page title as well so the caller can echo it back.
func extractExternalLinksFromPage(page map[string]interface{}) (string, []ExternalLink) {
	links := make([]ExternalLink, 0)
	pageTitle := getString(page["title"])
	extlinks, ok := page["extlinks"].([]interface{})
	if !ok {
		return pageTitle, links
	}
	for _, el := range extlinks {
		if link, ok := extractExternalLinkFromEntry(el); ok {
			links = append(links, link)
		}
	}
	return pageTitle, links
}

func (c *Client) GetExternalLinks(ctx context.Context, args GetExternalLinksArgs) (ExternalLinksResult, error) {
	if args.Title == "" {
		return ExternalLinksResult{}, fmt.Errorf("title is required")
	}
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

	for _, pageData := range pages {
		page, ok := pageData.(map[string]interface{})
		if !ok {
			continue
		}
		if _, missing := page["missing"]; missing {
			return ExternalLinksResult{}, fmt.Errorf("page '%s' does not exist", args.Title)
		}
		title, links := extractExternalLinksFromPage(page)
		return ExternalLinksResult{Title: title, Links: links, Count: len(links)}, nil
	}
	return ExternalLinksResult{Links: make([]ExternalLink, 0)}, nil
}

// GetExternalLinksBatch retrieves external links from multiple wiki pages
// Uses a worker pool pattern to limit concurrent API requests
func (c *Client) GetExternalLinksBatch(ctx context.Context, args GetExternalLinksBatchArgs) (ExternalLinksBatchResult, error) {
	if len(args.Titles) == 0 {
		return ExternalLinksBatchResult{}, fmt.Errorf("at least one title is required")
	}

	// Limit batch size to prevent overwhelming the API
	maxBatch := 10
	if len(args.Titles) > maxBatch {
		args.Titles = args.Titles[:maxBatch]
	}

	// Worker pool configuration
	numWorkers := 4 // Limit concurrent API requests
	if len(args.Titles) < numWorkers {
		numWorkers = len(args.Titles)
	}

	// Job and result types
	type job struct {
		index int
		title string
	}
	type pageResult struct {
		index int
		data  PageExternalLinks
	}

	jobs := make(chan job, len(args.Titles))
	results := make(chan pageResult, len(args.Titles))

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				// Check context cancellation
				select {
				case <-ctx.Done():
					results <- pageResult{
						index: j.index,
						data: PageExternalLinks{
							Title: j.title,
							Links: make([]ExternalLink, 0),
							Error: "request canceled",
						},
					}
					continue
				default:
				}

				pageLinks, err := c.GetExternalLinks(ctx, GetExternalLinksArgs{Title: j.title})
				if err != nil {
					results <- pageResult{
						index: j.index,
						data: PageExternalLinks{
							Title: j.title,
							Links: make([]ExternalLink, 0),
							Error: err.Error(),
						},
					}
					continue
				}

				results <- pageResult{
					index: j.index,
					data: PageExternalLinks{
						Title: pageLinks.Title,
						Links: pageLinks.Links,
						Count: pageLinks.Count,
					},
				}
			}
		}()
	}

	// Send jobs to workers
	for i, title := range args.Titles {
		jobs <- job{index: i, title: title}
	}
	close(jobs)

	// Close results channel when all workers complete
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
// validateLinkURLForCheck parses a URL and rejects unsupported schemes or
// targets pointing at private/internal hosts. Returns a populated LinkCheckResult
// (with Broken=true) when validation fails; ok=true means the URL passed.
// buildBacklinksParams composes the API params for a backlinks query.
func buildBacklinksParams(args GetBacklinksArgs, limit int) url.Values {
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
	return params
}

// backlinkInfoFromEntry converts one raw backlinks entry into a BacklinkInfo.
func backlinkInfoFromEntry(entry interface{}) (BacklinkInfo, bool) {
	link, ok := entry.(map[string]interface{})
	if !ok {
		return BacklinkInfo{}, false
	}
	info := BacklinkInfo{
		PageID:    getInt(link["pageid"]),
		Title:     getString(link["title"]),
		Namespace: getInt(link["ns"]),
	}
	if _, isRedirect := link["redirect"]; isRedirect {
		info.IsRedirect = true
	}
	return info, true
}

func (c *Client) GetBacklinks(ctx context.Context, args GetBacklinksArgs) (GetBacklinksResult, error) {
	if args.Title == "" {
		return GetBacklinksResult{}, fmt.Errorf("title is required")
	}
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetBacklinksResult{}, err
	}

	limit := normalizeLimit(args.Limit, 50, MaxLimit)
	resp, err := c.apiRequest(ctx, buildBacklinksParams(args, limit))
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
		if info, ok := backlinkInfoFromEntry(bl); ok {
			result.Backlinks = append(result.Backlinks, info)
		}
	}
	result.Count = len(result.Backlinks)
	if _, ok := resp["continue"]; ok {
		result.HasMore = true
	}
	return result, nil
}

// FindBrokenInternalLinks finds internal wiki links that point to non-existent pages
// internalLinkRegex matches "[[Target]]" or "[[Target|Display]]" wiki links.
