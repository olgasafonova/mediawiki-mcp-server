package wiki

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// collectPagesFromArgs resolves the set of pages to operate on from either
// an explicit list or a category. Returns an error if neither is specified.
// pagesFieldName is used in the "neither specified" error message so callers
// can match their own argument naming (e.g. "pages" vs "base_pages").
func (c *Client) collectPagesFromArgs(ctx context.Context, pages []string, category string, limit int, pagesFieldName string) ([]string, error) {
	if len(pages) > 0 {
		if len(pages) > limit {
			pages = pages[:limit]
		}
		return pages, nil
	}
	if category != "" {
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
	return nil, fmt.Errorf("either '%s' or 'category' must be specified", pagesFieldName)
}

// CheckTerminology checks pages for terminology inconsistencies based on a wiki glossary
func (c *Client) CheckTerminology(ctx context.Context, args CheckTerminologyArgs) (CheckTerminologyResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CheckTerminologyResult{}, err
	}

	glossaryPage := args.GlossaryPage
	if glossaryPage == "" {
		glossaryPage = "Brand Terminology Glossary"
	}

	glossary, err := c.loadGlossary(ctx, glossaryPage)
	if err != nil {
		return CheckTerminologyResult{}, fmt.Errorf("failed to load glossary from '%s': %w", glossaryPage, err)
	}
	if len(glossary) == 0 {
		return CheckTerminologyResult{}, fmt.Errorf("no terms found in glossary page '%s'", glossaryPage)
	}

	limit := normalizeLimit(args.Limit, 10, 50)
	pagesToCheck, err := c.collectPagesFromArgs(ctx, args.Pages, args.Category, limit, "pages")
	if err != nil {
		return CheckTerminologyResult{}, err
	}

	result := CheckTerminologyResult{
		GlossaryPage: glossaryPage,
		TermsLoaded:  len(glossary),
		Pages:        make([]PageTerminologyResult, 0, len(pagesToCheck)),
	}

	// Determine if code blocks should be excluded (default: true)
	excludeCode := true
	if args.ExcludeCodeBlocks != nil {
		excludeCode = *args.ExcludeCodeBlocks
	}

	// Check each page
	for _, pageTitle := range pagesToCheck {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		pageResult := c.checkPageTerminology(ctx, pageTitle, glossary, excludeCode)
		result.Pages = append(result.Pages, pageResult)
		result.IssuesFound += pageResult.IssueCount
	}

	result.PagesChecked = len(result.Pages)
	return result, nil
}

// loadGlossary parses a wiki table to extract glossary terms
func (c *Client) loadGlossary(ctx context.Context, glossaryPage string) ([]GlossaryTerm, error) {
	page, err := c.GetPage(ctx, GetPageArgs{Title: glossaryPage, Format: "wikitext"})
	if err != nil {
		return nil, err
	}

	return parseWikiTableGlossary(page.Content), nil
}

// glossaryTableRegex matches wikitable blocks tagged with mcp-glossary or wikitable.
var glossaryTableRegex = regexp.MustCompile(`(?s)\{\|[^\n]*class="[^"]*(?:mcp-glossary|wikitable)[^"]*"[^\n]*\n(.*?)\|\}`)

// glossaryTermFromCells converts parsed table cells into a GlossaryTerm.
// Returns ok=false for rows that should be skipped (too few cells, empty, or
// where the "incorrect" form already matches the "correct" form).
func glossaryTermFromCells(cells []string) (GlossaryTerm, bool) {
	if len(cells) < 2 {
		return GlossaryTerm{}, false
	}
	term := GlossaryTerm{
		Incorrect: strings.TrimSpace(cells[0]),
		Correct:   strings.TrimSpace(cells[1]),
	}
	if term.Incorrect == "" || term.Incorrect == term.Correct {
		return GlossaryTerm{}, false
	}
	if len(cells) >= 3 {
		term.Pattern = strings.TrimSpace(cells[2])
	}
	if len(cells) >= 4 {
		term.Notes = strings.TrimSpace(cells[3])
	}
	return term, true
}

// parseWikiTableGlossary extracts terms from wikitable format
func parseWikiTableGlossary(content string) []GlossaryTerm {
	var terms []GlossaryTerm
	for _, table := range glossaryTableRegex.FindAllStringSubmatch(content, -1) {
		if len(table) < 2 {
			continue
		}
		for _, row := range strings.Split(table[1], "|-") {
			row = strings.TrimSpace(row)
			if row == "" || strings.HasPrefix(row, "!") {
				continue
			}
			if term, ok := glossaryTermFromCells(parseTableRow(row)); ok {
				terms = append(terms, term)
			}
		}
	}
	return terms
}

// parseTableRow extracts cells from a wiki table row
func parseTableRow(row string) []string {
	var cells []string
	lines := strings.Split(row, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}

		// Remove leading | if present
		line = strings.TrimPrefix(line, "|")

		// Split by || for multiple cells on one line
		parts := strings.Split(line, "||")
		for _, part := range parts {
			cell := strings.TrimSpace(part)
			if cell != "" {
				cells = append(cells, cell)
			}
		}
	}

	return cells
}

// compileTermMatcher returns a case-insensitive regex for a glossary term.
// Returns nil if the regex fails to compile (caller should skip the term).
func compileTermMatcher(term GlossaryTerm) *regexp.Regexp {
	expr := term.Pattern
	if expr == "" {
		expr = regexp.QuoteMeta(term.Incorrect)
	}
	re, err := regexp.Compile("(?i)" + expr)
	if err != nil {
		return nil
	}
	return re
}

// findTermIssuesInLine returns terminology issues for a single (line, term) pair.
// Skips matches whose text already equals the correct form.
func findTermIssuesInLine(line string, lineNum int, term GlossaryTerm, re *regexp.Regexp) []TerminologyIssue {
	var issues []TerminologyIssue
	for _, match := range re.FindAllStringIndex(line, -1) {
		matchedText := line[match[0]:match[1]]
		if strings.EqualFold(matchedText, term.Correct) {
			continue
		}
		issues = append(issues, TerminologyIssue{
			Incorrect: matchedText,
			Correct:   term.Correct,
			Line:      lineNum + 1,
			Context:   extractContext(line, match[0], match[1], 40),
			Notes:     term.Notes,
		})
	}
	return issues
}

// checkPageTerminology checks a single page against the glossary
func (c *Client) checkPageTerminology(ctx context.Context, title string, glossary []GlossaryTerm, excludeCode bool) PageTerminologyResult {
	result := PageTerminologyResult{
		Title:  title,
		Issues: make([]TerminologyIssue, 0),
	}

	page, err := c.GetPage(ctx, GetPageArgs{Title: title, Format: "wikitext"})
	if err != nil {
		result.Error = err.Error()
		return result
	}

	content := page.Content
	if excludeCode {
		content = stripCodeBlocksForTerminology(content)
	}

	// Pre-compile term matchers once per page.
	matchers := make([]*regexp.Regexp, len(glossary))
	for i, term := range glossary {
		matchers[i] = compileTermMatcher(term)
	}

	for lineNum, line := range strings.Split(content, "\n") {
		for i, term := range glossary {
			if matchers[i] == nil {
				continue
			}
			result.Issues = append(result.Issues, findTermIssuesInLine(line, lineNum, term, matchers[i])...)
		}
	}

	result.IssueCount = len(result.Issues)
	return result
}

// extractContext extracts surrounding text for context
func extractContext(line string, start, end, contextLen int) string {
	// Calculate bounds
	ctxStart := start - contextLen
	if ctxStart < 0 {
		ctxStart = 0
	}
	ctxEnd := end + contextLen
	if ctxEnd > len(line) {
		ctxEnd = len(line)
	}

	context := line[ctxStart:ctxEnd]

	// Add ellipsis if truncated
	if ctxStart > 0 {
		context = "..." + context
	}
	if ctxEnd < len(line) {
		context = context + "..."
	}

	return context
}

// stripCodeBlocksForTerminology removes code block content while preserving line structure
// This prevents false positives on code paths like SI.Data, namespace.Class, etc.
func stripCodeBlocksForTerminology(content string) string {
	// Replace content inside code tags with spaces to preserve line numbers
	// Handles: <syntaxhighlight>, <source>, <pre>, <code>
	codeTagPatterns := []string{
		`(?is)<syntaxhighlight[^>]*>(.*?)</syntaxhighlight>`,
		`(?is)<source[^>]*>(.*?)</source>`,
		`(?is)<pre[^>]*>(.*?)</pre>`,
		`(?is)<code[^>]*>(.*?)</code>`,
	}

	for _, pattern := range codeTagPatterns {
		re := regexp.MustCompile(pattern)
		content = re.ReplaceAllStringFunc(content, func(match string) string {
			// Replace the entire match with spaces, preserving newlines
			var result strings.Builder
			for _, ch := range match {
				if ch == '\n' {
					result.WriteRune('\n')
				} else {
					result.WriteRune(' ')
				}
			}
			return result.String()
		})
	}

	return content
}

// validTranslationPatterns is the set of accepted translation pattern names.
var validTranslationPatterns = map[string]struct{}{
	"subpage": {},
	"suffix":  {},
	"prefix":  {},
}

// validateTranslationPattern resolves the default and validates the pattern name.
func validateTranslationPattern(pattern string) (string, error) {
	if pattern == "" {
		pattern = "subpage"
	}
	if _, ok := validTranslationPatterns[pattern]; !ok {
		return "", fmt.Errorf("invalid pattern: %s (use 'subpage', 'suffix', or 'prefix')", pattern)
	}
	return pattern, nil
}

// buildTranslationTitle composes the localized page title for a base page,
// language, and naming pattern.
func buildTranslationTitle(basePage, lang, pattern string) string {
	switch pattern {
	case "suffix":
		return fmt.Sprintf("%s (%s)", basePage, lang)
	case "prefix":
		return fmt.Sprintf("%s:%s", lang, basePage)
	default: // "subpage"
		return fmt.Sprintf("%s/%s", basePage, lang)
	}
}

// checkBasePageTranslations checks one base page across all requested languages
// and returns the per-page result plus the count of missing translations.
func (c *Client) checkBasePageTranslations(ctx context.Context, basePage string, languages []string, pattern string) (PageTranslationResult, int) {
	pageResult := PageTranslationResult{
		BasePage:     basePage,
		Translations: make(map[string]TranslationStatus),
		Complete:     true,
	}
	missing := 0

	for _, lang := range languages {
		langPage := buildTranslationTitle(basePage, lang, pattern)
		status := TranslationStatus{PageTitle: langPage}

		info, err := c.GetPageInfo(ctx, PageInfoArgs{Title: langPage})
		if err == nil && info.Exists {
			status.Exists = true
			status.PageID = info.PageID
			status.Length = info.Length
		} else {
			pageResult.MissingLangs = append(pageResult.MissingLangs, lang)
			pageResult.Complete = false
			missing++
		}

		pageResult.Translations[lang] = status
	}

	return pageResult, missing
}

// CheckTranslations checks if pages exist in all specified languages
func (c *Client) CheckTranslations(ctx context.Context, args CheckTranslationsArgs) (CheckTranslationsResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CheckTranslationsResult{}, err
	}

	if len(args.Languages) == 0 {
		return CheckTranslationsResult{}, fmt.Errorf("at least one language is required")
	}

	pattern, err := validateTranslationPattern(args.Pattern)
	if err != nil {
		return CheckTranslationsResult{}, err
	}

	limit := normalizeLimit(args.Limit, 20, 100)
	basePages, err := c.collectPagesFromArgs(ctx, args.BasePages, args.Category, limit, "base_pages")
	if err != nil {
		return CheckTranslationsResult{}, err
	}

	result := CheckTranslationsResult{
		LanguagesChecked: args.Languages,
		Pattern:          pattern,
		Pages:            make([]PageTranslationResult, 0, len(basePages)),
	}

	for _, basePage := range basePages {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		pageResult, missing := c.checkBasePageTranslations(ctx, basePage, args.Languages, pattern)
		result.MissingCount += missing
		result.Pages = append(result.Pages, pageResult)
	}

	result.PagesChecked = len(result.Pages)
	return result, nil
}

// healthCheckApply mutates the audit result with one check's outcome.
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
		end := i + MaxBatchSize
		if end > len(titles) {
			end = len(titles)
		}
		batchInfo, err := c.GetPagesInfoBatch(ctx, GetPagesInfoBatchArgs{Titles: titles[i:end]})
		if err != nil {
			continue
		}
		for _, info := range batchInfo.Pages {
			if page, ok := staleEntryFromInfo(info, cutoff); ok {
				stale = append(stale, page)
			}
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
