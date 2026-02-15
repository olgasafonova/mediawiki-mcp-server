package wiki

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

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
				linkURL := getString(link["*"])
				if linkURL == "" {
					linkURL = getString(link["url"])
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
	requestTimeout := time.Duration(timeout) * time.Second

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
				linkResult.Error = fmt.Sprintf("[%s] Invalid URL format", SSRFCodeInvalidURL)
				linkResult.Broken = true
			} else if hostname := parsedURL.Hostname(); hostname != "" {
				// SSRF protection: block private/internal IPs
				isPrivate, ssrfErr := isPrivateHost(hostname)
				if isPrivate {
					linkResult.Status = "blocked"
					if ssrfErr != nil {
						// DNS error - use the structured error message
						linkResult.Error = ssrfErr.Error()
					} else {
						// Private IP detected
						linkResult.Error = fmt.Sprintf("[%s] URLs pointing to private/internal networks are not allowed", SSRFCodePrivateIP)
					}
					linkResult.Broken = true
				} else {
					// Create per-request context with timeout
					reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)

					// Make HEAD request first (faster)
					req, _ := http.NewRequestWithContext(reqCtx, "HEAD", checkURL, nil)
					req.Header.Set("User-Agent", "MediaWiki-MCP-LinkChecker/1.0")

					resp, err := linkCheckClient.Do(req) // #nosec G704 -- link checker intentionally fetches external URLs
					if err != nil {
						// Try GET if HEAD fails (some servers don't support HEAD)
						req, _ = http.NewRequestWithContext(reqCtx, "GET", checkURL, nil)
						req.Header.Set("User-Agent", "MediaWiki-MCP-LinkChecker/1.0")
						resp, err = linkCheckClient.Do(req) // #nosec G704 -- link checker intentionally fetches external URLs to verify they're alive
					}

					cancel() // Release context resources

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
			PageID:    getInt(link["pageid"]),
			Title:     getString(link["title"]),
			Namespace: getInt(link["ns"]),
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
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

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

	// Collect filtered titles
	var filteredTitles []string
	for _, r := range results {
		page := r.(map[string]interface{})

		// Filter by namespace if specified
		ns := getInt(page["ns"])
		if args.Namespace >= 0 && ns != args.Namespace {
			continue
		}

		title := getString(page["title"])

		// Filter by prefix if specified
		if args.Prefix != "" && !strings.HasPrefix(title, args.Prefix) {
			continue
		}

		filteredTitles = append(filteredTitles, title)
	}

	// Fetch actual page info (pageid, length, touched) for the filtered pages
	orphaned := make([]OrphanedPage, 0, len(filteredTitles))
	if len(filteredTitles) > 0 {
		// Batch fetch page info (up to 50 at a time)
		for i := 0; i < len(filteredTitles); i += 50 {
			end := i + 50
			if end > len(filteredTitles) {
				end = len(filteredTitles)
			}
			batch := filteredTitles[i:end]

			infoParams := url.Values{}
			infoParams.Set("action", "query")
			infoParams.Set("titles", strings.Join(batch, "|"))
			infoParams.Set("prop", "info")

			infoResp, err := c.apiRequest(ctx, infoParams)
			if err != nil {
				// Fall back to basic info if fetch fails
				for _, title := range batch {
					orphaned = append(orphaned, OrphanedPage{Title: title})
				}
				continue
			}

			infoQuery, _ := infoResp["query"].(map[string]interface{})
			pages, _ := infoQuery["pages"].(map[string]interface{})

			for _, pageData := range pages {
				p := pageData.(map[string]interface{})
				orphaned = append(orphaned, OrphanedPage{
					Title:      getString(p["title"]),
					PageID:     getInt(p["pageid"]),
					Length:     getInt(p["length"]),
					LastEdited: getString(p["touched"]),
				})
			}
		}
	}

	return FindOrphanedPagesResult{
		OrphanedPages: orphaned,
		TotalChecked:  len(results),
		OrphanedCount: len(orphaned),
	}, nil
}
