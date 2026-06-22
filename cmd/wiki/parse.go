package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newParseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "parse [wikitext]",
		Short: "Render wikitext to HTML (preview)",
		Long: `Parse wikitext and return the rendered HTML plus extracted categories
and links. Useful for previewing how markup will render before publishing.

Provide the wikitext as a positional argument, via --file, or piped on stdin.
Use --title to give the parser page context (affects template expansion).

  wiki parse "'''bold''' and [[Main Page]]"
  wiki parse --file draft.wiki --title "Draft"
  cat draft.wiki | wiki parse --json`,
		Args: cobra.ArbitraryArgs,
		RunE: runParse,
	}

	cmd.Flags().StringP("file", "f", "", "Path to a file containing wikitext")
	cmd.Flags().String("title", "", "Page title for template/context resolution")

	return cmd
}

func runParse(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	wikitext, err := loadWikitext(cmd, args)
	if err != nil {
		return err
	}
	if wikitext == "" {
		return usageErr(fmt.Errorf("wikitext is required (pass an argument, --file, or pipe from stdin)"))
	}

	title, _ := cmd.Flags().GetString("title")

	result, err := client.Parse(context.Background(), wiki.ParseArgs{
		Wikitext: wikitext,
		Title:    title,
	})
	if err != nil {
		return fmt.Errorf("parse failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	printParseResult(result)
	return nil
}

func printParseResult(result wiki.ParseResult) {
	fmt.Println(result.HTML)
	if len(result.Categories) > 0 {
		fmt.Printf("\nCategories: %v\n", result.Categories)
	}
	if result.Truncated && result.Message != "" {
		fmt.Printf("\n%s\n", result.Message)
	}
}

// loadWikitext resolves wikitext content from a positional arg, --file, or stdin,
// in that order. Shared by `parse` and `format`.
func loadWikitext(cmd *cobra.Command, args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	if filePath, _ := cmd.Flags().GetString("file"); filePath != "" {
		fileBytes, err := os.ReadFile(filePath) // #nosec G304 -- path supplied via CLI flag by the invoking user
		if err != nil {
			return "", fmt.Errorf("failed to read file: %w", err)
		}
		return string(fileBytes), nil
	}
	content, err := readStdin()
	if err != nil {
		return "", fmt.Errorf("failed to read stdin: %w", err)
	}
	return content, nil
}
