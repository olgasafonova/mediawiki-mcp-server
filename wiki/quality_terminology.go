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
	pagesToCheck, err := c.collectPagesFromArgs(ctx, pageSelection{Pages: args.Pages, Category: args.Category, Limit: limit, FieldName: "pages"})
	if err != nil {
		return CheckTerminologyResult{}, err
	}

	result := CheckTerminologyResult{
		GlossaryPage: glossaryPage,
		TermsLoaded:  len(glossary),
		Pages:        make([]PageTerminologyResult, 0, len(pagesToCheck)),
	}

	check := terminologyCheck{
		glossary:    glossary,
		excludeCode: excludeCodeBlocks(args.ExcludeCodeBlocks),
	}
	if err := c.checkPagesTerminology(ctx, pagesToCheck, check, &result); err != nil {
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

// terminologyCheck bundles the glossary and options applied to every page.
type terminologyCheck struct {
	glossary    []GlossaryTerm
	excludeCode bool
}

// checkPagesTerminology checks each page against the glossary, accumulating
// results. It aborts early on context cancellation.
func (c *Client) checkPagesTerminology(ctx context.Context, pages []string, check terminologyCheck, result *CheckTerminologyResult) error {
	for _, pageTitle := range pages {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		pageResult := c.checkPageTerminology(ctx, pageTitle, check)
		result.Pages = append(result.Pages, pageResult)
		result.IssuesFound += pageResult.IssueCount
	}
	return nil
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
func (c *Client) checkPageTerminology(ctx context.Context, title string, check terminologyCheck) PageTerminologyResult {
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
	if check.excludeCode {
		content = stripCodeBlocksForTerminology(content)
	}

	// Pre-compile term matchers once per page.
	matchers := make([]*regexp.Regexp, len(check.glossary))
	for i, term := range check.glossary {
		matchers[i] = compileTermMatcher(term)
	}

	for lineNum, line := range strings.Split(content, "\n") {
		for i, term := range check.glossary {
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
