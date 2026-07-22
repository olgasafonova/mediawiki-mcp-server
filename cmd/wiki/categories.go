package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newCategoriesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "categories <page>",
		Short: "Add or remove categories on a page (no body edit)",
		Long: `Manage a page's category memberships without touching the rest of its content.

Provide --add and/or --remove with comma-separated category names (no
'Category:' prefix). Use --preview to see what would change without saving.

  wiki categories "API Reference" --add "Documentation,API"
  wiki categories "Old Guide" --remove "Active" --add "Deprecated"
  wiki categories "Draft" --add "Documentation" --preview`,
		Args: cobra.ExactArgs(1),
		RunE: runCategories,
	}

	cmd.Flags().String("add", "", "Comma-separated categories to add")
	cmd.Flags().String("remove", "", "Comma-separated categories to remove")
	cmd.Flags().String("summary", "", "Edit summary")
	cmd.Flags().Bool("preview", false, "Preview changes without saving")

	return cmd
}

func runCategories(cmd *cobra.Command, args []string) error {
	title := args[0]
	addStr, _ := cmd.Flags().GetString("add")
	removeStr, _ := cmd.Flags().GetString("remove")
	summary, _ := cmd.Flags().GetString("summary")
	preview, _ := cmd.Flags().GetBool("preview")

	add := splitCSV(addStr)
	remove := splitCSV(removeStr)
	if len(add) == 0 && len(remove) == 0 {
		return usageErr(fmt.Errorf("at least one of --add or --remove is required"))
	}

	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.ManageCategories(context.Background(), wiki.ManageCategoriesArgs{
		Title:   title,
		Add:     add,
		Remove:  remove,
		Summary: summary,
		Preview: &preview,
	})
	if err != nil {
		return fmt.Errorf("categories failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if preview {
		fmt.Printf("Preview for %s\n", result.Title)
	} else {
		fmt.Printf("Updated categories on %s (rev: %d)\n", result.Title, result.RevisionID)
	}
	if len(result.Added) > 0 {
		fmt.Printf("  added:           %s\n", strings.Join(result.Added, ", "))
	}
	if len(result.Removed) > 0 {
		fmt.Printf("  removed:         %s\n", strings.Join(result.Removed, ", "))
	}
	if len(result.AlreadyPresent) > 0 {
		fmt.Printf("  already present: %s\n", strings.Join(result.AlreadyPresent, ", "))
	}
	if len(result.NotFound) > 0 {
		fmt.Printf("  not present:     %s\n", strings.Join(result.NotFound, ", "))
	}
	fmt.Printf("  current:         %s\n", strings.Join(result.CurrentCategories, ", "))
	return nil
}

// splitCSV splits a comma-separated string and trims whitespace from each item,
// dropping empties. Returns nil for an empty input so the args struct keeps
// the field as a missing-value rather than an empty slice.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
