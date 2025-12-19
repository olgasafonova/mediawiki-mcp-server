package wiki

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// CheckTerminology checks pages for terminology inconsistencies based on a wiki glossary
func (c *Client) CheckTerminology(ctx context.Context, args CheckTerminologyArgs) (CheckTerminologyResult, error) {
	// Ensure logged in for wikis requiring auth for read
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CheckTerminologyResult{}, err
	}

	// Default glossary page
	glossaryPage := args.GlossaryPage
	if glossaryPage == "" {
		glossaryPage = "Brand Terminology Glossary"
	}

	// Load glossary from wiki
	glossary, err := c.loadGlossary(ctx, glossaryPage)
	if err != nil {
		return CheckTerminologyResult{}, fmt.Errorf("failed to load glossary from '%s': %w", glossaryPage, err)
	}

	if len(glossary) == 0 {
		return CheckTerminologyResult{}, fmt.Errorf("no terms found in glossary page '%s'", glossaryPage)
	}

	// Get pages to check
	var pagesToCheck []string
	limit := normalizeLimit(args.Limit, 10, 50)

	if len(args.Pages) > 0 {
		pagesToCheck = args.Pages
		if len(pagesToCheck) > limit {
			pagesToCheck = pagesToCheck[:limit]
		}
	} else if args.Category != "" {
		// Get pages from category
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
			Category: args.Category,
			Limit:    limit,
		})
		if err != nil {
			return CheckTerminologyResult{}, fmt.Errorf("failed to get category members: %w", err)
		}
		for _, p := range catResult.Members {
			pagesToCheck = append(pagesToCheck, p.Title)
		}
	} else {
		return CheckTerminologyResult{}, fmt.Errorf("either 'pages' or 'category' must be specified")
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

// parseWikiTableGlossary extracts terms from wikitable format
func parseWikiTableGlossary(content string) []GlossaryTerm {
	var terms []GlossaryTerm

	// Match wiki tables with class containing "mcp-glossary" or any wikitable
	// Format: {| class="wikitable..." ... |}
	tableRegex := regexp.MustCompile(`(?s)\{\|[^\n]*class="[^"]*(?:mcp-glossary|wikitable)[^"]*"[^\n]*\n(.*?)\|\}`)
	tables := tableRegex.FindAllStringSubmatch(content, -1)

	for _, table := range tables {
		if len(table) < 2 {
			continue
		}

		tableContent := table[1]

		// Split into rows (|-) and process each
		rows := strings.Split(tableContent, "|-")
		for _, row := range rows {
			row = strings.TrimSpace(row)
			if row == "" || strings.HasPrefix(row, "!") {
				// Skip empty rows and header rows
				continue
			}

			// Parse cells (|| separator or | at line start)
			cells := parseTableRow(row)
			if len(cells) >= 2 {
				term := GlossaryTerm{
					Incorrect: strings.TrimSpace(cells[0]),
					Correct:   strings.TrimSpace(cells[1]),
				}

				// Skip if incorrect is empty or equals correct
				if term.Incorrect == "" || term.Incorrect == term.Correct {
					continue
				}

				if len(cells) >= 3 {
					term.Pattern = strings.TrimSpace(cells[2])
				}
				if len(cells) >= 4 {
					term.Notes = strings.TrimSpace(cells[3])
				}
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

	// Strip code blocks to avoid false positives on code paths like SI.Data
	if excludeCode {
		content = stripCodeBlocksForTerminology(content)
	}

	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		for _, term := range glossary {
			// Use regex pattern if specified, otherwise literal match
			var re *regexp.Regexp
			var err error

			if term.Pattern != "" {
				re, err = regexp.Compile("(?i)" + term.Pattern)
			} else {
				// Escape special regex characters and do case-insensitive match
				escaped := regexp.QuoteMeta(term.Incorrect)
				re, err = regexp.Compile("(?i)" + escaped)
			}

			if err != nil {
				continue
			}

			matches := re.FindAllStringIndex(line, -1)
			for _, match := range matches {
				// Extract the actual matched text
				matchedText := line[match[0]:match[1]]

				// Skip if the matched text is actually the correct form
				if strings.EqualFold(matchedText, term.Correct) {
					continue
				}

				// Get context (surrounding text)
				context := extractContext(line, match[0], match[1], 40)

				result.Issues = append(result.Issues, TerminologyIssue{
					Incorrect: matchedText,
					Correct:   term.Correct,
					Line:      lineNum + 1,
					Context:   context,
					Notes:     term.Notes,
				})
			}
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

// CheckTranslations checks if pages exist in all specified languages
func (c *Client) CheckTranslations(ctx context.Context, args CheckTranslationsArgs) (CheckTranslationsResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return CheckTranslationsResult{}, err
	}

	if len(args.Languages) == 0 {
		return CheckTranslationsResult{}, fmt.Errorf("at least one language is required")
	}

	// Default pattern
	pattern := args.Pattern
	if pattern == "" {
		pattern = "subpage"
	}
	if pattern != "subpage" && pattern != "suffix" && pattern != "prefix" {
		return CheckTranslationsResult{}, fmt.Errorf("invalid pattern: %s (use 'subpage', 'suffix', or 'prefix')", pattern)
	}

	// Get base pages to check
	var basePages []string
	limit := normalizeLimit(args.Limit, 20, 100)

	if len(args.BasePages) > 0 {
		basePages = args.BasePages
		if len(basePages) > limit {
			basePages = basePages[:limit]
		}
	} else if args.Category != "" {
		catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
			Category: args.Category,
			Limit:    limit,
		})
		if err != nil {
			return CheckTranslationsResult{}, fmt.Errorf("failed to get category members: %w", err)
		}
		for _, p := range catResult.Members {
			basePages = append(basePages, p.Title)
		}
	} else {
		return CheckTranslationsResult{}, fmt.Errorf("either 'base_pages' or 'category' must be specified")
	}

	result := CheckTranslationsResult{
		LanguagesChecked: args.Languages,
		Pattern:          pattern,
		Pages:            make([]PageTranslationResult, 0, len(basePages)),
	}

	// Check each base page for all languages
	for _, basePage := range basePages {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		pageResult := PageTranslationResult{
			BasePage:     basePage,
			Translations: make(map[string]TranslationStatus),
			Complete:     true,
		}

		for _, lang := range args.Languages {
			// Build page title based on pattern
			var langPage string
			switch pattern {
			case "subpage":
				langPage = fmt.Sprintf("%s/%s", basePage, lang)
			case "suffix":
				langPage = fmt.Sprintf("%s (%s)", basePage, lang)
			case "prefix":
				langPage = fmt.Sprintf("%s:%s", lang, basePage)
			}

			// Check if page exists
			info, err := c.GetPageInfo(ctx, PageInfoArgs{Title: langPage})
			status := TranslationStatus{
				PageTitle: langPage,
			}

			if err == nil && info.Exists {
				status.Exists = true
				status.PageID = info.PageID
				status.Length = info.Length
			} else {
				status.Exists = false
				pageResult.MissingLangs = append(pageResult.MissingLangs, lang)
				pageResult.Complete = false
				result.MissingCount++
			}

			pageResult.Translations[lang] = status
		}

		result.Pages = append(result.Pages, pageResult)
	}

	result.PagesChecked = len(result.Pages)
	return result, nil
}

// HealthAudit runs a comprehensive wiki health audit, checking multiple quality metrics in parallel.
// It aggregates results from various checks and calculates an overall health score.
func (c *Client) HealthAudit(ctx context.Context, args WikiHealthAuditArgs) (WikiHealthAuditResult, error) {
	if err := c.EnsureLoggedIn(ctx); err != nil {
		return WikiHealthAuditResult{}, err
	}

	// Initialize result
	result := WikiHealthAuditResult{
		WikiName:  c.config.BaseURL,
		AuditedAt: time.Now().UTC().Format(time.RFC3339),
		Errors:    make([]string, 0),
	}

	// Determine which checks to run
	checksToRun := args.Checks
	if len(checksToRun) == 0 {
		// Default: all except external (which can be slow)
		checksToRun = []string{"links", "terminology", "orphans", "activity"}
	}

	// Normalize limit
	limit := normalizeLimit(args.Limit, 20, 50)

	// Build check set for quick lookup
	checkSet := make(map[string]bool)
	for _, check := range checksToRun {
		checkSet[check] = true
	}

	// Mutex for thread-safe result updates
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Run checks in parallel
	if checkSet["links"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			linksResult, err := c.FindBrokenInternalLinks(ctx, FindBrokenInternalLinksArgs{
				Pages:    args.Pages,
				Category: args.Category,
				Limit:    limit,
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("links check failed: %v", err))
			} else {
				result.BrokenLinks = &linksResult
				result.Summary.BrokenLinksCount = linksResult.BrokenCount
				if linksResult.PagesChecked > result.PagesAudited {
					result.PagesAudited = linksResult.PagesChecked
				}
			}
		}()
	}

	if checkSet["terminology"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			termResult, err := c.CheckTerminology(ctx, CheckTerminologyArgs{
				Pages:    args.Pages,
				Category: args.Category,
				Limit:    limit,
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("terminology check failed: %v", err))
			} else {
				result.Terminology = &termResult
				result.Summary.TerminologyIssues = termResult.IssuesFound
				if termResult.PagesChecked > result.PagesAudited {
					result.PagesAudited = termResult.PagesChecked
				}
			}
		}()
	}

	if checkSet["orphans"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			orphanResult, err := c.FindOrphanedPages(ctx, FindOrphanedPagesArgs{
				Namespace: 0,
				Limit:     limit,
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("orphans check failed: %v", err))
			} else {
				result.OrphanedPages = &orphanResult
				result.Summary.OrphanedPagesCount = orphanResult.OrphanedCount
			}
		}()
	}

	if checkSet["activity"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			activityResult, err := c.GetRecentChanges(ctx, RecentChangesArgs{
				Limit: limit,
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("activity check failed: %v", err))
			} else {
				// Aggregate by user
				aggregated := aggregateChanges(activityResult.Changes, "user")
				result.RecentActivity = aggregated
			}
		}()
	}

	if checkSet["external"] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Get a sample of pages to check external links
			var pagesToCheck []string
			if len(args.Pages) > 0 {
				pagesToCheck = args.Pages
				if len(pagesToCheck) > 5 {
					pagesToCheck = pagesToCheck[:5] // Limit external checks
				}
			} else if args.Category != "" {
				catResult, err := c.GetCategoryMembers(ctx, CategoryMembersArgs{
					Category: args.Category,
					Limit:    5,
				})
				if err == nil {
					for _, p := range catResult.Members {
						pagesToCheck = append(pagesToCheck, p.Title)
					}
				}
			}

			if len(pagesToCheck) > 0 {
				// Get external links from first page
				page, err := c.GetPage(ctx, GetPageArgs{Title: pagesToCheck[0], Format: "wikitext"})
				if err == nil {
					// Extract URLs from content (simple regex for external links)
					urls := extractExternalURLs(page.Content, 10)
					if len(urls) > 0 {
						extResult, err := c.CheckLinks(ctx, CheckLinksArgs{URLs: urls})
						mu.Lock()
						defer mu.Unlock()
						if err != nil {
							result.Errors = append(result.Errors, fmt.Sprintf("external links check failed: %v", err))
						} else {
							result.ExternalLinks = &extResult
							result.Summary.ExternalBrokenCount = extResult.BrokenCount
						}
						return
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			result.Errors = append(result.Errors, "external links check: no pages or URLs found to check")
		}()
	}

	// Wait for all checks to complete
	wg.Wait()

	// Calculate health score
	// Formula: 100 - (broken_links*5 + terminology*2 + orphans*1 + external*3)
	score := 100
	score -= result.Summary.BrokenLinksCount * 5
	score -= result.Summary.TerminologyIssues * 2
	score -= result.Summary.OrphanedPagesCount * 1
	score -= result.Summary.ExternalBrokenCount * 3
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

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
