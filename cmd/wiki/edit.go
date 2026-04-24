package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <title>",
		Short: "Create or edit a wiki page",
		Long: `Edit a wiki page with new content. Content can be provided via --content flag
or piped from stdin:

  wiki edit "Page Title" --content "= Hello ="
  cat page.wiki | wiki edit "Page Title" --summary "Update from file"`,
		Args: cobra.ExactArgs(1),
		RunE: runEdit,
	}

	cmd.Flags().String("content", "", "Page content in wikitext format")
	cmd.Flags().String("summary", "", "Edit summary explaining the change")
	cmd.Flags().Bool("minor", false, "Mark as minor edit")
	cmd.Flags().Bool("bot", false, "Mark as bot edit (requires bot flag)")
	cmd.Flags().String("section", "", "Section to edit ('new' for new section, number for existing)")

	return cmd
}

func runEdit(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	title := args[0]
	content, _ := cmd.Flags().GetString("content")
	summary, _ := cmd.Flags().GetString("summary")
	minor, _ := cmd.Flags().GetBool("minor")
	bot, _ := cmd.Flags().GetBool("bot")
	section, _ := cmd.Flags().GetString("section")

	// If --content is empty, try reading from stdin
	if content == "" {
		content, err = readStdin()
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
	}

	if content == "" {
		return fmt.Errorf("content is required (use --content or pipe from stdin)")
	}

	result, err := client.EditPage(context.Background(), wiki.EditPageArgs{
		Title:   title,
		Content: content,
		Summary: summary,
		Minor:   minor,
		Bot:     bot,
		Section: section,
	})
	if err != nil {
		return fmt.Errorf("edit failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	// Human-readable output
	if result.NewPage {
		fmt.Printf("Created %s (rev: %d)\n", result.Title, result.RevisionID)
	} else {
		fmt.Printf("Edited %s (rev: %d)\n", result.Title, result.RevisionID)
	}

	return nil
}

// readStdin reads all available data from stdin if it's not a terminal.
func readStdin() (string, error) {
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}

	// No piped data
	if info.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}

	var sb strings.Builder
	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer for large pages
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return sb.String(), nil
}
