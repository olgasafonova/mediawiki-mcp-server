# MediaWiki MCP Server

A Model Context Protocol (MCP) server that enables AI assistants to interact with any MediaWiki-powered wiki. Search, read, and edit wiki content directly from Claude, Cursor, or any MCP-compatible client.

Works with Wikipedia, Fandom wikis, corporate wikis, and any MediaWiki installation.

## Features

- **Search** - Full-text search across wiki pages
- **Read** - Get page content in wikitext or HTML format
- **Browse** - List pages, categories, and recent changes
- **Edit** - Create and modify wiki pages (with bot password authentication)
- **Parse** - Preview wikitext rendering before saving
- **External Links** - Extract all external URLs from pages
- **Link Checker** - Detect broken links across your wiki
- **Terminology Checker** - Ensure consistent brand/product naming using a wiki-hosted glossary

### Production-Ready

- **Rate limiting** - Prevents overwhelming the wiki API with concurrent requests
- **Panic recovery** - Graceful error handling keeps the server running
- **Automatic retries** - Exponential backoff for transient failures

## Requirements

- Go 1.23 or later

## Quick Start

### Installation

```bash
# Clone the repository
git clone https://github.com/olgasafonova/mediawiki-mcp-server.git
cd mediawiki-mcp-server

# Build
go build -o mediawiki-mcp-server .
```

### Configuration

Set the wiki API URL as an environment variable:

```bash
# Linux/macOS
export MEDIAWIKI_URL="https://your-wiki.example.com/api.php"

# Windows (PowerShell)
$env:MEDIAWIKI_URL = "https://your-wiki.example.com/api.php"
```

**Finding your wiki's API URL:**
- Most wikis: `https://your-wiki.com/api.php`
- Wikipedia: `https://en.wikipedia.org/w/api.php`
- Check `Special:Version` on your wiki for the exact path

### Usage with Claude Desktop

Add to your Claude Desktop configuration:

**macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows:** `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "mediawiki": {
      "command": "/path/to/mediawiki-mcp-server",
      "env": {
        "MEDIAWIKI_URL": "https://your-wiki.example.com/api.php"
      }
    }
  }
}
```

### Usage with Claude Code

```bash
claude mcp add mediawiki /path/to/mediawiki-mcp-server \
  -e MEDIAWIKI_URL="https://your-wiki.example.com/api.php"
```

## Available Tools

### Read Operations

| Tool | Description |
|------|-------------|
| `mediawiki_search` | Search for pages by text |
| `mediawiki_get_page` | Get page content (wikitext or HTML) |
| `mediawiki_list_pages` | List all pages with pagination |
| `mediawiki_list_categories` | List all categories |
| `mediawiki_get_category_members` | Get pages in a category |
| `mediawiki_get_page_info` | Get page metadata |
| `mediawiki_get_recent_changes` | Get recent wiki changes |
| `mediawiki_get_wiki_info` | Get wiki information and statistics |
| `mediawiki_parse` | Parse wikitext to HTML |

### Link Analysis Tools

| Tool | Description |
|------|-------------|
| `mediawiki_get_external_links` | Get all external URLs from a single page |
| `mediawiki_get_external_links_batch` | Get external links from multiple pages (max 10) |
| `mediawiki_check_links` | Check if URLs are accessible (max 20 URLs) |

### Content Quality Tools

| Tool | Description |
|------|-------------|
| `mediawiki_check_terminology` | Check pages for terminology inconsistencies using a wiki glossary |

### Write Operations

| Tool | Description |
|------|-------------|
| `mediawiki_edit_page` | Create or edit a page (requires authentication) |

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `MEDIAWIKI_URL` | Yes | Wiki API endpoint (e.g., `https://wiki.example.com/api.php`) |
| `MEDIAWIKI_USERNAME` | No | Bot username for editing (format: `User@BotName`) |
| `MEDIAWIKI_PASSWORD` | No | Bot password for editing |
| `MEDIAWIKI_TIMEOUT` | No | Request timeout (default: `30s`) |
| `MEDIAWIKI_USER_AGENT` | No | Custom User-Agent header |
| `MEDIAWIKI_MAX_RETRIES` | No | Max retry attempts (default: `3`) |

## Authentication

**Read-only operations require no authentication.**

For editing, you need a **bot password**:

### Step 1: Create a Bot Password

1. Log in to your wiki
2. Go to `Special:BotPasswords` (e.g., `https://your-wiki.com/wiki/Special:BotPasswords`)
3. Enter a bot name (e.g., `mcp-server`)
4. Select permissions:
   - **Basic rights** (required)
   - **Edit existing pages** (for editing)
   - **Create, edit, and move pages** (for creating new pages)
5. Click **Create**
6. **Save the generated password** - it won't be shown again

### Step 2: Configure Credentials

The username format is `YourWikiUsername@BotName`:

```bash
# Example: Wiki user "john.doe" with bot "mcp-server"
export MEDIAWIKI_USERNAME="john.doe@mcp-server"
export MEDIAWIKI_PASSWORD="abc123def456ghi789"
```

Or with Claude Code:

```bash
claude mcp add mediawiki /path/to/mediawiki-mcp-server \
  -e MEDIAWIKI_URL="https://your-wiki.example.com/api.php" \
  -e MEDIAWIKI_USERNAME="john.doe@mcp-server" \
  -e MEDIAWIKI_PASSWORD="abc123def456ghi789"
```

## Examples

Once configured, ask your AI assistant:

### Search
> "Search the wiki for installation guide"

### Read
> "Get the content of the Main Page"

### Browse
> "List all categories" or "Show recent changes from the last week"

### Edit (requires authentication)
> "Create a new page called 'Meeting Notes' with today's agenda"

### Find Broken Links
> "Get all external links from the Installation Guide page and check if any are broken"

### Check Terminology
> "Check the Cloud Documentation category for terminology issues"

## Terminology Glossary Setup

The `mediawiki_check_terminology` tool reads term definitions from a wiki page containing a table. By default, it looks for a page called **"Brand Terminology Glossary"**.

### Glossary Page Format

Create a wiki page with one or more tables using this structure:

```wikitext
= Brand Terminology Glossary =

{| class="wikitable sortable mcp-glossary"
! Incorrect !! Correct !! Pattern !! Notes
|-
| 360 || 360° || \b360\b(?!°) || Always use degree symbol
|-
| P360 || Public 360° || \bP360\b || Use full product name
|-
| Public 360 || Public 360° || Public 360(?!°) || Needs degree symbol
|-
| on-premise || on-premises || on-premise\b || Always plural
|}
```

### Column Reference

| Column | Required | Description |
|--------|----------|-------------|
| Incorrect | Yes | The term to flag (what to find) |
| Correct | Yes | The replacement suggestion |
| Pattern | No | Regex pattern for precise matching (otherwise literal match) |
| Notes | No | Explanation shown in the report |

### Usage Examples

```
# Check specific pages
mediawiki_check_terminology(pages=["Installation Guide", "Release Notes"])

# Check an entire category
mediawiki_check_terminology(category="Cloud Documentation", limit=20)

# Use a custom glossary page
mediawiki_check_terminology(pages=["My Page"], glossary_page="Technical Writing Style Guide")
```

## Response Handling

All tools return structured JSON responses with:
- **Pagination**: `has_more` and `continue_from` fields for large result sets
- **Character limits**: Responses truncated at 25,000 characters
- **Truncation indicators**: Clear messages when content is cut off

## Troubleshooting

### "MEDIAWIKI_URL environment variable is required"
Set the `MEDIAWIKI_URL` environment variable to your wiki's API endpoint.

### "authentication failed" or "login failed"
- Verify your bot password hasn't expired
- Check the username format: `WikiUsername@BotName`
- Ensure the bot has the required permissions

### "page does not exist"
The page title is case-sensitive. Check the exact title on the wiki.

### API errors
Some wikis restrict API access. Check if:
- The wiki allows API access
- You need to be logged in to read content
- Rate limiting is in effect (the server retries automatically)

## Development

### Build

```bash
go build -o mediawiki-mcp-server .
```

### Project Structure

```
mediawiki-mcp-server/
├── main.go           # MCP server and tool registration
├── wiki/
│   ├── config.go     # Environment configuration
│   ├── types.go      # Request/response types
│   ├── client.go     # HTTP client with auth
│   └── methods.go    # MediaWiki API operations
└── README.md
```

## License

MIT License

## Credits

- Built with the [Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk)
- Powered by [MediaWiki API](https://www.mediawiki.org/wiki/API:Main_page)
