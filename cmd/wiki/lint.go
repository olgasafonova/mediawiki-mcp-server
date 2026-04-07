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

func runLint(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	category, _ := cmd.Flags().GetString("category")
	glossaryPage, _ := cmd.Flags().GetString("glossary-page")
	limit, _ := cmd.Flags().GetInt("limit")
	checksFlag, _ := cmd.Flags().GetString("check")

	if len(args) == 0 && category == "" {
		return fmt.Errorf("specify page titles as arguments or use --category")
	}

	checks := parseChecks(checksFlag)
	ctx := context.Background()

	var termResult *wiki.CheckTerminologyResult
	var linksResult *wiki.FindBrokenInternalLinksResult

	if checks["terminology"] {
		r, err := client.CheckTerminology(ctx, wiki.CheckTerminologyArgs{
			Pages:        args,
			Category:     category,
			GlossaryPage: glossaryPage,
			Limit:        limit,
		})
		if err != nil {
			return fmt.Errorf("terminology check failed: %w", err)
		}
		termResult = &r
	}

	if checks["links"] {
		r, err := client.FindBrokenInternalLinks(ctx, wiki.FindBrokenInternalLinksArgs{
			Pages:    args,
			Category: category,
			Limit:    limit,
		})
		if err != nil {
			return fmt.Errorf("broken links check failed: %w", err)
		}
		linksResult = &r
	}

	if isJSON(cmd) {
		combined := map[string]interface{}{}
		if termResult != nil {
			combined["terminology"] = termResult
		}
		if linksResult != nil {
			combined["broken_links"] = linksResult
		}
		return printJSON(combined)
	}

	// Human-readable output
	totalTermIssues := 0
	totalBrokenLinks := 0
	pagesChecked := 0

	if termResult != nil {
		totalTermIssues = termResult.IssuesFound
		pagesChecked = termResult.PagesChecked
		if termResult.IssuesFound > 0 {
			fmt.Printf("Terminology Issues (%d):\n", termResult.IssuesFound)
			for _, page := range termResult.Pages {
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
	}

	if linksResult != nil {
		totalBrokenLinks = linksResult.BrokenCount
		if linksResult.PagesChecked > pagesChecked {
			pagesChecked = linksResult.PagesChecked
		}
		if linksResult.BrokenCount > 0 {
			fmt.Printf("Broken Internal Links (%d):\n", linksResult.BrokenCount)
			for _, page := range linksResult.Pages {
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
	}

	// Summary line
	parts := []string{}
	if termResult != nil {
		parts = append(parts, fmt.Sprintf("%d terminology issue%s",
			totalTermIssues, plural(totalTermIssues)))
	}
	if linksResult != nil {
		parts = append(parts, fmt.Sprintf("%d broken link%s",
			totalBrokenLinks, plural(totalBrokenLinks)))
	}
	fmt.Printf("%s across %d page%s\n",
		strings.Join(parts, ", "), pagesChecked, plural(pagesChecked))

	if totalTermIssues > 0 || totalBrokenLinks > 0 {
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
