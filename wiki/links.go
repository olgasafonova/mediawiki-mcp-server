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
func validateLinkURLForCheck(rawURL string) (LinkCheckResult, bool) {
	r := LinkCheckResult{URL: rawURL}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		r.Status = "invalid_url"
		r.Error = fmt.Sprintf("[%s] Invalid URL format", SSRFCodeInvalidURL)
		r.Broken = true
		return r, false
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		// No hostname to test for SSRF; treat as caller's responsibility but pass.
		return r, true
	}
	isPrivate, ssrfErr := isPrivateHost(hostname)
	if !isPrivate {
		return r, true
	}
	r.Status = "blocked"
	if ssrfErr != nil {
		r.Error = ssrfErr.Error()
	} else {
		r.Error = fmt.Sprintf("[%s] URLs pointing to private/internal networks are not allowed", SSRFCodePrivateIP)
	}
	r.Broken = true
	return r, false
}

// fetchLinkStatus issues a HEAD request, falling back to GET if the server
// rejects HEAD, and writes the resulting status onto r. Marks Broken=true if
// the request fails or returns 4xx/5xx.
func fetchLinkStatus(ctx context.Context, rawURL string, timeout time.Duration, r *LinkCheckResult) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	makeRequest := func(method string) (*http.Response, error) {
		req, _ := http.NewRequestWithContext(reqCtx, method, rawURL, nil)
		req.Header.Set("User-Agent", "MediaWiki-MCP-LinkChecker/1.0")
		return linkCheckClient.Do(req) // #nosec G704 -- link checker intentionally fetches external URLs
	}

	resp, err := makeRequest("HEAD")
	if err != nil {
		resp, err = makeRequest("GET")
	}
	if err != nil {
		r.Status = "error"
		r.Error = err.Error()
		r.Broken = true
		return
	}
	_ = resp.Body.Close()
	r.StatusCode = resp.StatusCode
	r.Status = resp.Status
	if resp.StatusCode >= 400 {
		r.Broken = true
	}
}

// checkSingleLink performs validation + status fetch for a single URL.
func checkSingleLink(ctx context.Context, rawURL string, timeout time.Duration) LinkCheckResult {
	r, ok := validateLinkURLForCheck(rawURL)
	if !ok {
		return r
	}
	fetchLinkStatus(ctx, rawURL, timeout, &r)
	return r
}

// resolveLinkCheckTimeout clamps the user-supplied timeout to the safe range.
func resolveLinkCheckTimeout(requested int) time.Duration {
	timeout := 10
	if requested > 0 && requested <= 30 {
		timeout = requested
	}
	return time.Duration(timeout) * time.Second
}

func (c *Client) CheckLinks(ctx context.Context, args CheckLinksArgs) (CheckLinksResult, error) {
	if len(args.URLs) == 0 {
		return CheckLinksResult{}, fmt.Errorf("at least one URL is required")
	}

	const maxURLs = 20
	if len(args.URLs) > maxURLs {
		args.URLs = args.URLs[:maxURLs]
	}
	requestTimeout := resolveLinkCheckTimeout(args.Timeout)

	result := CheckLinksResult{
		Results:    make([]LinkCheckResult, 0, len(args.URLs)),
		TotalLinks: len(args.URLs),
	}

	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, linkURL := range args.URLs {
		wg.Add(1)
		go func(rawURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			linkResult := checkSingleLink(ctx, rawURL, requestTimeout)
			mu.Lock()
			defer mu.Unlock()
			result.Results = append(result.Results, linkResult)
			if linkResult.Broken {
				result.BrokenCount++
			} else {
				result.ValidCount++
			}
		}(linkURL)
	}
	wg.Wait()
	return result, nil
}

// GetBacklinks returns pages that link to the specified page ("What links here")
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
var internalLinkRegex = regexp.MustCompile(`\[\[([^\]|#]+)(?:[|#][^\]]*)?]]`)

// internalLinkSkipPrefixes lists the lower-cased prefixes that indicate a link
// target is not an internal page reference (categories, files, interwiki,
// explicit-link prefix, external URL).
var internalLinkSkipPrefixes = []string{"category:", "file:", "image:", ":", "http"}

// isInternalLinkTarget reports whether a wiki link target should be treated as
// an internal page reference for broken-link checking.
func isInternalLinkTarget(target string) bool {
	lower := strings.ToLower(target)
	for _, prefix := range internalLinkSkipPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	return true
}

// linkLocation records a single link occurrence on a page.
type linkLocation struct {
	pageTitle string
	target    string
	line      int
	context   string
}

// extractInternalLinks pulls every internal page-reference link out of a single
// line, with the caller-supplied page title and line number stamped onto each.
func extractInternalLinks(pageTitle, line string, lineNum int) []linkLocation {
	var out []linkLocation
	for _, match := range internalLinkRegex.FindAllStringSubmatch(line, -1) {
		if len(match) < 2 {
			continue
		}
		target := strings.TrimSpace(match[1])
		if !isInternalLinkTarget(target) {
			continue
		}
		idx := strings.Index(line, match[0])
		out = append(out, linkLocation{
			pageTitle: pageTitle,
			target:    target,
			line:      lineNum + 1,
			context:   extractContext(line, idx, idx+len(match[0]), 30),
		})
	}
	return out
}

// collectInternalLinkLocations fetches each page and extracts its internal
// link locations. Pages that fail to fetch produce error entries in the result;
// successfully-fetched pages are recorded in fetched so the caller can build
// per-page result rows for them.
func (c *Client) collectInternalLinkLocations(ctx context.Context, pages []string) (locations []linkLocation, fetched map[string]struct{}, errResults []PageBrokenLinksResult, err error) {
	fetched = make(map[string]struct{}, len(pages))
	for _, pageTitle := range pages {
		select {
		case <-ctx.Done():
			return locations, fetched, errResults, ctx.Err()
		default:
		}
		page, fetchErr := c.GetPage(ctx, GetPageArgs{Title: pageTitle, Format: "wikitext"})
		if fetchErr != nil {
			errResults = append(errResults, PageBrokenLinksResult{
				Title:       pageTitle,
				BrokenLinks: make([]BrokenLink, 0),
				Error:       fetchErr.Error(),
			})
			continue
		}
		fetched[pageTitle] = struct{}{}
		for lineNum, line := range strings.Split(page.Content, "\n") {
			locations = append(locations, extractInternalLinks(pageTitle, line, lineNum)...)
		}
	}
	return locations, fetched, errResults, nil
}

// uniqueLinkTargets returns the deduplicated set of target titles from all
// link locations.
func uniqueLinkTargets(locations []linkLocation) []string {
	set := make(map[string]struct{})
	for _, loc := range locations {
		set[loc.target] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for target := range set {
		out = append(out, target)
	}
	return out
}

// buildBrokenLinksResults turns link locations + existence-map into per-page
// PageBrokenLinksResult rows. Within each page, only the first occurrence of
// each broken target is reported.
func buildBrokenLinksResults(pages []string, fetched map[string]struct{}, locations []linkLocation, existence map[string]bool) []PageBrokenLinksResult {
	pageResults := make(map[string]*PageBrokenLinksResult, len(fetched))
	for _, title := range pages {
		if _, ok := fetched[title]; ok {
			pageResults[title] = &PageBrokenLinksResult{
				Title:       title,
				BrokenLinks: make([]BrokenLink, 0),
			}
		}
	}

	seen := make(map[string]map[string]bool)
	for _, loc := range locations {
		pr := pageResults[loc.pageTitle]
		if pr == nil {
			continue
		}
		if seen[loc.pageTitle] == nil {
			seen[loc.pageTitle] = make(map[string]bool)
		}
		if seen[loc.pageTitle][loc.target] {
			continue
		}
		seen[loc.pageTitle][loc.target] = true

		exists, ok := existence[loc.target]
		if !ok || !exists {
			pr.BrokenLinks = append(pr.BrokenLinks, BrokenLink{
				Target:  loc.target,
				Line:    loc.line,
				Context: loc.context,
			})
		}
	}

	out := make([]PageBrokenLinksResult, 0, len(pageResults))
	for _, title := range pages {
		if pr, ok := pageResults[title]; ok {
			pr.BrokenCount = len(pr.BrokenLinks)
			out = append(out, *pr)
		}
	}
	return out
}

func (c *Client) FindBrokenInternalLinks(ctx context.Context, args FindBrokenInternalLinksArgs) (FindBrokenInternalLinksResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return FindBrokenInternalLinksResult{}, err
	}

	limit := normalizeLimit(args.Limit, 20, 100)
	pagesToCheck, err := c.collectPagesFromArgs(ctx, args.Pages, args.Category, limit, "pages")
	if err != nil {
		return FindBrokenInternalLinksResult{}, err
	}

	locations, fetched, errResults, err := c.collectInternalLinkLocations(ctx, pagesToCheck)
	if err != nil {
		// Context cancellation: return what we have so far.
		return FindBrokenInternalLinksResult{
			Pages: append(errResults, buildBrokenLinksResults(pagesToCheck, fetched, locations, nil)...),
		}, err
	}

	existence, err := c.checkPagesExist(ctx, uniqueLinkTargets(locations))
	if err != nil {
		return FindBrokenInternalLinksResult{}, fmt.Errorf("failed to check page existence: %w", err)
	}

	successResults := buildBrokenLinksResults(pagesToCheck, fetched, locations, existence)

	result := FindBrokenInternalLinksResult{
		Pages: make([]PageBrokenLinksResult, 0, len(errResults)+len(successResults)),
	}
	result.Pages = append(result.Pages, errResults...)
	result.Pages = append(result.Pages, successResults...)
	for _, pr := range successResults {
		result.BrokenCount += pr.BrokenCount
	}
	result.PagesChecked = len(result.Pages)
	return result, nil
}

// FindOrphanedPages finds pages that have no incoming links from other pages
// queryLonelyPages calls the Lonelypages querypage and returns the raw page entries.
func (c *Client) queryLonelyPages(ctx context.Context, limit int) ([]interface{}, error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "querypage")
	params.Set("qppage", "Lonelypages")
	params.Set("qplimit", strconv.Itoa(limit))

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return nil, err
	}
	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format")
	}
	querypage, ok := query["querypage"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("querypage not found in response")
	}
	results, ok := querypage["results"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("results not found in querypage")
	}
	return results, nil
}

// orphanedPageMatchesFilter reports whether the page entry passes the namespace
// and title-prefix filters, returning the page's title when it does.
func orphanedPageMatchesFilter(entry interface{}, namespace int, prefix string) (string, bool) {
	page, ok := entry.(map[string]interface{})
	if !ok {
		return "", false
	}
	if namespace >= 0 && getInt(page["ns"]) != namespace {
		return "", false
	}
	title := getString(page["title"])
	if prefix != "" && !strings.HasPrefix(title, prefix) {
		return "", false
	}
	return title, true
}

// fetchOrphanInfoBatch fetches detailed page info for one batch of orphan
// titles. On API failure, returns minimal entries (title only) so the caller
// can still report the orphan list.
func (c *Client) fetchOrphanInfoBatch(ctx context.Context, batch []string) []OrphanedPage {
	out := make([]OrphanedPage, 0, len(batch))

	infoParams := url.Values{}
	infoParams.Set("action", "query")
	infoParams.Set("titles", strings.Join(batch, "|"))
	infoParams.Set("prop", "info")

	infoResp, err := c.apiRequest(ctx, infoParams)
	if err != nil {
		for _, title := range batch {
			out = append(out, OrphanedPage{Title: title})
		}
		return out
	}
	infoQuery, _ := infoResp["query"].(map[string]interface{})
	pages, _ := infoQuery["pages"].(map[string]interface{})
	for _, pageData := range pages {
		p, ok := pageData.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, OrphanedPage{
			Title:      getString(p["title"]),
			PageID:     getInt(p["pageid"]),
			Length:     getInt(p["length"]),
			LastEdited: getString(p["touched"]),
		})
	}
	return out
}

// fetchOrphanedPagesInfo fetches info for all titles in 50-at-a-time batches.
func (c *Client) fetchOrphanedPagesInfo(ctx context.Context, titles []string) []OrphanedPage {
	const batchSize = 50
	orphaned := make([]OrphanedPage, 0, len(titles))
	for i := 0; i < len(titles); i += batchSize {
		end := i + batchSize
		if end > len(titles) {
			end = len(titles)
		}
		orphaned = append(orphaned, c.fetchOrphanInfoBatch(ctx, titles[i:end])...)
	}
	return orphaned
}

func (c *Client) FindOrphanedPages(ctx context.Context, args FindOrphanedPagesArgs) (FindOrphanedPagesResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return FindOrphanedPagesResult{}, err
	}

	limit := normalizeLimit(args.Limit, 50, 200)
	results, err := c.queryLonelyPages(ctx, limit)
	if err != nil {
		return FindOrphanedPagesResult{}, err
	}

	var filteredTitles []string
	for _, r := range results {
		if title, ok := orphanedPageMatchesFilter(r, args.Namespace, args.Prefix); ok {
			filteredTitles = append(filteredTitles, title)
		}
	}

	orphaned := c.fetchOrphanedPagesInfo(ctx, filteredTitles)
	return FindOrphanedPagesResult{
		OrphanedPages: orphaned,
		TotalChecked:  len(results),
		OrphanedCount: len(orphaned),
	}, nil
}
