package wiki

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

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

	excludeCode := excludeCodeBlocks(args.ExcludeCodeBlocks)
	if err := c.checkPagesTerminology(ctx, pagesToCheck, glossary, excludeCode, &result); err != nil {
		return result, err
	}

	result.PagesChecked = len(result.Pages)
	return result, nil
}

// excludeCodeBlocks resolves the exclude-code-blocks flag, defaulting to true.
func excludeCodeBlocks(flag *bool) bool {
	if flag != nil {
		return *flag
	}
	return true
}

// checkPagesTerminology checks each page against the glossary, accumulating
// results. It aborts early on context cancellation.
func (c *Client) checkPagesTerminology(ctx context.Context, pages []string, glossary []GlossaryTerm, excludeCode bool, result *CheckTerminologyResult) error {
	for _, pageTitle := range pages {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		pageResult := c.checkPageTerminology(ctx, pageTitle, glossary, excludeCode)
		result.Pages = append(result.Pages, pageResult)
		result.IssuesFound += pageResult.IssueCount
	}
	return nil
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
		terms = append(terms, parseGlossaryTableRows(table[1])...)
	}
	return terms
}

// parseGlossaryTableRows parses the rows of a single glossary table body into
// glossary terms.
func parseGlossaryTableRows(tableBody string) []GlossaryTerm {
	var terms []GlossaryTerm
	for _, row := range strings.Split(tableBody, "|-") {
		row = strings.TrimSpace(row)
		if row == "" || strings.HasPrefix(row, "!") {
			continue
		}
		if term, ok := glossaryTermFromCells(parseTableRow(row)); ok {
			terms = append(terms, term)
		}
	}
	return terms
}

// parseTableRow extracts cells from a wiki table row
func parseTableRow(row string) []string {
	var cells []string
	for _, line := range strings.Split(row, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		cells = append(cells, parseRowLineCells(line)...)
	}
	return cells
}

// parseRowLineCells splits one table-row line into trimmed, non-empty cells.
func parseRowLineCells(line string) []string {
	var cells []string
	// Remove leading | if present, then split by || for multiple cells.
	for _, part := range strings.Split(strings.TrimPrefix(line, "|"), "||") {
		if cell := strings.TrimSpace(part); cell != "" {
			cells = append(cells, cell)
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
