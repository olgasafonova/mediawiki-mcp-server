package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
	"github.com/spf13/cobra"
)

// newWikiClient creates a wiki.Client from CLI flags and env vars.
// It respects the --url flag (overrides MEDIAWIKI_URL env var).
func newWikiClient(cmd *cobra.Command) (*wiki.Client, error) {
	urlFlag, _ := cmd.Flags().GetString("url")

	// When --url overrides the wiki, clear credentials to avoid
	// sending Tieto creds to Wikipedia (or vice versa).
	if urlFlag != "" {
		_ = os.Setenv("MEDIAWIKI_URL", urlFlag) //nolint:gosec // G104: CLI flag override, failure is non-critical
		_ = os.Unsetenv("MEDIAWIKI_USERNAME")   //nolint:gosec // G104
		_ = os.Unsetenv("MEDIAWIKI_PASSWORD")   //nolint:gosec // G104
	}

	cfg, err := wiki.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}

	// CLI uses a quiet logger; MCP server is chattier
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	client := wiki.NewClient(cfg, logger)

	// Authenticate if credentials are configured.
	// For public wikis (no credentials), skip auth entirely.
	if cfg.HasCredentials() {
		if err := client.EnsureLoggedIn(context.Background()); err != nil {
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
	}

	return client, nil
}

// isJSON checks if --json flag is set.
func isJSON(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

// isQuiet checks if --quiet flag is set.
func isQuiet(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("quiet")
	return v
}
