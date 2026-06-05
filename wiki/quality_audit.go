package wiki

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

type healthCheckApply func(*WikiHealthAuditResult)

// healthCheckFunc runs one health check and returns either an apply function
// or an error. Apply functions are invoked under the dispatcher's lock so
// individual checks don't need their own synchronization.
type healthCheckFunc func(context.Context, WikiHealthAuditArgs, int) (healthCheckApply, error)

// runLinksCheck checks for broken internal links and updates the broken-link summary.
func (c *Client) runLinksCheck(ctx context.Context, args WikiHealthAuditArgs, limit int) (healthCheckApply, error) {
	r, err := c.FindBrokenInternalLinks(ctx, FindBrokenInternalLinksArgs{
		Pages:    args.Pages,
		Category: args.Category,
		Limit:    limit,
	})
	if err != nil {
		return nil, err
	}
	return func(out *WikiHealthAuditResult) {
		out.BrokenLinks = &r
		out.Summary.BrokenLinksCount = r.BrokenCount
		if r.PagesChecked > out.PagesAudited {
			out.PagesAudited = r.PagesChecked
		}
	}, nil
}

// runTerminologyCheck runs the terminology consistency check.
func (c *Client) runTerminologyCheck(ctx context.Context, args WikiHealthAuditArgs, limit int) (healthCheckApply, error) {
	r, err := c.CheckTerminology(ctx, CheckTerminologyArgs{
		Pages:    args.Pages,
		Category: args.Category,
		Limit:    limit,
	})
	if err != nil {
		return nil, err
	}
	return func(out *WikiHealthAuditResult) {
		out.Terminology = &r
		out.Summary.TerminologyIssues = r.IssuesFound
		if r.PagesChecked > out.PagesAudited {
			out.PagesAudited = r.PagesChecked
		}
	}, nil
}

// runOrphansCheck looks for pages with no incoming links in the main namespace.
func (c *Client) runOrphansCheck(ctx context.Context, _ WikiHealthAuditArgs, limit int) (healthCheckApply, error) {
	r, err := c.FindOrphanedPages(ctx, FindOrphanedPagesArgs{
		Namespace: 0,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	return func(out *WikiHealthAuditResult) {
		out.OrphanedPages = &r
		out.Summary.OrphanedPagesCount = r.OrphanedCount
	}, nil
}

// runActivityCheck summarizes recent changes by user.
func (c *Client) runActivityCheck(ctx context.Context, _ WikiHealthAuditArgs, limit int) (healthCheckApply, error) {
	r, err := c.GetRecentChanges(ctx, RecentChangesArgs{Limit: limit})
	if err != nil {
		return nil, err
	}
	aggregated := aggregateChanges(r.Changes, "user")
	return func(out *WikiHealthAuditResult) {
		out.RecentActivity = aggregated
	}, nil
}

// runExternalCheck samples external links from a sample page and tests reachability.
func (c *Client) runExternalCheck(ctx context.Context, args WikiHealthAuditArgs, _ int) (healthCheckApply, error) {
	pages := samplePagesForExternalCheck(ctx, c, args, 5)
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages or URLs found to check")
	}
	page, err := c.GetPage(ctx, GetPageArgs{Title: pages[0], Format: "wikitext"})
	if err != nil {
		return nil, fmt.Errorf("no pages or URLs found to check")
	}
	urls := extractExternalURLs(page.Content, 10)
	if len(urls) == 0 {
		return nil, fmt.Errorf("no pages or URLs found to check")
	}
	r, err := c.CheckLinks(ctx, CheckLinksArgs{URLs: urls})
	if err != nil {
		return nil, err
	}
	return func(out *WikiHealthAuditResult) {
		out.ExternalLinks = &r
		out.Summary.ExternalBrokenCount = r.BrokenCount
	}, nil
}

// samplePagesForExternalCheck returns up to maxPages titles from args.Pages or args.Category.
// External checks are slow, so the sample is intentionally small.
func samplePagesForExternalCheck(ctx context.Context, c *Client, args WikiHealthAuditArgs, maxPages int) []string {
	if len(args.Pages) > 0 {
		if len(args.Pages) > maxPages {
			return args.Pages[:maxPages]
		}
		return args.Pages
	}
	if args.Category == "" {
		return nil
	}
	catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
		Category: args.Category,
		Limit:    maxPages,
	})
	if err != nil {
		return nil
	}
	titles := make([]string, 0, len(catResult.Members))
	for _, p := range catResult.Members {
		titles = append(titles, p.Title)
	}
	return titles
}

// computeHealthScore turns the summary counts into a 0-100 score.
// Formula: 100 - (broken_links*5 + terminology*2 + orphans*1 + external*3).
func computeHealthScore(summary WikiHealthAuditSummary) int {
	score := 100 -
		summary.BrokenLinksCount*5 -
		summary.TerminologyIssues*2 -
		summary.OrphanedPagesCount*1 -
		summary.ExternalBrokenCount*3
	if score < 0 {
		return 0
	}
	return score
}

// healthAuditChecks returns the registry mapping check names to runners.
func (c *Client) healthAuditChecks() map[string]healthCheckFunc {
	return map[string]healthCheckFunc{
		"links":       c.runLinksCheck,
		"terminology": c.runTerminologyCheck,
		"orphans":     c.runOrphansCheck,
		"activity":    c.runActivityCheck,
		"external":    c.runExternalCheck,
	}
}

// HealthAudit runs a comprehensive wiki health audit, checking multiple quality metrics in parallel.
// It aggregates results from various checks and calculates an overall health score.
func (c *Client) HealthAudit(ctx context.Context, args WikiHealthAuditArgs) (WikiHealthAuditResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return WikiHealthAuditResult{}, err
	}

	result := WikiHealthAuditResult{
		WikiName:  c.config.BaseURL,
		AuditedAt: time.Now().UTC().Format(time.RFC3339),
		Errors:    make([]string, 0),
	}

	checksToRun := args.Checks
	if len(checksToRun) == 0 {
		checksToRun = []string{"links", "terminology", "orphans", "activity"}
	}
	limit := normalizeLimit(args.Limit, 20, 50)
	registry := c.healthAuditChecks()

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, name := range checksToRun {
		check, ok := registry[name]
		if !ok {
			continue
		}
		wg.Add(1)
		go func(name string, check healthCheckFunc) {
			defer wg.Done()
			apply, err := check(ctx, args, limit)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s check failed: %v", name, err))
				return
			}
			apply(&result)
		}(name, check)
	}
	wg.Wait()

	result.HealthScore = computeHealthScore(result.Summary)
	return result, nil
}

// extractExternalURLs extracts external URLs from wiki content
func extractExternalURLs(content string, limit int) []string {
	// Match URLs in external link syntax [http...] or bare URLs
	urlRegex := regexp.MustCompile(`https?://[^\s\]\[<>\"]+`)
	matches := urlRegex.FindAllString(content, -1)

	// Deduplicate and limit
	seen := make(map[string]bool)
	var urls []string
	for _, url := range matches {
		// Clean up URL (remove trailing punctuation)
		url = strings.TrimRight(url, ".,;:!?)")
		if !seen[url] && len(urls) < limit {
			seen[url] = true
			urls = append(urls, url)
		}
	}
	return urls
}

// listPagesForStaleCheck collects candidate page titles for the stale-page check
// from either a category or a namespace. Returns oversampled results because
