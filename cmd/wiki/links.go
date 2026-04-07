package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newLinksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "links",
		Short: "Link analysis — external links, backlinks, broken links, orphans",
		Long:  "Inspect and validate links on wiki pages. Find external URLs, backlinks, broken internal links, and orphaned pages.",
	}

	cmd.AddCommand(
		newLinksExternalCmd(),
		newLinksBacklinksCmd(),
		newLinksBrokenCmd(),
		newLinksCheckCmd(),
		newLinksOrphanedCmd(),
	)

	return cmd
}

// ========== external ==========

func newLinksExternalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "external <title>",
		Short: "Show external links on a page",
		Long:  "List all external URLs referenced on a wiki page.",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runLinksExternal,
	}

	return cmd
}

func runLinksExternal(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	ctx := context.Background()
	title := strings.Join(args, " ")

	// Use batch if multiple titles are passed (pipe-separated? no — just join as title)
	result, err := client.GetExternalLinks(ctx, wiki.GetExternalLinksArgs{
		Title: title,
	})
	if err != nil {
		return fmt.Errorf("failed to get external links: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if len(result.Links) == 0 {
		fmt.Printf("No external links on %q\n", result.Title)
		return nil
	}

	fmt.Printf("External links on %q (%d):\n\n", result.Title, result.Count)

	tw := table()
	fmt.Fprintf(tw, "URL\n")
	for _, link := range result.Links {
		fmt.Fprintf(tw, "%s\n", link.URL)
	}
	tw.Flush()

	return nil
}

// ========== backlinks ==========

func newLinksBacklinksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backlinks <title>",
		Short: "Show pages linking to this page",
		Long:  "Find all pages that contain a link to the specified page (\"What links here\").",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runLinksBacklinks,
	}

	cmd.Flags().IntP("limit", "n", 50, "Maximum results to return")
	cmd.Flags().Int("namespace", 0, "Filter by namespace (-1 for all, 0 for main)")
	cmd.Flags().Bool("redirects", false, "Include redirect pages in results")

	return cmd
}

func runLinksBacklinks(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	limit, _ := cmd.Flags().GetInt("limit")
	namespace, _ := cmd.Flags().GetInt("namespace")
	redirects, _ := cmd.Flags().GetBool("redirects")
	title := strings.Join(args, " ")

	result, err := client.GetBacklinks(context.Background(), wiki.GetBacklinksArgs{
		Title:     title,
		Namespace: namespace,
		Limit:     limit,
		Redirect:  redirects,
	})
	if err != nil {
		return fmt.Errorf("failed to get backlinks: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if len(result.Backlinks) == 0 {
		fmt.Printf("No pages link to %q\n", result.Title)
		return nil
	}

	fmt.Printf("Pages linking to %q (%d)", result.Title, result.Count)
	if result.HasMore {
		fmt.Printf(" (showing %d)", len(result.Backlinks))
	}
	fmt.Println()
	fmt.Println()

	tw := table()
	fmt.Fprintf(tw, "TITLE\tNAMESPACE\tREDIRECT\n")
	for _, bl := range result.Backlinks {
		redirect := ""
		if bl.IsRedirect {
			redirect = "yes"
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\n", bl.Title, bl.Namespace, redirect)
	}
	tw.Flush()

	if result.HasMore {
		fmt.Printf("\nMore results available. Use --limit %d to see more.\n", limit*2)
	}

	return nil
}

// ========== broken ==========

func newLinksBrokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "broken [page1 page2 ...]",
		Short: "Find broken internal links",
		Long: `Find broken internal wiki links on specified pages or within a category.

Provide page titles as arguments, or use --category to check all pages in a category.`,
		RunE: runLinksBroken,
	}

	cmd.Flags().String("category", "", "Category to check (alternative to listing pages)")
	cmd.Flags().Int("limit", 20, "Maximum pages to check")

	return cmd
}

func runLinksBroken(cmd *cobra.Command, args []string) error {
	category, _ := cmd.Flags().GetString("category")

	if len(args) == 0 && category == "" {
		return fmt.Errorf("provide page titles as arguments or use --category")
	}

	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	limit, _ := cmd.Flags().GetInt("limit")

	result, err := client.FindBrokenInternalLinks(context.Background(), wiki.FindBrokenInternalLinksArgs{
		Pages:    args,
		Category: category,
		Limit:    limit,
	})
	if err != nil {
		return fmt.Errorf("failed to find broken links: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if result.BrokenCount == 0 {
		fmt.Printf("No broken internal links found (%d pages checked).\n", result.PagesChecked)
		return nil
	}

	fmt.Printf("Found %d broken internal links across %d pages:\n\n", result.BrokenCount, result.PagesChecked)

	for _, page := range result.Pages {
		if page.BrokenCount == 0 {
			continue
		}
		fmt.Printf("  %s (%d broken):\n", page.Title, page.BrokenCount)
		for _, link := range page.BrokenLinks {
			if link.Line > 0 {
				fmt.Printf("    -> %s (line %d)\n", link.Target, link.Line)
			} else {
				fmt.Printf("    -> %s\n", link.Target)
			}
		}
		fmt.Println()
	}

	return nil
}

// ========== check ==========

func newLinksCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check <url1> [url2 ...]",
		Short: "Check if URLs are accessible",
		Long:  "Verify that external URLs respond with a valid HTTP status.",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runLinksCheck,
	}

	cmd.Flags().Int("timeout", 10, "Timeout per URL in seconds")

	return cmd
}

func runLinksCheck(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	timeout, _ := cmd.Flags().GetInt("timeout")

	result, err := client.CheckLinks(context.Background(), wiki.CheckLinksArgs{
		URLs:    args,
		Timeout: timeout,
	})
	if err != nil {
		return fmt.Errorf("failed to check links: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	fmt.Printf("Checked %d URLs: %d valid, %d broken\n\n", result.TotalLinks, result.ValidCount, result.BrokenCount)

	tw := table()
	fmt.Fprintf(tw, "URL\tSTATUS\tCODE\n")
	for _, r := range result.Results {
		status := "OK"
		if r.Broken {
			status = "BROKEN"
		}
		code := ""
		if r.StatusCode > 0 {
			code = fmt.Sprintf("%d", r.StatusCode)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.URL, status, code)
	}
	tw.Flush()

	return nil
}

// ========== orphaned ==========

func newLinksOrphanedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orphaned",
		Short: "Find pages with no incoming links",
		Long:  "Discover wiki pages that no other page links to. These are candidates for linking or cleanup.",
		RunE:  runLinksOrphaned,
	}

	cmd.Flags().Int("namespace", 0, "Namespace to check (0=main, -1=all)")
	cmd.Flags().IntP("limit", "n", 50, "Maximum pages to return")
	cmd.Flags().String("prefix", "", "Only check pages starting with this prefix")

	return cmd
}

func runLinksOrphaned(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	namespace, _ := cmd.Flags().GetInt("namespace")
	limit, _ := cmd.Flags().GetInt("limit")
	prefix, _ := cmd.Flags().GetString("prefix")

	result, err := client.FindOrphanedPages(context.Background(), wiki.FindOrphanedPagesArgs{
		Namespace: namespace,
		Limit:     limit,
		Prefix:    prefix,
	})
	if err != nil {
		return fmt.Errorf("failed to find orphaned pages: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if result.OrphanedCount == 0 {
		fmt.Printf("No orphaned pages found (%d pages checked).\n", result.TotalChecked)
		return nil
	}

	fmt.Printf("Found %d orphaned pages (%d checked):\n\n", result.OrphanedCount, result.TotalChecked)

	tw := table()
	fmt.Fprintf(tw, "TITLE\tSIZE\tLAST EDITED\n")
	for _, page := range result.OrphanedPages {
		fmt.Fprintf(tw, "%s\t%d\t%s\n", page.Title, page.Length, page.LastEdited)
	}
	tw.Flush()

	return nil
}
