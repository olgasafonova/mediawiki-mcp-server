// Nordic Registry MCP Server - Access Nordic company registries via MCP
// Provides tools for searching and validating companies across Norway, Denmark, Finland, and Sweden
package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/olgasafonova/nordic-registry-mcp-server/internal/denmark"
	"github.com/olgasafonova/nordic-registry-mcp-server/internal/finland"
	"github.com/olgasafonova/nordic-registry-mcp-server/internal/norway"
	"github.com/olgasafonova/nordic-registry-mcp-server/tools"
)

const (
	ServerName    = "nordic-registry-mcp-server"
	ServerVersion = "0.1.0"
)

func main() {
	// Parse command-line flags
	verbose := flag.Bool("verbose", false, "Enable verbose debug logging")
	flag.Parse()

	// Configure logging to stderr (stdout is used for MCP protocol)
	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Create country clients
	norwayClient := norway.NewClient(norway.DefaultConfig(), logger)
	finlandClient := finland.NewClient(finland.DefaultConfig(), logger)
	denmarkClient := denmark.NewClient(denmark.DefaultConfig(), logger)

	// Create MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Version: ServerVersion,
	}, &mcp.ServerOptions{
		Logger:       logger,
		Instructions: serverInstructions,
	})

	// Register tools
	toolRegistry := tools.NewRegistry(norwayClient, finlandClient, denmarkClient, logger)
	toolRegistry.RegisterAll(server)

	logger.Info("Starting Nordic Registry MCP Server",
		"name", ServerName,
		"version", ServerVersion,
		"verbose", *verbose,
	)

	// Run server with stdio transport
	ctx := context.Background()
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

const serverInstructions = `# Nordic Registry MCP Server

Access company information from Nordic business registries:
- Norway (BRREG - Brønnøysundregistrene)
- Finland (PRH - Patent and Registration Office)
- Denmark (CVR - Central Business Register)
- Sweden (Bolagsverket - coming soon)

## Quick Reference

### Universal Tools (auto-detect country)
- nordic_search: Search companies by name across registries
- nordic_get_company: Get company details by org number
- nordic_get_status: Check if company is active/dissolved/bankrupt
- nordic_get_board: Get board of directors (Norway only for now)
- nordic_validate_org_number: Validate org number format

### Country-Specific Tools
- norway_get_enhet: Get Norwegian entity from BRREG
- norway_search: Search Norwegian entities
- finland_get_company: Get Finnish company from PRH
- finland_search: Search Finnish companies
- denmark_get_company: Get Danish company from CVR
- denmark_search: Search Danish companies

## Organization Number Formats

| Country | Format | Example |
|---------|--------|---------|
| Norway | 9 digits | 923 609 016 |
| Denmark | 8 digits | 25 31 37 63 |
| Finland | 7+1 digits | 0112038-9 |
| Sweden | 10 digits | 556036-0793 |

## Example Queries

"Search for Equinor" → Use nordic_search with query="Equinor"
"Get company 923609016" → Use nordic_get_company with org_number="923609016"
"Is 923609016 valid?" → Use nordic_validate_org_number with org_number="923609016"
"Board of 923609016" → Use nordic_get_board with org_number="923609016"
"Search for Novo Nordisk" → Use denmark_search with name="Novo Nordisk"

## Tips

1. Organization numbers are auto-detected by format when possible
2. For 8-digit numbers (Denmark/Finland), specify country explicitly
3. Use nordic_search for general searches, country-specific tools for advanced queries
`
