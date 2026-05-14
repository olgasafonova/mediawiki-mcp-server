package main

import (
	"context"
	"fmt"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newGrepCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grep",
		Short: "Search inside a single page or file attachment",
		Long: `Find text matches inside one wiki page or one uploaded file.

Different from 'wiki search', which does full-text search across the whole wiki.

  wiki grep page "API Reference" "timeout"
  wiki grep page "API Reference" "v\\d+" --regex --context 4
  wiki grep file "Annual-Report.pdf" "budget"`,
	}

	cmd.AddCommand(
		newGrepPageCmd(),
		newGrepFileCmd(),
	)

	return cmd
}

// ========== grep page ==========

func newGrepPageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "page <title> <query>",
		Short: "Find text matches inside one wiki page",
		Long: `Return matches with line, column, and surrounding context for a single page.

  wiki grep page "API Reference" "timeout"
  wiki grep page "Release Notes" "v\\d+\\.\\d+" --regex
  wiki grep page "FAQ" "deprecated" --context 5`,
		Args: cobra.ExactArgs(2),
		RunE: runGrepPage,
	}

	cmd.Flags().Bool("regex", false, "Treat query as a Go RE2 regex (escape . [ ] * + ? ( ) for literal)")
	cmd.Flags().Int("context", 2, "Lines of context around each match")

	return cmd
}

func runGrepPage(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	title, query := args[0], args[1]
	useRegex, _ := cmd.Flags().GetBool("regex")
	context, _ := cmd.Flags().GetInt("context")

	result, err := client.SearchInPage(cmdCtx(), wiki.SearchInPageArgs{
		Title:        title,
		Query:        query,
		UseRegex:     useRegex,
		ContextLines: context,
	})
	if err != nil {
		return fmt.Errorf("grep page failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if result.MatchCount == 0 {
		fmt.Printf("No matches for %q in %s\n", query, title)
		return nil
	}

	fmt.Printf("%s: %d match(es) for %q\n\n", result.Title, result.MatchCount, result.Query)
	for _, m := range result.Matches {
		fmt.Printf("L%d:%d\n", m.Line, m.Column)
		fmt.Printf("%s\n\n", m.Context)
	}
	return nil
}

// ========== grep file ==========

func newGrepFileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "file <filename> <query>",
		Short: "Find text matches inside a wiki-hosted file (PDF or text)",
		Long: `Search the contents of an uploaded file. PDF search requires 'pdftotext'
(install poppler-utils). Text formats (txt, md, csv, json, xml, html)
work without extra dependencies.

  wiki grep file "Annual-Report.pdf" "budget"
  wiki grep file "changelog.txt" "API"`,
		Args: cobra.ExactArgs(2),
		RunE: runGrepFile,
	}

	return cmd
}

func runGrepFile(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	filename, query := args[0], args[1]

	result, err := client.SearchInFile(cmdCtx(), wiki.SearchInFileArgs{
		Filename: filename,
		Query:    query,
	})
	if err != nil {
		return fmt.Errorf("grep file failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if !result.Searchable {
		fmt.Printf("%s is not searchable: %s\n", result.Filename, result.Message)
		return nil
	}
	if result.MatchCount == 0 {
		fmt.Printf("No matches for %q in %s (%s)\n", query, result.Filename, result.FileType)
		return nil
	}

	fmt.Printf("%s [%s]: %d match(es) for %q\n\n", result.Filename, result.FileType, result.MatchCount, query)
	for _, m := range result.Matches {
		switch {
		case m.Page > 0:
			fmt.Printf("p%d:%d\n", m.Page, m.Line)
		case m.Line > 0:
			fmt.Printf("L%d\n", m.Line)
		}
		fmt.Printf("%s\n\n", m.Context)
	}
	return nil
}

// cmdCtx is a tiny helper so subcommands stay short. Background context is the
// right default for one-shot CLI invocations; pipeline cancellation hooks would
// go here if we add them.
func cmdCtx() context.Context { return context.Background() }
