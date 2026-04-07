package main

import (
	"context"
	"fmt"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history [title]",
		Short: "Show page revision history or user contributions",
		Long: `Show the revision history for a page, or a user's contributions across all pages.

With a title argument, shows the page's revision history.
With --user flag (no title), shows that user's contributions.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runHistory,
	}

	cmd.Flags().IntP("limit", "n", 20, "Maximum results to return")
	cmd.Flags().String("start", "", "Start timestamp (ISO 8601)")
	cmd.Flags().String("end", "", "End timestamp (ISO 8601)")
	cmd.Flags().String("user", "", "Show contributions for this user (instead of page history)")

	return cmd
}

func runHistory(cmd *cobra.Command, args []string) error {
	user, _ := cmd.Flags().GetString("user")

	// Dispatch: user contributions or page history
	if len(args) == 0 && user == "" {
		return fmt.Errorf("provide a page title or --user flag")
	}
	if len(args) == 0 && user != "" {
		return runUserContributions(cmd, user)
	}
	return runPageHistory(cmd, args[0])
}

func runPageHistory(cmd *cobra.Command, title string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	limit, _ := cmd.Flags().GetInt("limit")
	start, _ := cmd.Flags().GetString("start")
	end, _ := cmd.Flags().GetString("end")
	user, _ := cmd.Flags().GetString("user")

	result, err := client.GetRevisions(context.Background(), wiki.GetRevisionsArgs{
		Title: title,
		Limit: limit,
		Start: start,
		End:   end,
		User:  user,
	})
	if err != nil {
		return fmt.Errorf("failed to get revisions: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	fmt.Printf("History for %q (%d revisions)", result.Title, result.Count)
	if result.HasMore {
		fmt.Print(" (more available)")
	}
	fmt.Println()
	fmt.Println()

	tw := table()
	fmt.Fprintf(tw, "REV\tUSER\tDATE\tSIZE\tCOMMENT\n")
	for _, r := range result.Revisions {
		comment := truncate(r.Comment, 60)
		fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%s\n",
			r.RevID, r.User, r.Timestamp, r.Size, comment)
	}
	tw.Flush()

	return nil
}

func runUserContributions(cmd *cobra.Command, user string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	limit, _ := cmd.Flags().GetInt("limit")
	start, _ := cmd.Flags().GetString("start")
	end, _ := cmd.Flags().GetString("end")

	result, err := client.GetUserContributions(context.Background(), wiki.GetUserContributionsArgs{
		User:  user,
		Limit: limit,
		Start: start,
		End:   end,
	})
	if err != nil {
		return fmt.Errorf("failed to get contributions: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	fmt.Printf("Contributions by %q (%d edits)", result.User, result.Count)
	if result.HasMore {
		fmt.Print(" (more available)")
	}
	fmt.Println()
	fmt.Println()

	tw := table()
	fmt.Fprintf(tw, "TITLE\tREV\tDATE\tSIZE\tCOMMENT\n")
	for _, c := range result.Contributions {
		comment := truncate(c.Comment, 60)
		fmt.Fprintf(tw, "%s\t%d\t%s\t%d\t%s\n",
			c.Title, c.RevID, c.Timestamp, c.Size, comment)
	}
	tw.Flush()

	return nil
}
