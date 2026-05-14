package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newLintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lint [page1] [page2...]",
		Short: "Check wiki pages for terminology and link issues",
		Long: `Run quality checks on wiki pages. Checks terminology consistency
and broken internal links. Exit code 4 when violations are found (for CI).

Specify pages as arguments or use --category to check all pages in a category.`,
		RunE: runLint,
	}

	cmd.Flags().String("category", "", "Category to check (alternative to page arguments)")
	cmd.Flags().String("glossary-page", "", "Wiki page containing the glossary table")
	cmd.Flags().Int("limit", 20, "Maximum pages to check")
	cmd.Flags().String("check", "terminology,links", "Comma-separated checks to run: terminology, links")

	return cmd
}

type lintOpts struct {
	pages        []string
	category     string
	glossaryPage string
	limit        int
	checks       map[string]bool
}

func parseLintOpts(cmd *cobra.Command, args []string) (lintOpts, error) {
	category, _ := cmd.Flags().GetString("category")
	if len(args) == 0 && category == "" {
		return lintOpts{}, fmt.Errorf("specify page titles as arguments or use --category")
	}
	glossaryPage, _ := cmd.Flags().GetString("glossary-page")
	limit, _ := cmd.Flags().GetInt("limit")
	checksFlag, _ := cmd.Flags().GetString("check")
	return lintOpts{
		pages:        args,
		category:     category,
		glossaryPage: glossaryPage,
		limit:        limit,
		checks:       parseChecks(checksFlag),
	}, nil
}

type lintResults struct {
	term  *wiki.CheckTerminologyResult
	links *wiki.FindBrokenInternalLinksResult
}

func runLintChecks(ctx context.Context, client *wiki.Client, opts lintOpts) (lintResults, error) {
	var out lintResults
	if opts.checks["terminology"] {
		r, err := client.CheckTerminology(ctx, wiki.CheckTerminologyArgs{
			Pages:        opts.pages,
			Category:     opts.category,
			GlossaryPage: opts.glossaryPage,
			Limit:        opts.limit,
		})
		if err != nil {
			return out, fmt.Errorf("terminology check failed: %w", err)
		}
		out.term = &r
	}
	if opts.checks["links"] {
		r, err := client.FindBrokenInternalLinks(ctx, wiki.FindBrokenInternalLinksArgs{
			Pages:    opts.pages,
			Category: opts.category,
			Limit:    opts.limit,
		})
		if err != nil {
			return out, fmt.Errorf("broken links check failed: %w", err)
		}
		out.links = &r
	}
	return out, nil
}

func printLintJSON(r lintResults) error {
	combined := map[string]interface{}{}
	if r.term != nil {
		combined["terminology"] = r.term
	}
	if r.links != nil {
		combined["broken_links"] = r.links
	}
	return printJSON(combined)
}

func printTerminologyIssues(r *wiki.CheckTerminologyResult) {
	if r == nil || r.IssuesFound == 0 {
		return
	}
	fmt.Printf("Terminology Issues (%d):\n", r.IssuesFound)
	for _, page := range r.Pages {
		if page.IssueCount == 0 {
			continue
		}
		for _, issue := range page.Issues {
			fmt.Printf("  %s: %q should be %q (line %d)\n",
				page.Title, issue.Incorrect, issue.Correct, issue.Line)
		}
	}
	fmt.Println()
}

func printBrokenLinks(r *wiki.FindBrokenInternalLinksResult) {
	if r == nil || r.BrokenCount == 0 {
		return
	}
	fmt.Printf("Broken Internal Links (%d):\n", r.BrokenCount)
	for _, page := range r.Pages {
		if page.BrokenCount == 0 {
			continue
		}
		for _, link := range page.BrokenLinks {
			if link.Line > 0 {
				fmt.Printf("  %s -> %s (line %d)\n", page.Title, link.Target, link.Line)
			} else {
				fmt.Printf("  %s -> %s\n", page.Title, link.Target)
			}
		}
	}
	fmt.Println()
}

func printLintSummary(r lintResults) (termIssues, brokenLinks int) {
	pagesChecked := 0
	parts := []string{}
	if r.term != nil {
		termIssues = r.term.IssuesFound
		pagesChecked = r.term.PagesChecked
		parts = append(parts, fmt.Sprintf("%d terminology issue%s",
			termIssues, plural(termIssues)))
	}
	if r.links != nil {
		brokenLinks = r.links.BrokenCount
		if r.links.PagesChecked > pagesChecked {
			pagesChecked = r.links.PagesChecked
		}
		parts = append(parts, fmt.Sprintf("%d broken link%s",
			brokenLinks, plural(brokenLinks)))
	}
	fmt.Printf("%s across %d page%s\n",
		strings.Join(parts, ", "), pagesChecked, plural(pagesChecked))
	return termIssues, brokenLinks
}

func runLint(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	opts, err := parseLintOpts(cmd, args)
	if err != nil {
		return err
	}

	results, err := runLintChecks(context.Background(), client, opts)
	if err != nil {
		return err
	}

	if isJSON(cmd) {
		return printLintJSON(results)
	}

	printTerminologyIssues(results.term)
	printBrokenLinks(results.links)
	termIssues, brokenLinks := printLintSummary(results)

	if termIssues > 0 || brokenLinks > 0 {
		os.Exit(4)
	}
	return nil
}

func parseChecks(s string) map[string]bool {
	result := map[string]bool{}
	for _, c := range strings.Split(s, ",") {
		result[strings.TrimSpace(c)] = true
	}
	return result
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
