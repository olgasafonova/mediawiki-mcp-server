package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/olgasafonova/mediawiki-mcp-server/converter"
	"github.com/spf13/cobra"
)

func newPublishCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish <file.md> <page-title>",
		Short: "Convert a Markdown file to wikitext and publish",
		Long: `Read a Markdown file, convert it to MediaWiki markup, and publish to the wiki.

Use --preview to see the converted wikitext without publishing:

  wiki publish README.md "Getting Started" --preview
  wiki publish notes.md "Meeting Notes" --summary "Weekly sync notes"
  wiki publish doc.md "API Reference" --theme tieto --css`,
		Args: cobra.ExactArgs(2),
		RunE: runPublish,
	}

	cmd.Flags().String("summary", "", "Edit summary explaining the change")
	cmd.Flags().Bool("minor", false, "Mark as minor edit")
	cmd.Flags().String("theme", "neutral", "Conversion theme (neutral, tieto, dark)")
	cmd.Flags().Bool("css", false, "Include CSS styling block")
	cmd.Flags().Bool("preview", false, "Show converted wikitext without publishing")

	return cmd
}

func runPublish(cmd *cobra.Command, args []string) error {
	filePath := args[0]
	pageTitle := args[1]

	theme, _ := cmd.Flags().GetString("theme")
	addCSS, _ := cmd.Flags().GetBool("css")
	preview, _ := cmd.Flags().GetBool("preview")

	// Read the markdown file
	data, err := os.ReadFile(filePath) // #nosec G304 -- path from CLI argument
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	// Configure the converter
	cfg := converter.DefaultConfig()
	cfg.Theme = theme
	cfg.AddCSS = addCSS

	// Convert markdown to wikitext
	wikitext := converter.Convert(string(data), cfg)

	// Preview mode: print and stop
	if preview {
		return emitPublishPreview(cmd, filePath, pageTitle, wikitext)
	}

	return publishWikitext(cmd, filePath, pageTitle, wikitext)
}

// emitPublishPreview prints the converted wikitext without publishing.
func emitPublishPreview(cmd *cobra.Command, filePath, pageTitle, wikitext string) error {
	if isJSON(cmd) {
		return printJSON(map[string]string{
			"file":     filePath,
			"page":     pageTitle,
			"wikitext": wikitext,
		})
	}
	fmt.Printf("--- Preview: %s -> %s ---\n\n", filepath.Base(filePath), pageTitle)
	fmt.Println(wikitext)
	return nil
}

// publishWikitext submits the converted wikitext to the wiki, including the
// CAPTCHA retry loop and result reporting.
func publishWikitext(cmd *cobra.Command, filePath, pageTitle, wikitext string) error {
	summary, _ := cmd.Flags().GetString("summary")
	minor, _ := cmd.Flags().GetBool("minor")
	if summary == "" {
		summary = fmt.Sprintf("Published from %s", filepath.Base(filePath))
	}

	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	req := editRequest{title: pageTitle, content: wikitext, summary: summary, minor: minor}
	result, err := client.EditPage(cmd.Context(), req.toArgs())
	if err != nil {
		return fmt.Errorf("publish failed: %w", err)
	}

	// CAPTCHA retry loop
	session := editSession{client: client, req: req}
	result = session.retryLoop(cmd, result)

	if isJSON(cmd) {
		return printJSON(result)
	}

	if !result.Success {
		return fmt.Errorf("publish failed: %s", result.Message)
	}

	fmt.Printf("Published %s -> %s (rev: %d)\n", filepath.Base(filePath), result.Title, result.RevisionID)
	return nil
}
