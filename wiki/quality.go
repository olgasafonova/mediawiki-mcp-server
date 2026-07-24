package wiki

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// pageSelection describes which pages an operation should run on: either an
// explicit list or a category. FieldName is used in the "neither specified"
// error message so callers can match their own argument naming
// (e.g. "pages" vs "base_pages").
type pageSelection struct {
	Pages     []string
	Category  string
	Limit     int
	FieldName string
}

// collectPagesFromArgs resolves the set of pages to operate on from a
// pageSelection. Returns an error if neither pages nor category is specified.
func (c *Client) collectPagesFromArgs(ctx context.Context, sel pageSelection) ([]string, error) {
	if len(sel.Pages) > 0 {
		return capPageList(sel.Pages, sel.Limit), nil
	}
	if sel.Category != "" {
		return c.categoryPageTitles(ctx, sel.Category, sel.Limit)
	}
	return nil, fmt.Errorf("either '%s' or 'category' must be specified", sel.FieldName)
}

// capPageList truncates the page list to at most limit entries.
func capPageList(pages []string, limit int) []string {
	if len(pages) > limit {
		return pages[:limit]
	}
	return pages
}

// categoryPageTitles returns the titles of up to limit members of a category.
func (c *Client) categoryPageTitles(ctx context.Context, category string, limit int) ([]string, error) {
	catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
		Category: category,
		Limit:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get category members: %w", err)
	}
	titles := make([]string, 0, len(catResult.Members))
	for _, p := range catResult.Members {
		titles = append(titles, p.Title)
	}
	return titles, nil
}

// listPagesForStaleCheck gathers candidate page titles for a staleness audit;
// the caller filters them by last-edited timestamp.
func (c *Client) listPagesForStaleCheck(ctx context.Context, args GetStalePagesArgs, limit int) ([]string, error) {
	if args.Category != "" {
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
			Category: args.Category,
			Limit:    limit * 2,
			Type:     "page",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get category members: %w", err)
		}
		titles := make([]string, 0, len(catResult.Members))
		for _, m := range catResult.Members {
			titles = append(titles, m.Title)
		}
		return titles, nil
	}

	listResult, err := c.ListPages(ctx, ListPagesArgs{
		Namespace: args.Namespace,
		Limit:     limit * 3,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pages: %w", err)
	}
	titles := make([]string, 0, len(listResult.Pages))
	for _, p := range listResult.Pages {
		titles = append(titles, p.Title)
	}
	return titles, nil
}

// findStaleInBatches fetches page info in batches and returns pages last
// touched before cutoff. Errors on individual batches are skipped silently
// so a single API hiccup doesn't sink the whole audit.
func (c *Client) findStaleInBatches(ctx context.Context, titles []string, cutoff time.Time) []StalePage {
	stale := make([]StalePage, 0)
	for i := 0; i < len(titles); i += MaxBatchSize {
		if ctx.Err() != nil {
			break
		}
		end := min(i+MaxBatchSize, len(titles))
		stale = append(stale, c.staleInBatch(ctx, titles[i:end], cutoff)...)
	}
	return stale
}

// staleInBatch fetches info for one batch of titles and returns the entries
// last touched before cutoff. A failed batch yields no entries.
func (c *Client) staleInBatch(ctx context.Context, batch []string, cutoff time.Time) []StalePage {
	batchInfo, err := c.GetPagesInfoBatch(ctx, GetPagesInfoBatchArgs{Titles: batch})
	if err != nil {
		return nil
	}
	stale := make([]StalePage, 0)
	for _, info := range batchInfo.Pages {
		if page, ok := staleEntryFromInfo(info, cutoff); ok {
			stale = append(stale, page)
		}
	}
	return stale
}

// staleEntryFromInfo returns a StalePage if the page exists and was touched
// before cutoff. The bool reports whether the page qualified.
func staleEntryFromInfo(info PageInfo, cutoff time.Time) (StalePage, bool) {
	if !info.Exists || info.Touched == "" {
		return StalePage{}, false
	}
	touched, err := time.Parse("2006-01-02T15:04:05Z", info.Touched)
	if err != nil || !touched.Before(cutoff) {
		return StalePage{}, false
	}
	return StalePage{
		Title:      info.Title,
		PageID:     info.PageID,
		LastEdited: info.Touched,
		DaysStale:  int(time.Since(touched).Hours() / 24),
		Length:     info.Length,
	}, true
}

// GetStalePages finds pages that haven't been edited in a specified number of days.
func (c *Client) GetStalePages(ctx context.Context, args GetStalePagesArgs) (GetStalePagesResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetStalePagesResult{}, err
	}

	days := args.Days
	if days <= 0 {
		days = 90
	}

	limit := normalizeLimit(args.Limit, 50, 200)
	cutoff := time.Now().AddDate(0, 0, -days)

	pageTitles, err := c.listPagesForStaleCheck(ctx, args, limit)
	if err != nil {
		return GetStalePagesResult{}, err
	}

	if len(pageTitles) == 0 {
		return GetStalePagesResult{
			Days:       days,
			StalePages: []StalePage{},
			Message:    "No pages found to check",
		}, nil
	}

	result := GetStalePagesResult{
		Days:         days,
		StalePages:   c.findStaleInBatches(ctx, pageTitles, cutoff),
		TotalScanned: len(pageTitles),
	}

	sort.Slice(result.StalePages, func(i, j int) bool {
		return result.StalePages[i].LastEdited < result.StalePages[j].LastEdited
	})

	if len(result.StalePages) > limit {
		result.StalePages = result.StalePages[:limit]
	}

	result.StaleCount = len(result.StalePages)
	result.Message = fmt.Sprintf("Found %d pages not edited in the last %d days (scanned %d pages)",
		result.StaleCount, days, result.TotalScanned)

	return result, nil
}
