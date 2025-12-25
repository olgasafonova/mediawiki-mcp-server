# Nordic Registry MCP Server

MCP server for accessing Nordic company registries: BRREG (Norway), PRH (Finland), CVR (Denmark), and Bolagsverket (Sweden).

## Status

| Country | API | Status |
|---------|-----|--------|
| Norway | BRREG | Implemented |
| Finland | PRH | Implemented |
| Denmark | CVR | Planned (Phase 2) |
| Sweden | Bolagsverket | Planned (Phase 2) |

## Quick Start

```bash
# Build
go build -o nordic-registry-mcp-server .

# Run
./nordic-registry-mcp-server
```

## MCP Tools

### Universal Tools
- `nordic_search` - Search companies across registries
- `nordic_get_company` - Get company by org number (auto-detects country)
- `nordic_get_status` - Check if company is active
- `nordic_get_board` - Get board members (Norway only)
- `nordic_validate_org_number` - Validate org number format

### Country-Specific
- `norway_get_enhet` - Raw BRREG data
- `norway_search` - Search Norwegian entities
- `finland_get_company` - Raw PRH data
- `finland_search` - Search Finnish companies

## Org Number Formats

| Country | Format | Example |
|---------|--------|---------|
| Norway | 9 digits | 923609016 |
| Denmark | 8 digits | 25313763 |
| Finland | 7+1 digits | 0112038-9 |
| Sweden | 10 digits | 5560360793 |

## Configuration for Claude Code

Add to your MCP settings:

```json
{
  "mcpServers": {
    "nordic-registry": {
      "command": "/path/to/nordic-registry-mcp-server",
      "args": []
    }
  }
}
```

## Development

```bash
# Run tests
go test ./...

# Build with verbose logging
./nordic-registry-mcp-server -verbose
```

## Project Structure

```
├── main.go                     # Server entry point
├── internal/
│   ├── registry/               # Unified types and validation
│   │   ├── types.go            # Company, Address, etc.
│   │   └── orgnumber.go        # Org number validation
│   ├── norway/                 # BRREG client
│   └── finland/                # PRH client
└── tools/
    ├── definitions.go          # MCP tool specs
    └── handlers.go             # Tool implementations
```

## License

Proprietary - TietoEvry
