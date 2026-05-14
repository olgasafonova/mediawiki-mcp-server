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

func buildAuditArgs(cmd *cobra.Command) wiki.WikiHealthAuditArgs {
	pages, _ := cmd.Flags().GetStringSlice("pages")
	category, _ := cmd.Flags().GetString("category")
	limit, _ := cmd.Flags().GetInt("limit")
	checksFlag, _ := cmd.Flags().GetString("checks")

	out := wiki.WikiHealthAuditArgs{
		Pages:    pages,
		Category: category,
		Limit:    limit,
	}
	if checksFlag != "" {
		for _, c := range strings.Split(checksFlag, ",") {
			out.Checks = append(out.Checks, strings.TrimSpace(c))
		}
	}
	return out
}

func printAuditBrokenInternalLinks(r *wiki.FindBrokenInternalLinksResult) {
	if r == nil || r.BrokenCount == 0 {
		return
	}
	fmt.Printf("Broken Internal Links: %d\n", r.BrokenCount)
	for _, page := range r.Pages {
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

func printAuditTerminology(r *wiki.CheckTerminologyResult) {
	if r == nil || r.IssuesFound == 0 {
		return
	}
	fmt.Printf("Terminology Issues: %d\n", r.IssuesFound)
	for _, page := range r.Pages {
		for _, issue := range page.Issues {
			fmt.Printf("  %s: %q should be %q (line %d)\n",
				page.Title, issue.Incorrect, issue.Correct, issue.Line)
		}
	}
	fmt.Println()
}

func printAuditOrphaned(r *wiki.FindOrphanedPagesResult) {
	if r == nil || r.OrphanedCount == 0 {
		return
	}
	fmt.Printf("Orphaned Pages: %d\n", r.OrphanedCount)
	for _, page := range r.OrphanedPages {
		fmt.Printf("  %s (%d bytes)\n", page.Title, page.Length)
	}
	fmt.Println()
}

func printAuditExternalLinks(r *wiki.CheckLinksResult) {
	if r == nil || r.BrokenCount == 0 {
		return
	}
	fmt.Printf("Broken External Links: %d\n", r.BrokenCount)
	for _, link := range r.Results {
		if link.Broken {
			fmt.Printf("  %s (%s)\n", link.URL, link.Error)
		}
	}
	fmt.Println()
}

func printAuditActivity(r *wiki.AggregatedChanges) {
	if r == nil || r.TotalChanges == 0 {
		return
	}
	fmt.Printf("Recent Activity: %d changes\n", r.TotalChanges)
	for _, item := range r.Items {
		fmt.Printf("  %s: %d\n", item.Key, item.Count)
	}
	fmt.Println()
}

func printAuditErrors(errs []string) {
	if len(errs) == 0 {
		return
	}
	fmt.Printf("Errors (%d):\n", len(errs))
	for _, e := range errs {
		fmt.Printf("  %s\n", e)
	}
	fmt.Println()
}

func printAuditHuman(result *wiki.WikiHealthAuditResult) {
	fmt.Printf("Wiki: %s\n", result.WikiName)
	fmt.Printf("Audited: %s (%d pages)\n\n", result.AuditedAt, result.PagesAudited)
	fmt.Printf("Health Score: %d/100 %s\n\n", result.HealthScore, healthBar(result.HealthScore))

	printAuditBrokenInternalLinks(result.BrokenLinks)
	printAuditTerminology(result.Terminology)
	printAuditOrphaned(result.OrphanedPages)
	printAuditExternalLinks(result.ExternalLinks)
	printAuditActivity(result.RecentActivity)
	printAuditErrors(result.Errors)

	fmt.Printf("Summary: %d broken links, %d terminology issues, %d orphaned pages, %d broken external links\n",
		result.Summary.BrokenLinksCount,
		result.Summary.TerminologyIssues,
		result.Summary.OrphanedPagesCount,
		result.Summary.ExternalBrokenCount)
}

func runAudit(cmd *cobra.Command, _ []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.HealthAudit(context.Background(), buildAuditArgs(cmd))
	if err != nil {
		return fmt.Errorf("audit failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	printAuditHuman(&result)
	return nil
}

func healthBar(score int) string {
	filled := score / 10
	empty := 10 - filled
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", empty) + "]"
}
