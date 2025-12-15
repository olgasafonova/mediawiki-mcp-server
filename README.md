# MediaWiki MCP Server

Connect your AI assistant to any MediaWiki wiki. Search, read, analyze, and edit wiki content directly from Claude, Cursor, or any MCP-compatible tool.

## What is this?

This tool lets AI assistants like Claude or Cursor interact directly with your wiki. Instead of copying and pasting content, you can simply ask:

- *"What does our wiki say about the onboarding process?"*
- *"Find all pages that mention the API"*
- *"Who edited the Release Notes page last week?"*
- *"Are there any broken links on the Documentation page?"*

The AI reads your wiki directly and gives you accurate, up-to-date answers.

**Works with:** Wikipedia, Fandom, corporate wikis, and any MediaWiki installation.

---

## Quick Start

Choose your platform:

- [Claude Desktop (Mac)](#claude-desktop-mac)
- [Claude Desktop (Windows)](#claude-desktop-windows)
- [Cursor](#cursor)
- [VS Code](#vs-code)
- [Claude Code CLI](#claude-code-cli)

---

### Claude Desktop (Mac)

**Step 1: Download the server**

Download the latest release from [GitHub Releases](https://github.com/olgasafonova/mediawiki-mcp-server/releases):

```bash
# Or build from source:
git clone https://github.com/olgasafonova/mediawiki-mcp-server.git
cd mediawiki-mcp-server
go build -o mediawiki-mcp-server .
```

**Step 2: Configure Claude Desktop**

Open the config file:
```bash
open ~/Library/Application\ Support/Claude/claude_desktop_config.json
```

Add this configuration (replace the paths and URL):

```json
{
  "mcpServers": {
    "mediawiki": {
      "command": "/Users/YOUR_USERNAME/mediawiki-mcp-server/mediawiki-mcp-server",
      "env": {
        "MEDIAWIKI_URL": "https://your-wiki.example.com/api.php"
      }
    }
  }
}
```

**Step 3: Restart Claude Desktop**

Quit and reopen Claude Desktop. You should see the MCP tools available.

---

### Claude Desktop (Windows)

**Step 1: Download the server**

Download `mediawiki-mcp-server-windows.exe` from [GitHub Releases](https://github.com/olgasafonova/mediawiki-mcp-server/releases).

Or build from source:
```powershell
git clone https://github.com/olgasafonova/mediawiki-mcp-server.git
cd mediawiki-mcp-server
go build -o mediawiki-mcp-server.exe .
```

**Step 2: Configure Claude Desktop**

Open the config file at:
```
%APPDATA%\Claude\claude_desktop_config.json
```

Add this configuration:

```json
{
  "mcpServers": {
    "mediawiki": {
      "command": "C:\\Users\\YOUR_USERNAME\\mediawiki-mcp-server\\mediawiki-mcp-server.exe",
      "env": {
        "MEDIAWIKI_URL": "https://your-wiki.example.com/api.php"
      }
    }
  }
}
```

**Step 3: Restart Claude Desktop**

---

### Cursor

**Step 1: Download the server**

```bash
git clone https://github.com/olgasafonova/mediawiki-mcp-server.git
cd mediawiki-mcp-server
go build -o mediawiki-mcp-server .
```

**Step 2: Configure Cursor**

Open Cursor Settings (`Cmd+,` on Mac, `Ctrl+,` on Windows), search for "MCP", and add a new server:

**Or** edit the MCP config file directly:

**Mac:** `~/Library/Application Support/Cursor/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`

**Windows:** `%APPDATA%\Cursor\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json`

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

**Step 3: Restart Cursor**

---

### VS Code

VS Code requires an MCP-compatible extension. Options include:

1. **Cline** - Install from VS Code marketplace, then configure MCP servers in extension settings
2. **Continue** - Similar setup process

The configuration format is the same as Cursor above.

---

### Claude Code CLI

The fastest setup if you have Claude Code installed:

```bash
# Clone and build
git clone https://github.com/olgasafonova/mediawiki-mcp-server.git
cd mediawiki-mcp-server
go build -o mediawiki-mcp-server .

# Add to Claude Code (one command)
claude mcp add mediawiki ./mediawiki-mcp-server \
  -e MEDIAWIKI_URL="https://your-wiki.example.com/api.php"
```

Done! The MCP is now available in your Claude Code sessions.

---

## What Can You Ask?

Once configured, try these prompts in Claude, Cursor, or your AI assistant:

### Finding Information
| Prompt | What it does |
|--------|--------------|
| *"What does our wiki say about deployment?"* | Searches and summarizes relevant pages |
| *"Find all pages mentioning the API"* | Full-text search across the wiki |
| *"Show me the Getting Started guide"* | Retrieves specific page content |
| *"List all pages in the Documentation category"* | Browses category contents |

### Tracking Changes
| Prompt | What it does |
|--------|--------------|
| *"What pages were updated this week?"* | Shows recent changes |
| *"Who edited the Release Notes page?"* | Shows revision history |
| *"What did @john.doe change last month?"* | Shows user's contributions |
| *"Show me the diff between the last two versions of Installation Guide"* | Compares revisions |

### Content Quality
| Prompt | What it does |
|--------|--------------|
| *"Are there any broken links on the Documentation page?"* | Checks external URLs |
| *"Find pages with broken internal links"* | Finds links to non-existent pages |
| *"Which pages have no links pointing to them?"* | Finds orphaned content |
| *"What pages link to the API Reference?"* | Shows backlinks |
| *"Check the Product category for terminology issues"* | Validates consistent naming |

### Editing (requires authentication)
| Prompt | What it does |
|--------|--------------|
| *"Create a new page called 'Meeting Notes' with today's date"* | Creates new page |
| *"Update the FAQ page to add a new question about pricing"* | Edits existing page |
| *"Add a section about troubleshooting to the Installation guide"* | Adds content |

---

## Finding Your Wiki's API URL

Your wiki's API URL is typically:

| Wiki Type | API URL Format |
|-----------|---------------|
| Standard MediaWiki | `https://your-wiki.com/api.php` |
| Wikipedia | `https://en.wikipedia.org/w/api.php` |
| Fandom | `https://your-wiki.fandom.com/api.php` |
| Wiki in subdirectory | `https://example.com/wiki/api.php` |

**To verify:** Visit `Special:Version` on your wiki (e.g., `https://your-wiki.com/wiki/Special:Version`) and look for the API entry point.

---

## Features

### Read & Search
- Full-text search across all wiki pages
- Get page content in wikitext or HTML
- Browse categories and page listings
- View recent changes and activity

### Content Analysis
- **Link Checker** - Find broken external URLs
- **Broken Internal Links** - Find wiki links to non-existent pages
- **Orphaned Pages** - Find pages nobody links to
- **Terminology Checker** - Ensure consistent naming using a wiki glossary
- **Translation Checker** - Find missing translations

### History & Tracking
- View page revision history
- Compare any two revisions (diff)
- Track user contributions

### Editing
- Create and edit pages (requires bot password)
- Preview wikitext rendering

### Production Ready
- Rate limiting prevents API overload
- Automatic retries with backoff
- Graceful error handling

---

## Resources (Direct Access)

MCP Resources let AI access wiki content directly as context:

| Resource URI | Description |
|--------------|-------------|
| `wiki://page/{title}` | Access page content |
| `wiki://category/{name}` | List category members |

Examples:
- `wiki://page/Main_Page`
- `wiki://page/Help%3AEditing` (URL-encode special characters)
- `wiki://category/Documentation`

---

## Authentication (For Editing)

**Reading doesn't require authentication.** For editing, you need a bot password:

### Create a Bot Password

1. Log in to your wiki
2. Go to `Special:BotPasswords`
3. Enter a bot name (e.g., `mcp-assistant`)
4. Select permissions: **Basic rights** + **Edit existing pages**
5. Click **Create** and save the generated password

### Add Credentials

**Claude Desktop/Cursor config:**
```json
{
  "mcpServers": {
    "mediawiki": {
      "command": "/path/to/mediawiki-mcp-server",
      "env": {
        "MEDIAWIKI_URL": "https://your-wiki.example.com/api.php",
        "MEDIAWIKI_USERNAME": "YourUsername@mcp-assistant",
        "MEDIAWIKI_PASSWORD": "your-bot-password-here"
      }
    }
  }
}
```

**Claude Code CLI:**
```bash
claude mcp add mediawiki ./mediawiki-mcp-server \
  -e MEDIAWIKI_URL="https://your-wiki.example.com/api.php" \
  -e MEDIAWIKI_USERNAME="YourUsername@mcp-assistant" \
  -e MEDIAWIKI_PASSWORD="your-bot-password-here"
```

---

## All Available Tools

### Read Operations
| Tool | Description |
|------|-------------|
| `mediawiki_search` | Full-text search |
| `mediawiki_get_page` | Get page content |
| `mediawiki_list_pages` | List all pages |
| `mediawiki_list_categories` | List categories |
| `mediawiki_get_category_members` | Get pages in category |
| `mediawiki_get_page_info` | Get page metadata |
| `mediawiki_get_recent_changes` | Recent activity |
| `mediawiki_get_wiki_info` | Wiki statistics |
| `mediawiki_parse` | Preview wikitext |

### Link Analysis
| Tool | Description |
|------|-------------|
| `mediawiki_get_external_links` | Get external URLs from page |
| `mediawiki_get_external_links_batch` | Get URLs from multiple pages |
| `mediawiki_check_links` | Check if URLs work |
| `mediawiki_find_broken_internal_links` | Find broken wiki links |
| `mediawiki_get_backlinks` | "What links here" |

### Content Quality
| Tool | Description |
|------|-------------|
| `mediawiki_check_terminology` | Check naming consistency |
| `mediawiki_check_translations` | Find missing translations |
| `mediawiki_find_orphaned_pages` | Find unlinked pages |

### History
| Tool | Description |
|------|-------------|
| `mediawiki_get_revisions` | Page edit history |
| `mediawiki_compare_revisions` | Diff between versions |
| `mediawiki_get_user_contributions` | User's edit history |

### Write Operations
| Tool | Description |
|------|-------------|
| `mediawiki_edit_page` | Create or edit pages |

---

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `MEDIAWIKI_URL` | Yes | Wiki API endpoint |
| `MEDIAWIKI_USERNAME` | No | Bot username (`User@BotName`) |
| `MEDIAWIKI_PASSWORD` | No | Bot password |
| `MEDIAWIKI_TIMEOUT` | No | Request timeout (default: `30s`) |

---

## Troubleshooting

**"MEDIAWIKI_URL environment variable is required"**
Check that the URL is set in your config and the path is correct.

**"authentication failed"**
- Verify bot password hasn't expired
- Check username format: `WikiUsername@BotName`
- Ensure bot has required permissions

**"page does not exist"**
Page titles are case-sensitive. Check the exact title on your wiki.

**Tools not appearing**
Restart your AI application after config changes.

---

## Compatibility

| Platform | Status |
|----------|--------|
| Claude Desktop (Mac) | ✅ Fully supported |
| Claude Desktop (Windows) | ✅ Fully supported |
| Claude Code CLI | ✅ Fully supported |
| Cursor | ✅ Fully supported |
| VS Code + Cline | ✅ Supported |
| VS Code + Continue | ✅ Supported |
| ChatGPT | ❌ Not supported (MCP is an Anthropic protocol) |

*If OpenAI adopts MCP in the future, this server would work with ChatGPT automatically.*

---

## Development

### Build from Source

```bash
git clone https://github.com/olgasafonova/mediawiki-mcp-server.git
cd mediawiki-mcp-server
go build -o mediawiki-mcp-server .
```

Requires Go 1.23+

### Project Structure

```
mediawiki-mcp-server/
├── main.go           # MCP server and tool registration
├── wiki/
│   ├── config.go     # Configuration
│   ├── types.go      # Request/response types
│   ├── client.go     # HTTP client
│   └── methods.go    # MediaWiki API operations
└── README.md
```

---

## License

MIT License

## Credits

- Built with [Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk)
- Powered by [MediaWiki API](https://www.mediawiki.org/wiki/API:Main_page)
