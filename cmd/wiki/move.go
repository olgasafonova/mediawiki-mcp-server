package main

import (
	"context"
	"fmt"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newMoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "move <from> <to>",
		Short: "Rename a wiki page (leaves a redirect by default)",
		Long: `Move (rename) a wiki page from one title to another.

A redirect from the old title is created by default. Use --no-redirect to
suppress it (requires suppressredirect right on the wiki).

  wiki move "Old Guide" "Updated Guide"
  wiki move "Foo" "Bar" --reason "matches naming convention"
  wiki move "Internal" "Internal/Legacy" --subpages --no-redirect`,
		Args: cobra.ExactArgs(2),
		RunE: runMove,
	}

	cmd.Flags().String("reason", "", "Reason for the move (shown in move log)")
	cmd.Flags().Bool("no-redirect", false, "Don't leave a redirect from the old title")
	cmd.Flags().Bool("no-talk", false, "Don't move the talk page (default: move it)")
	cmd.Flags().Bool("subpages", false, "Also move subpages if they exist")

	return cmd
}

func runMove(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	from := args[0]
	to := args[1]
	reason, _ := cmd.Flags().GetString("reason")
	noRedirect, _ := cmd.Flags().GetBool("no-redirect")
	noTalk, _ := cmd.Flags().GetBool("no-talk")
	subpages, _ := cmd.Flags().GetBool("subpages")

	result, err := client.MovePage(context.Background(), wiki.MovePageArgs{
		From:         from,
		To:           to,
		Reason:       reason,
		NoRedirect:   noRedirect,
		MoveTalk:     !noTalk,
		MoveSubpages: subpages,
	})
	if err != nil {
		return fmt.Errorf("move failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	fmt.Printf("Moved %q -> %q", result.From, result.To)
	if result.TalkMoved {
		fmt.Print(" (talk page moved)")
	}
	fmt.Println()
	if result.Message != "" {
		fmt.Println(result.Message)
	}
	return nil
}
