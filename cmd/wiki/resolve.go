package main

import (
	"context"
	"fmt"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <title>",
		Short: "Resolve a page title (handles case and fuzzy variants)",
		Long: `Resolve an inexact page title to the canonical wiki title.

Useful in shell scripts to avoid 404s when the caller's title is slightly off
(case, punctuation, redirect). Exits 0 on an exact match, 3 on no match.

  wiki resolve "getting started"
  wiki resolve "Releas Notes" --fuzzy
  wiki resolve "API" --fuzzy --max-results 10`,
		Args: cobra.ExactArgs(1),
		RunE: runResolve,
	}

	cmd.Flags().Bool("fuzzy", false, "Enable fuzzy matching for similar titles")
	cmd.Flags().Int("max-results", 5, "Max fuzzy suggestions to return")

	return cmd
}

func runResolve(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	title := args[0]
	fuzzy, _ := cmd.Flags().GetBool("fuzzy")
	maxResults, _ := cmd.Flags().GetInt("max-results")

	result, err := client.ResolveTitle(context.Background(), wiki.ResolveTitleArgs{
		Title:      title,
		Fuzzy:      fuzzy,
		MaxResults: maxResults,
	})
	if err != nil {
		return fmt.Errorf("resolve failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if result.ExactMatch {
		fmt.Println(result.ResolvedTitle)
		return nil
	}

	if len(result.Suggestions) == 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "No match for %q\n", title)
		return notFoundErr(fmt.Errorf("no match for %q", title))
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "No exact match for %q. Suggestions:\n\n", title)
	tw := table()
	fmt.Fprintf(tw, "SCORE\tTITLE\n")
	for _, s := range result.Suggestions {
		fmt.Fprintf(tw, "%.2f\t%s\n", s.Similarity, s.Title)
	}
	_ = tw.Flush()
	return notFoundErr(fmt.Errorf("no exact match"))
}
