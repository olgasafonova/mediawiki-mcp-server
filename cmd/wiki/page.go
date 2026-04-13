package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newPageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "page <title> [title2 ...]",
		Short: "Read wiki page content",
		Long: `Retrieve the content of one or more wiki pages.

With a single title, prints the full page content.
With multiple titles, fetches all pages in a single batch request (up to 50).`,
		Args: cobra.MinimumNArgs(1),
		RunE: runPage,
	}

	cmd.Flags().Bool("summary", false, "Show lead section and metadata only")
	cmd.Flags().Bool("sections", false, "Show table of contents (section structure)")
	cmd.Flags().Bool("info", false, "Show page metadata only (no content)")
	cmd.Flags().Bool("related", false, "Show related pages via links and categories")
	cmd.Flags().Bool("images", false, "Show images used on the page")
	cmd.Flags().Int("section", -1, "Read a specific section by index")
	cmd.Flags().String("format", "wikitext", "Output format: wikitext or html")

	return cmd
}

func runPage(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	ctx := context.Background()

	// Dispatch to subcommand based on flags
	if v, _ := cmd.Flags().GetBool("summary"); v {
		return runPageSummary(cmd, client, ctx, args[0])
	}
	if v, _ := cmd.Flags().GetBool("sections"); v {
		return runPageSections(cmd, client, ctx, args[0])
	}
	if v, _ := cmd.Flags().GetBool("info"); v {
		return runPageInfo(cmd, client, ctx, args)
	}
	if v, _ := cmd.Flags().GetBool("related"); v {
		return runPageRelated(cmd, client, ctx, args[0])
	}
	if v, _ := cmd.Flags().GetBool("images"); v {
		return runPageImages(cmd, client, ctx, args[0])
	}

	// Default: read page content
	if len(args) > 1 {
		return runPageBatch(cmd, client, ctx, args)
	}
	return runPageSingle(cmd, client, ctx, args[0])
}

func runPageSingle(cmd *cobra.Command, client *wiki.Client, ctx context.Context, title string) error {
	format, _ := cmd.Flags().GetString("format")
	sectionIdx, _ := cmd.Flags().GetInt("section")

	// If requesting a specific section, use GetSections
	if sectionIdx >= 0 {
		result, err := client.GetSections(ctx, wiki.GetSectionsArgs{
			Title:   title,
			Section: sectionIdx,
		})
		if err != nil {
			return fmt.Errorf("failed to get section: %w", err)
		}
		if isJSON(cmd) {
			return printJSON(result)
		}
		if result.SectionContent != "" {
			fmt.Println(result.SectionContent)
		} else {
			fmt.Printf("Section %d not found in %q\n", sectionIdx, title)
		}
		return nil
	}

	result, err := client.GetPage(ctx, wiki.GetPageArgs{
		Title:  title,
		Format: format,
	})
	if err != nil {
		return fmt.Errorf("failed to get page: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	// Human-readable: header + content
	if !isQuiet(cmd) {
		fmt.Printf("# %s", result.Title)
		if result.Timestamp != "" {
			fmt.Printf("    [rev:%d %s]", result.Revision, result.Timestamp)
		}
		fmt.Println()
		if result.Truncated {
			fmt.Println("(content truncated)")
		}
		fmt.Println()
	}
	fmt.Println(result.Content)
	return nil
}

func runPageBatch(cmd *cobra.Command, client *wiki.Client, ctx context.Context, titles []string) error {
	format, _ := cmd.Flags().GetString("format")

	result, err := client.GetPagesBatch(ctx, wiki.GetPagesBatchArgs{
		Titles: titles,
		Format: format,
	})
	if err != nil {
		return fmt.Errorf("batch get failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	for i, page := range result.Pages {
		if i > 0 {
			fmt.Println(strings.Repeat("─", 60))
		}
		if !page.Exists {
			fmt.Printf("# %s  [NOT FOUND]\n\n", page.Title)
			continue
		}
		fmt.Printf("# %s    [rev:%d]\n\n", page.Title, page.Revision)
		fmt.Println(page.Content)
		fmt.Println()
	}

	if !isQuiet(cmd) {
		fmt.Printf("(%d found, %d missing)\n", result.FoundCount, result.MissingCount)
	}
	return nil
}

func runPageSummary(cmd *cobra.Command, client *wiki.Client, ctx context.Context, title string) error {
	result, err := client.GetPageSummary(ctx, wiki.GetPageSummaryArgs{Title: title})
	if err != nil {
		return fmt.Errorf("failed to get summary: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	fmt.Printf("# %s\n\n", result.Title)
	fmt.Println(result.LeadContent)
	if len(result.Categories) > 0 {
		fmt.Printf("\nCategories: %s\n", strings.Join(result.Categories, ", "))
	}
	if len(result.Sections) > 0 {
		fmt.Println("\nSections:")
		for i, s := range result.Sections {
			fmt.Printf("  %d. %s\n", i+1, s)
		}
	}
	return nil
}

func runPageSections(cmd *cobra.Command, client *wiki.Client, ctx context.Context, title string) error {
	result, err := client.GetSections(ctx, wiki.GetSectionsArgs{Title: title})
	if err != nil {
		return fmt.Errorf("failed to get sections: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	fmt.Printf("Sections in %q:\n\n", title)
	for _, s := range result.Sections {
		indent := strings.Repeat("  ", s.Level-1)
		fmt.Printf("  %s[%d] %s\n", indent, s.Index, s.Title)
	}
	fmt.Printf("\nUse: wiki page %q --section N  to read a specific section.\n", title)
	return nil
}

func runPageInfo(cmd *cobra.Command, client *wiki.Client, ctx context.Context, titles []string) error {
	if len(titles) > 1 {
		result, err := client.GetPagesInfoBatch(ctx, wiki.GetPagesInfoBatchArgs{Titles: titles})
		if err != nil {
			return fmt.Errorf("failed to get info: %w", err)
		}
		if isJSON(cmd) {
			return printJSON(result)
		}
		tw := table()
		fmt.Fprintf(tw, "TITLE\tSIZE\tLAST REVISION\tREDIRECT\n")
		for _, p := range result.Pages {
			if !p.Exists {
				fmt.Fprintf(tw, "%s\t-\t-\t(not found)\n", p.Title)
				continue
			}
			redirect := ""
			if p.Redirect {
				redirect = "-> " + p.RedirectTo
			}
			fmt.Fprintf(tw, "%s\t%d\t%d\t%s\n", p.Title, p.Length, p.LastRevision, redirect)
		}
		_ = tw.Flush()
		return nil
	}

	result, err := client.GetPageInfo(ctx, wiki.PageInfoArgs{Title: titles[0]})
	if err != nil {
		return fmt.Errorf("failed to get info: %w", err)
	}
	if isJSON(cmd) {
		return printJSON(result)
	}

	fmt.Printf("Title:       %s\n", result.Title)
	fmt.Printf("Page ID:     %d\n", result.PageID)
	fmt.Printf("Size:        %d bytes\n", result.Length)
	fmt.Printf("Last rev:    %d\n", result.LastRevision)
	fmt.Printf("Touched:     %s\n", result.Touched)
	if result.Redirect {
		fmt.Printf("Redirect:    -> %s\n", result.RedirectTo)
	}
	if len(result.Protection) > 0 {
		fmt.Printf("Protected:   %s\n", strings.Join(result.Protection, ", "))
	}
	return nil
}

func runPageRelated(cmd *cobra.Command, client *wiki.Client, ctx context.Context, title string) error {
	result, err := client.GetRelated(ctx, wiki.GetRelatedArgs{Title: title})
	if err != nil {
		return fmt.Errorf("failed to get related: %w", err)
	}
	if isJSON(cmd) {
		return printJSON(result)
	}

	fmt.Printf("Pages related to %q:\n\n", title)
	for _, r := range result.RelatedPages {
		fmt.Printf("  %s  (%s)\n", r.Title, r.Relation)
	}
	if len(result.RelatedPages) == 0 {
		fmt.Println("  (no related pages found)")
	}
	return nil
}

func runPageImages(cmd *cobra.Command, client *wiki.Client, ctx context.Context, title string) error {
	result, err := client.GetImages(ctx, wiki.GetImagesArgs{Title: title})
	if err != nil {
		return fmt.Errorf("failed to get images: %w", err)
	}
	if isJSON(cmd) {
		return printJSON(result)
	}

	fmt.Printf("Images on %q:\n\n", title)
	tw := table()
	fmt.Fprintf(tw, "FILENAME\tSIZE\tDIMENSIONS\n")
	for _, img := range result.Images {
		dims := ""
		if img.Width > 0 {
			dims = fmt.Sprintf("%dx%d", img.Width, img.Height)
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\n", img.Title, img.Size, dims)
	}
	_ = tw.Flush()
	return nil
}
