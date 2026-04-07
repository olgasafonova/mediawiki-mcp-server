package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Run a comprehensive wiki health audit",
		Long: `Audit wiki health by checking broken links, terminology consistency,
orphaned pages, external links, and recent activity. Returns a health score
out of 100 with detailed findings.`,
		RunE: runAudit,
	}

	cmd.Flags().StringSlice("pages", nil, "Specific pages to audit (comma-separated)")
	cmd.Flags().String("category", "", "Category to audit")
	cmd.Flags().Int("limit", 20, "Maximum pages to audit")
	cmd.Flags().String("checks", "", "Comma-separated checks: links, terminology, orphans, external, activity (default: all except external)")

	return cmd
}

func runAudit(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	pages, _ := cmd.Flags().GetStringSlice("pages")
	category, _ := cmd.Flags().GetString("category")
	limit, _ := cmd.Flags().GetInt("limit")
	checksFlag, _ := cmd.Flags().GetString("checks")

	auditArgs := wiki.WikiHealthAuditArgs{
		Pages:    pages,
		Category: category,
		Limit:    limit,
	}

	if checksFlag != "" {
		for _, c := range strings.Split(checksFlag, ",") {
			auditArgs.Checks = append(auditArgs.Checks, strings.TrimSpace(c))
		}
	}

	result, err := client.HealthAudit(context.Background(), auditArgs)
	if err != nil {
		return fmt.Errorf("audit failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	// Human-readable output
	fmt.Printf("Wiki: %s\n", result.WikiName)
	fmt.Printf("Audited: %s (%d pages)\n\n", result.AuditedAt, result.PagesAudited)

	// Health score with visual bar
	fmt.Printf("Health Score: %d/100 %s\n\n", result.HealthScore, healthBar(result.HealthScore))

	// Broken internal links
	if result.BrokenLinks != nil && result.BrokenLinks.BrokenCount > 0 {
		fmt.Printf("Broken Internal Links: %d\n", result.BrokenLinks.BrokenCount)
		for _, page := range result.BrokenLinks.Pages {
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

	// Terminology issues
	if result.Terminology != nil && result.Terminology.IssuesFound > 0 {
		fmt.Printf("Terminology Issues: %d\n", result.Terminology.IssuesFound)
		for _, page := range result.Terminology.Pages {
			for _, issue := range page.Issues {
				fmt.Printf("  %s: %q should be %q (line %d)\n",
					page.Title, issue.Incorrect, issue.Correct, issue.Line)
			}
		}
		fmt.Println()
	}

	// Orphaned pages
	if result.OrphanedPages != nil && result.OrphanedPages.OrphanedCount > 0 {
		fmt.Printf("Orphaned Pages: %d\n", result.OrphanedPages.OrphanedCount)
		for _, page := range result.OrphanedPages.OrphanedPages {
			fmt.Printf("  %s (%d bytes)\n", page.Title, page.Length)
		}
		fmt.Println()
	}

	// External broken links
	if result.ExternalLinks != nil && result.ExternalLinks.BrokenCount > 0 {
		fmt.Printf("Broken External Links: %d\n", result.ExternalLinks.BrokenCount)
		for _, link := range result.ExternalLinks.Results {
			if link.Broken {
				fmt.Printf("  %s (%s)\n", link.URL, link.Error)
			}
		}
		fmt.Println()
	}

	// Recent activity
	if result.RecentActivity != nil && result.RecentActivity.TotalChanges > 0 {
		fmt.Printf("Recent Activity: %d changes\n", result.RecentActivity.TotalChanges)
		for _, item := range result.RecentActivity.Items {
			fmt.Printf("  %s: %d\n", item.Key, item.Count)
		}
		fmt.Println()
	}

	// Errors
	if len(result.Errors) > 0 {
		fmt.Printf("Errors (%d):\n", len(result.Errors))
		for _, e := range result.Errors {
			fmt.Printf("  %s\n", e)
		}
		fmt.Println()
	}

	// Summary
	fmt.Printf("Summary: %d broken links, %d terminology issues, %d orphaned pages, %d broken external links\n",
		result.Summary.BrokenLinksCount,
		result.Summary.TerminologyIssues,
		result.Summary.OrphanedPagesCount,
		result.Summary.ExternalBrokenCount)

	return nil
}

func healthBar(score int) string {
	filled := score / 10
	empty := 10 - filled
	return "[" + strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", empty) + "]"
}
