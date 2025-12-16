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

## Public 360° Wiki Setup (Tietoevry)

If you're connecting to the **Public 360° Wiki** at `wiki.software-innovation.com`, follow these steps:

### Step 1: Get Your Bot Password

1. Go to [Special:BotPasswords](https://wiki.software-innovation.com/wiki/Special:BotPasswords)
2. Log in with your Tietoevry account if prompted
3. Enter a bot name: `wiki-MCP` (or any name you like)
4. Check these permissions:
   - ✅ **Basic rights**
   - ✅ **Edit existing pages** (if you need to edit)
   - ✅ **Create, edit, and move pages** (optional)
5. Click **Create**
6. **Save the generated password** - you won't see it again!

Your username format: `YourEmail#wiki-MCP` (e.g., `john.doe@tietoevry.com#wiki-MCP`)

### Step 2: Download the Server

```bash
git clone https://github.com/olgasafonova/mediawiki-mcp-server.git
cd mediawiki-mcp-server
go build -o mediawiki-mcp-server .
```

Or download the pre-built binary from [GitHub Releases](https://github.com/olgasafonova/mediawiki-mcp-server/releases).

### Step 3: Configure Your AI Tool

<details>
<summary><strong>Claude Code CLI</strong></summary>

```bash
claude mcp add mediawiki /path/to/mediawiki-mcp-server \
  -e MEDIAWIKI_URL="https://wiki.software-innovation.com/api.php" \
  -e MEDIAWIKI_USERNAME="your.email@tietoevry.com#wiki-MCP" \
  -e MEDIAWIKI_PASSWORD="your-bot-password-here"
```

Restart Claude Code or run `claude mcp list` to verify.
</details>

<details>
<summary><strong>Claude Desktop (Mac)</strong></summary>

Edit `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "mediawiki": {
      "command": "/path/to/mediawiki-mcp-server",
      "env": {
        "MEDIAWIKI_URL": "https://wiki.software-innovation.com/api.php",
        "MEDIAWIKI_USERNAME": "your.email@tietoevry.com#wiki-MCP",
        "MEDIAWIKI_PASSWORD": "your-bot-password-here"
      }
    }
  }
}
```

Restart Claude Desktop.
</details>

<details>
<summary><strong>Claude Desktop (Windows)</strong></summary>

Edit `%APPDATA%\Claude\claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "mediawiki": {
      "command": "C:\\path\\to\\mediawiki-mcp-server.exe",
      "env": {
        "MEDIAWIKI_URL": "https://wiki.software-innovation.com/api.php",
        "MEDIAWIKI_USERNAME": "your.email@tietoevry.com#wiki-MCP",
        "MEDIAWIKI_PASSWORD": "your-bot-password-here"
      }
    }
  }
}
```

Restart Claude Desktop.
</details>

<details>
<summary><strong>Cursor</strong></summary>

**Mac:** `~/Library/Application Support/Cursor/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`

**Windows:** `%APPDATA%\Cursor\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json`

```json
{
  "mcpServers": {
    "mediawiki": {
      "command": "/path/to/mediawiki-mcp-server",
      "env": {
        "MEDIAWIKI_URL": "https://wiki.software-innovation.com/api.php",
        "MEDIAWIKI_USERNAME": "your.email@tietoevry.com#wiki-MCP",
        "MEDIAWIKI_PASSWORD": "your-bot-password-here"
      }
    }
  }
}
```

Restart Cursor.
</details>

<details>
<summary><strong>VS Code (with Cline extension)</strong></summary>

1. Install the **Cline** extension from VS Code marketplace
2. Open Cline settings and add MCP server with these environment variables:
   - `MEDIAWIKI_URL`: `https://wiki.software-innovation.com/api.php`
   - `MEDIAWIKI_USERNAME`: `your.email@tietoevry.com#wiki-MCP`
   - `MEDIAWIKI_PASSWORD`: `your-bot-password-here`

Or edit the Cline MCP settings file directly (same location as Cursor).
</details>

<details>
<summary><strong>n8n (via HTTP transport)</strong></summary>

n8n connects via HTTP transport using the **MCP Client Tool** node.

1. **Start the server** with HTTP transport:
   ```bash
   export MEDIAWIKI_URL="https://wiki.software-innovation.com/api.php"
   export MEDIAWIKI_USERNAME="your.email@tietoevry.com#wiki-MCP"
   export MEDIAWIKI_PASSWORD="your-bot-password-here"
   ./mediawiki-mcp-server -http :8080 -token "your-secret-token"
   ```

2. **In n8n**, add an **MCP Client Tool** node and configure:
   - Transport: HTTP Streamable
   - URL: `http://your-server:8080`
   - Authentication: Bearer → your token

See the [HTTP Transport section](#http-transport-chatgpt-n8n-remote-clients) for full setup guide.
</details>

### Step 4: Test It

Ask your AI: *"Search the wiki for release notes"* or *"What categories exist on our wiki?"*

**Example prompts for Public 360° Wiki:**
- *"Find all pages about eFormidling"*
- *"What does the wiki say about AutoSaver?"*
- *"Show me the category structure"*
- *"Check for broken links on the API documentation page"*

---

## Quick Start

Choose your platform:

- [Claude Desktop (Mac)](#claude-desktop-mac)
- [Claude Desktop (Windows)](#claude-desktop-windows)
- [Cursor](#cursor)
- [VS Code](#vs-code)
- [Claude Code CLI](#claude-code-cli)
- [ChatGPT](#chatgpt) ✨ New in v1.9.0
- [n8n](#n8n) ✨ New in v1.9.0

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

### ChatGPT

ChatGPT supports MCP via HTTP transport (requires Pro, Plus, Business, Enterprise, or Education account).

**Step 1: Start the server with HTTP transport**

```bash
# Set your wiki credentials
export MEDIAWIKI_URL="https://your-wiki.example.com/api.php"

# Generate a secure token
export MCP_AUTH_TOKEN=$(openssl rand -hex 32)
echo "Your token: $MCP_AUTH_TOKEN"

# Start the server
./mediawiki-mcp-server -http :8080
```

**Step 2: Configure ChatGPT**

1. Go to **Settings** → **Connectors** → **Advanced** → **Developer Mode**
2. Add a new MCP connector:
   - **URL**: `http://your-server:8080` (must be publicly accessible)
   - **Authentication**: Bearer token → paste your token

**Step 3: Test it**

Ask ChatGPT: *"Search the wiki for release notes"*

For production use with HTTPS, see [Security Best Practices](#security-best-practices).

---

### n8n

n8n connects via HTTP transport using the MCP Client Tool node.

**Step 1: Start the server with HTTP transport**

```bash
export MEDIAWIKI_URL="https://your-wiki.example.com/api.php"
export MCP_AUTH_TOKEN="your-secure-token"
./mediawiki-mcp-server -http :8080
```

**Step 2: Configure n8n**

1. Add an **MCP Client Tool** node to your workflow
2. Configure:
   - **Transport**: HTTP Streamable
   - **URL**: `http://your-server:8080`
   - **Authentication**: Bearer → your token

3. Set environment variable on your n8n instance:
   ```
   N8N_COMMUNITY_PACKAGES_ALLOW_TOOL_USAGE=true
   ```

**Step 3: Use in workflows**

Connect the MCP Client Tool to an AI Agent node to let it search and interact with your wiki.

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

### Users & Administration
| Prompt | What it does |
|--------|--------------|
| *"Who are the wiki admins?"* | Lists users with sysop rights |
| *"Show me all bot accounts"* | Lists users in the bot group |
| *"List bureaucrats on this wiki"* | Lists users who can grant admin rights |

### Quick Edits (requires authentication)
| Prompt | What it does |
|--------|--------------|
| *"Strike out 'John Smith' on the Team page - he left the company"* | Applies strikethrough formatting |
| *"Replace 'Public 360' with 'Public 360°' on the Release Notes page"* | Find and replace text |
| *"Make all occurrences of 'API Gateway' bold on the Architecture page"* | Applies bold formatting |
| *"Replace 'old-domain.com' with 'new-domain.com' across all Documentation pages"* | Bulk replace across multiple pages |

### Full Page Editing (requires authentication)
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

### AI-Guided Tool Selection ✨
The server includes comprehensive instructions that help AI assistants:
- **Decision trees** for choosing the right tool based on user intent
- **Common mistake warnings** to avoid inefficient workflows
- **Example mappings** from natural language to specific tools
- **Wiki editing guidelines** following MediaWiki best practices
- **MCP 2025 security guidelines** for safe, consensual editing

### Quick Edits
Make simple changes without the overhead of fetching, modifying, and pushing entire pages:
- **Find & Replace** - Replace text in a page with one command
- **Apply Formatting** - Add strikethrough, bold, italic, underline, or code formatting
- **Bulk Replace** - Update text across multiple pages or an entire category
- **Search in Page** - Find specific text within a page before editing
- **Fuzzy Title Matching** - Find pages even with typos or case differences

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
| `mediawiki_list_users` | List users by group (admins, bots, etc.) |
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

### Quick Edit Tools
| Tool | Description |
|------|-------------|
| `mediawiki_find_replace` | Find and replace text in a page |
| `mediawiki_apply_formatting` | Apply formatting (strikethrough, bold, italic, underline, code) |
| `mediawiki_bulk_replace` | Replace text across multiple pages or a category |
| `mediawiki_search_in_page` | Search for text within a specific page |
| `mediawiki_resolve_title` | Find page titles with fuzzy matching |

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
| `MCP_AUTH_TOKEN` | No | Bearer token for HTTP transport authentication |

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

## Quick Edit Tools Reference

These tools let you make simple edits without manually fetching and rewriting entire pages.

### Find and Replace (`mediawiki_find_replace`)

Replace specific text in a page:

```
Parameters:
- title: Page title (required)
- find: Text to find (required)
- replace: Replacement text (required)
- all: Replace all occurrences (default: false, replaces only first)
- use_regex: Treat 'find' as regex pattern
- preview: Show changes without saving
- summary: Edit summary for wiki history
```

**Example:** Replace "version 2.0" with "version 3.0" on the Release Notes page.

### Apply Formatting (`mediawiki_apply_formatting`)

Add wiki formatting to specific text:

```
Parameters:
- title: Page title (required)
- text: Text to format (required)
- format: One of: strikethrough, bold, italic, underline, code, nowiki
- all: Apply to all occurrences (default: false)
- preview: Show changes without saving
```

**Supported formats:**
| Format | Wiki syntax | Result |
|--------|-------------|--------|
| `strikethrough` | `<s>text</s>` | ~~text~~ |
| `bold` | `'''text'''` | **text** |
| `italic` | `''text''` | *text* |
| `underline` | `<u>text</u>` | <u>text</u> |
| `code` | `<code>text</code>` | `text` |
| `nowiki` | `<nowiki>text</nowiki>` | Prevents wiki parsing |

**Example:** Strike out "John Smith" on the Team page because he left the company.

### Bulk Replace (`mediawiki_bulk_replace`)

Replace text across multiple pages at once:

```
Parameters:
- find: Text to find (required)
- replace: Replacement text (required)
- pages: List of specific page titles
- category: OR apply to all pages in a category
- use_regex: Treat 'find' as regex pattern
- preview: Show what would change without saving
- limit: Maximum pages to process (default: 50)
```

**Example:** Replace "old-api.example.com" with "api.example.com" across all pages in the Documentation category.

### Search in Page (`mediawiki_search_in_page`)

Find text within a specific page before editing:

```
Parameters:
- title: Page title (required)
- query: Search text (required)
- use_regex: Treat query as regex
- context_lines: Lines of context around matches (default: 2)
```

**Example:** Search for "deprecated" on the API Reference page to see which sections need updating.

### Resolve Title (`mediawiki_resolve_title`)

Find pages with fuzzy matching, helpful when exact title is unknown:

```
Parameters:
- title: Page title to search for (required)
- fuzzy: Enable fuzzy matching (default: true)
- max_results: Maximum results to return (default: 5)
```

**Example:** Find the correct title when you're unsure if it's "Module Overview", "Module overview", or "Modules Overview".

### List Users (`mediawiki_list_users`)

List wiki users, optionally filtered by group:

```
Parameters:
- group: Filter by user group (optional)
- limit: Maximum users to return (default: 50)
- active_only: Only show recently active users
- continue_from: Pagination token for more results
```

**Common groups:**
| Group | Description |
|-------|-------------|
| `sysop` | Administrators |
| `bureaucrat` | Can grant admin rights |
| `bot` | Automated accounts |
| `interface-admin` | Can edit CSS/JS |

**Example:** List all wiki administrators to find who can help with access issues.

---

## HTTP Transport (ChatGPT, n8n, Remote Clients)

Starting with v1.9.0, the server supports **HTTP transport** in addition to stdio. This enables integration with ChatGPT, n8n, and any MCP client that uses HTTP.

### Quick Start (HTTP Mode)

```bash
# Start with HTTP transport on port 8080
./mediawiki-mcp-server -http :8080

# With authentication (recommended for production)
./mediawiki-mcp-server -http :8080 -token "your-secret-token"

# Or use environment variable
export MCP_AUTH_TOKEN="your-secret-token"
./mediawiki-mcp-server -http :8080
```

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-http` | (empty) | HTTP address (e.g., `:8080`, `127.0.0.1:8080`). Empty = stdio mode |
| `-token` | (empty) | Bearer token for authentication. Also reads `MCP_AUTH_TOKEN` env var |
| `-origins` | (empty) | Comma-separated allowed origins for CORS. Empty = allow all |
| `-rate-limit` | 60 | Max requests per minute per IP. 0 = unlimited |

### ChatGPT Configuration

1. **Start the server** with HTTP transport:
   ```bash
   ./mediawiki-mcp-server -http :8080 -token "your-secret-token"
   ```

2. **In ChatGPT**, go to **Settings** → **Connectors** → **Advanced** → **Developer Mode**

3. **Add your MCP server** with:
   - **URL**: `http://your-server:8080` (or your public URL)
   - **Authentication**: Bearer token → enter your secret token

4. **Test it**: Ask ChatGPT *"Search the wiki for release notes"*

**Requirements**: ChatGPT Pro, Plus, Business, Enterprise, or Education account.

### n8n Configuration

n8n can connect via the **MCP Client Tool** node:

1. **Start the server** with HTTP transport:
   ```bash
   ./mediawiki-mcp-server -http :8080 -token "your-secret-token"
   ```

2. **In n8n**, add an **MCP Client Tool** node

3. **Configure connection**:
   - **Transport**: HTTP Streamable
   - **URL**: `http://your-server:8080`
   - **Authentication**: Bearer → your token

4. **Set environment variable** (required for AI agent use):
   ```
   N8N_COMMUNITY_PACKAGES_ALLOW_TOOL_USAGE=true
   ```

### Security Best Practices

The HTTP transport includes built-in security features:

| Feature | Description |
|---------|-------------|
| **Bearer Authentication** | Require token via `Authorization: Bearer <token>` header |
| **Origin Validation** | Restrict which domains can connect (CORS) |
| **Rate Limiting** | Prevent abuse with per-IP request limits |
| **Security Headers** | X-Content-Type-Options, X-Frame-Options, Cache-Control |

**Production Checklist:**

1. ✅ **Always use authentication** in production:
   ```bash
   ./mediawiki-mcp-server -http :8080 -token "$(openssl rand -hex 32)"
   ```

2. ✅ **Use HTTPS** via reverse proxy (nginx, Caddy, etc.):
   ```nginx
   server {
       listen 443 ssl;
       server_name mcp.example.com;

       location / {
           proxy_pass http://127.0.0.1:8080;
           proxy_http_version 1.1;
           proxy_set_header Upgrade $http_upgrade;
           proxy_set_header Connection "upgrade";
           proxy_set_header Host $host;
           proxy_set_header X-Real-IP $remote_addr;
       }
   }
   ```

3. ✅ **Restrict origins** for known clients:
   ```bash
   ./mediawiki-mcp-server -http :8080 -token "secret" \
     -origins "https://chat.openai.com,https://n8n.example.com"
   ```

4. ✅ **Bind to localhost** if using a reverse proxy:
   ```bash
   ./mediawiki-mcp-server -http 127.0.0.1:8080 -token "secret"
   ```

### Transport Comparison

| Aspect | stdio | HTTP |
|--------|-------|------|
| **Use case** | Local apps (Claude Desktop, Cursor) | Remote clients (ChatGPT, n8n) |
| **Performance** | Fastest (no network) | Slight overhead |
| **Security** | OS-level process isolation | Token auth + HTTPS |
| **Concurrency** | Single client | Multiple clients |
| **Setup** | Just run the binary | Need auth token + optional proxy |

**Recommendation**: Use stdio for local development and desktop apps. Use HTTP for remote access, automation, and web-based AI assistants.

---

## Compatibility

| Platform | Transport | Status |
|----------|-----------|--------|
| Claude Desktop (Mac) | stdio | ✅ Fully supported |
| Claude Desktop (Windows) | stdio | ✅ Fully supported |
| Claude Code CLI | stdio | ✅ Fully supported |
| Cursor | stdio | ✅ Fully supported |
| VS Code + Cline | stdio | ✅ Supported |
| VS Code + Continue | stdio | ✅ Supported |
| **ChatGPT** | HTTP | ✅ **Supported (v1.9.0+)** |
| **n8n** | HTTP | ✅ **Supported (v1.9.0+)** |
| Any MCP HTTP client | HTTP | ✅ Supported |

---

## AI Guidance (For Developers)

The server embeds comprehensive instructions via the MCP `Instructions` field. These instructions help AI models:

### Tool Selection
```
User: "Strike out John Smith on the Team page"
AI reads: Decision tree → "Strike out [name]" → mediawiki_apply_formatting
```

### Editing Guidelines
- Content accuracy and conciseness
- Formatting standards (headings, bold, italic, lists)
- Category and page naming conventions
- What NOT to do (don't remove content without asking, don't over-edit)

### MCP 2025 Security
- User consent requirements
- Preview-before-save workflow
- Data privacy guidelines

The guidelines are defined in `wiki_editing_guidelines.go` and can be customized for specific wiki policies.

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
