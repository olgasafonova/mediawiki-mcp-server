package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List wiki pages, categories, members, and users",
		Long:  "Browse wiki content by listing pages, categories, category members, or users.",
	}

	cmd.AddCommand(
		newListPagesCmd(),
		newListCategoriesCmd(),
		newListMembersCmd(),
		newListUsersCmd(),
	)

	return cmd
}

func newListPagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pages",
		Short: "List wiki pages",
		Long:  "List pages in the wiki, optionally filtered by namespace or prefix.",
		RunE:  runListPages,
	}

	cmd.Flags().Int("namespace", 0, "Namespace to list (0=main, 1=talk, etc.)")
	cmd.Flags().String("prefix", "", "Filter pages by title prefix")
	cmd.Flags().IntP("limit", "n", 50, "Maximum pages to return")
	cmd.Flags().String("continue", "", "Continue from previous result set")

	return cmd
}

func runListPages(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	namespace, _ := cmd.Flags().GetInt("namespace")
	prefix, _ := cmd.Flags().GetString("prefix")
	limit, _ := cmd.Flags().GetInt("limit")
	continueFrom, _ := cmd.Flags().GetString("continue")

	result, err := client.ListPages(context.Background(), wiki.ListPagesArgs{
		Namespace:    namespace,
		Prefix:       prefix,
		Limit:        limit,
		ContinueFrom: continueFrom,
	})
	if err != nil {
		return fmt.Errorf("list pages failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if len(result.Pages) == 0 {
		fmt.Println("No pages found.")
		return nil
	}

	fmt.Printf("Pages (%d returned)", result.ReturnedCount)
	if result.TotalEstimate > 0 {
		fmt.Printf(", ~%d estimated total", result.TotalEstimate)
	}
	fmt.Println()
	fmt.Println()

	tw := table()
	fmt.Fprintf(tw, "ID\tTITLE\n")
	for _, p := range result.Pages {
		fmt.Fprintf(tw, "%d\t%s\n", p.PageID, p.Title)
	}
	_ = tw.Flush()

	if result.HasMore {
		fmt.Printf("\nMore pages available. Use --continue %q to see next page.\n", result.ContinueFrom)
	}

	return nil
}

func newListCategoriesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "categories",
		Short: "List wiki categories",
		Long:  "List categories in the wiki, optionally filtered by prefix.",
		RunE:  runListCategories,
	}

	cmd.Flags().String("prefix", "", "Filter categories by name prefix")
	cmd.Flags().IntP("limit", "n", 50, "Maximum categories to return")
	cmd.Flags().String("continue", "", "Continue from previous result set")

	return cmd
}

func runListCategories(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	prefix, _ := cmd.Flags().GetString("prefix")
	limit, _ := cmd.Flags().GetInt("limit")
	continueFrom, _ := cmd.Flags().GetString("continue")

	result, err := client.ListCategories(context.Background(), wiki.ListCategoriesArgs{
		Prefix:       prefix,
		Limit:        limit,
		ContinueFrom: continueFrom,
	})
	if err != nil {
		return fmt.Errorf("list categories failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if len(result.Categories) == 0 {
		fmt.Println("No categories found.")
		return nil
	}

	tw := table()
	fmt.Fprintf(tw, "CATEGORY\tMEMBERS\n")
	for _, c := range result.Categories {
		fmt.Fprintf(tw, "%s\t%d\n", c.Title, c.Members)
	}
	_ = tw.Flush()

	if result.HasMore {
		fmt.Printf("\nMore categories available. Use --continue %q to see next page.\n", result.ContinueFrom)
	}

	return nil
}

func newListMembersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "members <category>",
		Short: "List pages in a category",
		Long:  "List all pages that belong to a specific category.",
		Args:  cobra.ExactArgs(1),
		RunE:  runListMembers,
	}

	cmd.Flags().IntP("limit", "n", 50, "Maximum members to return")
	cmd.Flags().String("continue", "", "Continue from previous result set")

	return cmd
}

func runListMembers(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	category := args[0]
	limit, _ := cmd.Flags().GetInt("limit")
	continueFrom, _ := cmd.Flags().GetString("continue")

	result, err := client.GetCategoryMembers(context.Background(), wiki.CategoryMembersArgs{
		Category:     category,
		Limit:        limit,
		ContinueFrom: continueFrom,
	})
	if err != nil {
		return fmt.Errorf("list members failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if len(result.Members) == 0 {
		fmt.Printf("No members found in category %q.\n", category)
		return nil
	}

	fmt.Printf("Category %q", result.Category)
	if result.HasMore {
		fmt.Printf(" (showing %d)", len(result.Members))
	}
	fmt.Println()
	fmt.Println()

	tw := table()
	fmt.Fprintf(tw, "ID\tTITLE\n")
	for _, m := range result.Members {
		fmt.Fprintf(tw, "%d\t%s\n", m.PageID, m.Title)
	}
	_ = tw.Flush()

	if result.HasMore {
		fmt.Printf("\nMore members available. Use --continue %q to see next page.\n", result.ContinueFrom)
	}

	return nil
}

func newListUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "List wiki users",
		Long:  "List users on the wiki, optionally filtered by group (sysop, bureaucrat, bot).",
		RunE:  runListUsers,
	}

	cmd.Flags().String("group", "", "Filter by user group (sysop, bureaucrat, bot)")
	cmd.Flags().IntP("limit", "n", 50, "Maximum users to return")
	cmd.Flags().Bool("active", false, "Show only active users")
	cmd.Flags().String("continue", "", "Continue from previous result set")

	return cmd
}

func runListUsers(cmd *cobra.Command, args []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	group, _ := cmd.Flags().GetString("group")
	limit, _ := cmd.Flags().GetInt("limit")
	active, _ := cmd.Flags().GetBool("active")
	continueFrom, _ := cmd.Flags().GetString("continue")

	result, err := client.ListUsers(context.Background(), wiki.ListUsersArgs{
		Group:        group,
		Limit:        limit,
		ActiveOnly:   active,
		ContinueFrom: continueFrom,
	})
	if err != nil {
		return fmt.Errorf("list users failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	if len(result.Users) == 0 {
		fmt.Println("No users found.")
		return nil
	}

	label := "Users"
	if result.Group != "" {
		label = fmt.Sprintf("Users in group %q", result.Group)
	}
	fmt.Printf("%s (%d total)", label, result.TotalCount)
	if result.HasMore {
		fmt.Printf(", showing %d", len(result.Users))
	}
	fmt.Println()
	fmt.Println()

	tw := table()
	fmt.Fprintf(tw, "ID\tNAME\tGROUPS\tEDITS\tREGISTERED\n")
	for _, u := range result.Users {
		groups := strings.Join(u.Groups, ", ")
		fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%s\n", u.UserID, u.Name, groups, u.EditCount, u.Registration)
	}
	_ = tw.Flush()

	if result.HasMore {
		fmt.Printf("\nMore users available. Use --continue %q to see next page.\n", result.ContinueFrom)
	}

	return nil
}
