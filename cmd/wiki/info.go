package main

import (
	"context"
	"fmt"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show wiki installation and statistics",
		Long: `Show the wiki's site name, generator version, server, language,
and content statistics (pages, articles, edits, users, admins).

  wiki info
  wiki info --json   # for monitoring scripts`,
		Args: cobra.NoArgs,
		RunE: runInfo,
	}

	return cmd
}

func runInfo(cmd *cobra.Command, _ []string) error {
	client, err := newWikiClient(cmd)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.GetWikiInfo(context.Background(), wiki.WikiInfoArgs{})
	if err != nil {
		return fmt.Errorf("info failed: %w", err)
	}

	if isJSON(cmd) {
		return printJSON(result)
	}

	fmt.Printf("Site:        %s\n", result.SiteName)
	fmt.Printf("Server:      %s\n", result.Server)
	fmt.Printf("Generator:   %s\n", result.Generator)
	fmt.Printf("PHP:         %s\n", result.PHPVersion)
	fmt.Printf("Language:    %s\n", result.Language)
	fmt.Printf("Timezone:    %s\n", result.Timezone)
	fmt.Printf("Main page:   %s\n", result.MainPage)
	fmt.Printf("Write API:   %v\n", result.WriteAPI)

	if result.Statistics != nil {
		s := result.Statistics
		fmt.Println()
		fmt.Println("Statistics:")
		fmt.Printf("  pages:        %d\n", s.Pages)
		fmt.Printf("  articles:     %d\n", s.Articles)
		fmt.Printf("  edits:        %d\n", s.Edits)
		fmt.Printf("  images:       %d\n", s.Images)
		fmt.Printf("  users:        %d\n", s.Users)
		fmt.Printf("  active users: %d\n", s.ActiveUsers)
		fmt.Printf("  admins:       %d\n", s.Admins)
	}

	return nil
}
