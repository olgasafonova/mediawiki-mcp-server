package wiki

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

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
