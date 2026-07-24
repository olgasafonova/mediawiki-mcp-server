package main

import (
	"context"
	"fmt"
	"os"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or verify wiki configuration",
		Long: `Display current wiki configuration from environment variables.
Use --wiki to also fetch wiki server info (requires connectivity).`,
		RunE: runConfig,
	}

	cmd.Flags().Bool("wiki", false, "Also fetch wiki server info (name, version, stats)")

	return cmd
}

func runConfig(cmd *cobra.Command, args []string) error {
	url := resolveConfigURL(cmd)
	user := os.Getenv("MEDIAWIKI_USERNAME")

	if url == "" {
		printConfigHelp()
		return nil
	}

	if isJSON(cmd) {
		return printJSON(map[string]interface{}{
			"url":      url,
			"username": user,
			"auth":     user != "",
		})
	}

	fmt.Printf("Wiki URL:  %s\n", url)
	if user != "" {
		fmt.Printf("Username:  %s\n", user)
		fmt.Printf("Auth:      configured\n")
	} else {
		fmt.Printf("Auth:      anonymous (read-only)\n")
	}

	if showWiki, _ := cmd.Flags().GetBool("wiki"); showWiki {
		return printWikiInfo(cmd)
	}

	return nil
}

// resolveConfigURL returns the wiki URL from the environment, falling back
// to the --url flag.
func resolveConfigURL(cmd *cobra.Command) string {
	url := os.Getenv("MEDIAWIKI_URL")
	if url != "" {
		return url
	}
	urlFlag, _ := cmd.Flags().GetString("url")
	return urlFlag
}

// printConfigHelp explains how to configure the wiki connection when no URL
// is set.
func printConfigHelp() {
	fmt.Println("No wiki configured.")
	fmt.Println()
	fmt.Println("Set the MEDIAWIKI_URL environment variable:")
	fmt.Println("  export MEDIAWIKI_URL=\"https://your-wiki.com/api.php\"")
	fmt.Println()
	fmt.Println("Optional (for editing):")
	fmt.Println("  export MEDIAWIKI_USERNAME=\"User@BotName\"")
	fmt.Println("  export MEDIAWIKI_PASSWORD=\"your-bot-password\"")
}

// printWikiInfo connects to the wiki and prints server name, generator,
// language, and statistics.
func printWikiInfo(cmd *cobra.Command) error {
	fmt.Println()
	client, err := newWikiClient(cmd)
	if err != nil {
		return fmt.Errorf("cannot connect: %w", err)
	}
	defer client.Close()

	info, err := client.GetWikiInfo(context.Background(), wiki.WikiInfoArgs{})
	if err != nil {
		return fmt.Errorf("failed to get wiki info: %w", err)
	}

	fmt.Printf("Wiki name: %s\n", info.SiteName)
	fmt.Printf("Generator: %s\n", info.Generator)
	fmt.Printf("Language:  %s\n", info.Language)
	if info.Statistics != nil {
		fmt.Printf("Pages:     %d\n", info.Statistics.Pages)
		fmt.Printf("Articles:  %d\n", info.Statistics.Articles)
		fmt.Printf("Edits:     %d\n", info.Statistics.Edits)
		fmt.Printf("Users:     %d\n", info.Statistics.Users)
	}

	return nil
}
