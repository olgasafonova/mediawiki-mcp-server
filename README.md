# Nordic Registry MCP Server

MCP server for accessing Nordic company registries: BRREG (Norway), PRH (Finland), and CVR (Denmark).

## Status

| Country | Registry | Status |
|---------|----------|--------|
| 🇳🇴 Norway | BRREG (Brønnøysundregistrene) | ✅ Implemented |
| 🇫🇮 Finland | PRH (Patent and Registration Office) | ✅ Implemented |
| 🇩🇰 Denmark | CVR (Central Business Register) | ✅ Implemented |
| 🇸🇪 Sweden | Bolagsverket | 🔜 Planned |

## Installation

### Option 1: Download Binary (Recommended)

Download the latest release for your platform from [Releases](https://github.com/olgasafonova/nordic-registry-mcp-server/releases/latest):

| Platform | Binary |
|----------|--------|
| macOS Apple Silicon | `nordic-registry-mcp-server-darwin-arm64` |
| macOS Intel | `nordic-registry-mcp-server-darwin-amd64` |
| Linux x64 | `nordic-registry-mcp-server-linux-amd64` |
| Linux ARM64 | `nordic-registry-mcp-server-linux-arm64` |
| Windows x64 | `nordic-registry-mcp-server-windows-amd64.exe` |

```bash
# macOS/Linux: Make executable
chmod +x nordic-registry-mcp-server-*

# Verify it works
./nordic-registry-mcp-server-darwin-arm64 --help
```

### Option 2: Build from Source

```bash
# Clone
git clone https://github.com/olgasafonova/nordic-registry-mcp-server.git
cd nordic-registry-mcp-server

# Build
go build -o nordic-registry-mcp-server .

# Run
./nordic-registry-mcp-server
```

### Option 3: Docker

```bash
docker build -t nordic-registry-mcp-server .
docker run -i nordic-registry-mcp-server
```

## Configuration

### Claude Code / Claude Desktop

Add to your MCP settings (`~/.claude/settings.json` or Claude Desktop config):

```json
{
  "mcpServers": {
    "nordic-registry": {
      "command": "/path/to/nordic-registry-mcp-server"
    }
  }
}
```

### With Verbose Logging

```json
{
  "mcpServers": {
    "nordic-registry": {
      "command": "/path/to/nordic-registry-mcp-server",
      "args": ["-verbose"]
    }
  }
}
```

## MCP Tools

### Universal Tools (Auto-detect Country)

| Tool | Description |
|------|-------------|
| `nordic_search` | Search companies by name across registries |
| `nordic_get_company` | Get company details by org number |
| `nordic_batch_lookup` | Look up multiple companies at once |
| `nordic_get_status` | Check if company is active/dissolved |
| `nordic_get_board` | Get board of directors (Norway only) |
| `nordic_validate_org_number` | Validate org number format and checksum |

### Country-Specific Tools

| Tool | Description |
|------|-------------|
| `norway_get_enhet` | Get raw BRREG entity data |
| `norway_search` | Search Norwegian entities |
| `finland_get_company` | Get raw PRH company data |
| `finland_search` | Search Finnish companies |
| `denmark_get_company` | Get raw CVR company data |
| `denmark_search` | Search Danish companies |

## Organization Number Formats

| Country | Format | Example | Auto-detected |
|---------|--------|---------|---------------|
| Norway | 9 digits | `923609016` | ✅ Yes |
| Finland | 7+1 with hyphen | `0112038-9` | ✅ Yes |
| Denmark | 8 digits | `24256790` | ✅ Yes |
| Sweden | 10 digits | `5560360793` | ✅ Yes |

## Example Usage

```
"Search for Equinor" → nordic_search with query="Equinor"
"Get company 923609016" → nordic_get_company with org_number="923609016"
"Look up Nokia and Novo Nordisk" → nordic_batch_lookup with org_numbers="0112038-9, 24256790"
"Is 923609016 valid?" → nordic_validate_org_number with org_number="923609016"
"Board of Equinor" → nordic_get_board with org_number="923609016"
```

## Development

```bash
# Run tests
go test ./...

# Run benchmarks
go test ./... -bench=. -benchmem

# Build for all platforms
GOOS=darwin GOARCH=arm64 go build -o dist/nordic-registry-mcp-server-darwin-arm64 .
GOOS=linux GOARCH=amd64 go build -o dist/nordic-registry-mcp-server-linux-amd64 .
```

## Project Structure

```
├── main.go                     # Server entry point
├── internal/
│   ├── registry/               # Unified types and validation
│   │   ├── types.go            # Company, Address, IndustryCode
│   │   └── orgnumber.go        # Org number validation (MOD11, Luhn)
│   ├── norway/                 # BRREG client
│   ├── finland/                # PRH client
│   ├── denmark/                # CVR client
│   ├── cache/                  # In-memory caching
│   └── httputil/               # HTTP client utilities
└── tools/
    ├── definitions.go          # MCP tool specifications
    └── handlers.go             # Tool implementations
```

## License

MIT
