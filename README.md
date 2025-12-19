# MediaWiki MCP Server

Connect your AI assistant to any MediaWiki wiki. Search, read, and edit wiki content using natural language.

**Works with:** Claude Desktop, Claude Code, Cursor, ChatGPT, n8n, and any MCP-compatible tool.

---

## What Can You Do?

Once connected, just ask your AI:

| You say... | What happens |
|------------|--------------|
| *"What does our wiki say about onboarding?"* | AI searches and summarizes relevant pages |
| *"Find all pages mentioning the API"* | Full-text search across your wiki |
| *"Who edited the Release Notes last week?"* | Shows revision history |
| *"Are there broken links on the Docs page?"* | Checks all external URLs |
| *"Strike out John Smith on the Team page"* | Applies formatting (requires auth) |
| *"Convert this README to wiki format"* | Transforms Markdown → MediaWiki markup ✨ |

---

## Get Started

### Step 1: Download

**Option A: Download pre-built binary** (easiest)

Go to [Releases](https://github.com/olgasafonova/mediawiki-mcp-server/releases) and download for your platform.

**Option B: Build from source** (requires Go 1.23+)

```bash
git clone https://github.com/olgasafonova/mediawiki-mcp-server.git
cd mediawiki-mcp-server
go build -o mediawiki-mcp-server .
```

### Step 2: Find Your Wiki's API URL

Your wiki's API URL is usually:

| Wiki type | API URL |
|-----------|---------|
| Standard MediaWiki | `https://your-wiki.com/api.php` |
| Wikipedia | `https://en.wikipedia.org/w/api.php` |
| Fandom | `https://your-wiki.fandom.com/api.php` |

**Tip:** Visit `Special:Version` on your wiki to find the exact API endpoint.

### Step 3: Configure Your AI Tool

**Pick your tool:**

| I use... | Jump to setup |
|----------|---------------|
| Claude Desktop (Mac/Windows) | [Setup instructions](#claude-desktop) |
| Claude Code CLI | [Setup instructions](#claude-code-cli) |
| Cursor | [Setup instructions](#cursor) |
| ChatGPT | [Setup instructions](#chatgpt) |
| n8n | [Setup instructions](#n8n) |
| VS Code + Cline | [Setup instructions](#vs-code) |
| Google ADK (Go/Python) | [Setup instructions](#google-adk) |

---

## Claude Desktop

Works on Mac and Windows. No terminal needed after initial setup.

<details open>
<summary><strong>Mac</strong></summary>

1. **Open the config file:**
   ```bash
   open ~/Library/Application\ Support/Claude/claude_desktop_config.json
   ```

   If the file doesn't exist, create it.

2. **Add this configuration** (replace the path and URL):
   ```json
   {
     "mcpServers": {
       "mediawiki": {
         "command": "/path/to/mediawiki-mcp-server",
         "env": {
           "MEDIAWIKI_URL": "https://your-wiki.com/api.php"
         }
       }
     }
   }
   ```

3. **Restart Claude Desktop** (quit and reopen)

4. **Test it:** Ask *"Search the wiki for getting started"*

</details>

<details>
<summary><strong>Windows</strong></summary>

1. **Open the config file:**
   ```
   %APPDATA%\Claude\claude_desktop_config.json
   ```

   If the file doesn't exist, create it.

2. **Add this configuration** (replace the path and URL):
   ```json
   {
     "mcpServers": {
       "mediawiki": {
         "command": "C:\\path\\to\\mediawiki-mcp-server.exe",
         "env": {
           "MEDIAWIKI_URL": "https://your-wiki.com/api.php"
         }
       }
     }
   }
   ```

3. **Restart Claude Desktop** (quit and reopen)

4. **Test it:** Ask *"Search the wiki for getting started"*

</details>

---

## Claude Code CLI

The fastest setup. One command and you're done.

```bash
claude mcp add mediawiki /path/to/mediawiki-mcp-server \
  -e MEDIAWIKI_URL="https://your-wiki.com/api.php"
```

**Test it:** Ask *"Search the wiki for getting started"*

---

## Cursor

<details open>
<summary><strong>Mac</strong></summary>

1. **Open the config file:**
   ```
   ~/Library/Application Support/Cursor/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json
   ```

2. **Add this configuration:**
   ```json
   {
     "mcpServers": {
       "mediawiki": {
         "command": "/path/to/mediawiki-mcp-server",
         "env": {
           "MEDIAWIKI_URL": "https://your-wiki.com/api.php"
         }
       }
     }
   }
   ```

3. **Restart Cursor**

</details>

<details>
<summary><strong>Windows</strong></summary>

1. **Open the config file:**
   ```
   %APPDATA%\Cursor\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json
   ```

2. **Add this configuration:**
   ```json
   {
     "mcpServers": {
       "mediawiki": {
         "command": "C:\\path\\to\\mediawiki-mcp-server.exe",
         "env": {
           "MEDIAWIKI_URL": "https://your-wiki.com/api.php"
         }
       }
     }
   }
   ```

3. **Restart Cursor**

</details>

---

## ChatGPT

ChatGPT connects via HTTP. You need to run the server on a machine ChatGPT can reach.

**Requirements:** ChatGPT Pro, Plus, Business, Enterprise, or Education account.

### Setup

1. **Start the server with HTTP mode:**
   ```bash
   # Set your wiki URL
   export MEDIAWIKI_URL="https://your-wiki.com/api.php"

   # Generate a secure token
   export MCP_AUTH_TOKEN=$(openssl rand -hex 32)
   echo "Save this token: $MCP_AUTH_TOKEN"

   # Start the server
   ./mediawiki-mcp-server -http :8080
   ```

2. **In ChatGPT:**
   - Go to **Settings** → **Connectors** → **Advanced** → **Developer Mode**
   - Add a new MCP connector
   - **URL:** `http://your-server:8080` (must be publicly accessible)
   - **Authentication:** Bearer token → paste your token

3. **Test it:** Ask *"Search the wiki for getting started"*

**For production:** See [Security Best Practices](#security-best-practices) for HTTPS setup.

---

## n8n

n8n connects via HTTP using the MCP Client Tool node.

### Setup

1. **Start the server with HTTP mode:**
   ```bash
   export MEDIAWIKI_URL="https://your-wiki.com/api.php"
   export MCP_AUTH_TOKEN="your-secure-token"
   ./mediawiki-mcp-server -http :8080
   ```

2. **In n8n:**
   - Add an **MCP Client Tool** node
   - **Transport:** HTTP Streamable
   - **URL:** `http://your-server:8080`
   - **Authentication:** Bearer → your token

3. **Enable for AI agents** (add to n8n environment):
   ```
   N8N_COMMUNITY_PACKAGES_ALLOW_TOOL_USAGE=true
   ```

4. Connect the MCP Client Tool to an AI Agent node.

---

## VS Code

Install the **Cline** extension, then configure it the same way as [Cursor](#cursor).

---

## Google ADK

Google's [Agent Development Kit](https://google.github.io/adk-docs/) connects to MCP servers via stdio or Streamable HTTP.

<details open>
<summary><strong>Go (stdio)</strong></summary>

```go
import (
    "os/exec"
    "google.golang.org/adk/tool/mcptoolset"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Create MCP toolset for wiki access
wikiTools, _ := mcptoolset.New(mcptoolset.Config{
    Transport: &mcp.CommandTransport{
        Command: exec.Command("/path/to/mediawiki-mcp-server"),
        Env: []string{
            "MEDIAWIKI_URL=https://your-wiki.com/api.php",
        },
    },
})

// Add to your agent
agent := llmagent.New(llmagent.Config{
    Name:     "wiki-agent",
    Model:    model,
    Toolsets: []tool.Set{wikiTools},
})
```

</details>

<details>
<summary><strong>Go (Streamable HTTP)</strong></summary>

First, start the server in HTTP mode:

```bash
export MEDIAWIKI_URL="https://your-wiki.com/api.php"
./mediawiki-mcp-server -http :8080 -token "your-secret-token"
```

Then connect from your ADK agent:

```go
import (
    "google.golang.org/adk/tool/mcptoolset"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

wikiTools, _ := mcptoolset.New(mcptoolset.Config{
    Transport: mcp.NewStreamableHTTPClientTransport("http://localhost:8080"),
})
```

</details>

<details>
<summary><strong>Python (stdio)</strong></summary>

```python
from google.adk.tools.mcp_tool import MCPToolset, StdioConnectionParams, StdioServerParameters

wiki_tools = MCPToolset(
    connection_params=StdioConnectionParams(
        server_params=StdioServerParameters(
            command="/path/to/mediawiki-mcp-server",
            env={"MEDIAWIKI_URL": "https://your-wiki.com/api.php"},
        )
    )
)
```

</details>

<details>
<summary><strong>Python (Streamable HTTP)</strong></summary>

Start the server in HTTP mode, then:

```python
from google.adk.tools.mcp_tool import MCPToolset, StreamableHTTPConnectionParams

wiki_tools = MCPToolset(
    connection_params=StreamableHTTPConnectionParams(
        url="http://localhost:8080",
        headers={"Authorization": "Bearer your-secret-token"},
    )
)
```

</details>

---

## Need to Edit Wiki Pages?

Reading works without login. **Editing requires a bot password.**

### Create a Bot Password

1. Log in to your wiki
2. Go to `Special:BotPasswords` (e.g., `https://your-wiki.com/wiki/Special:BotPasswords`)
3. Enter a bot name: `mcp-assistant`
4. Check these permissions:
   - ✅ Basic rights
   - ✅ Edit existing pages
5. Click **Create** and **save the password** (you won't see it again)

### Add Credentials to Your Config

**Claude Desktop / Cursor:**
```json
{
  "mcpServers": {
    "mediawiki": {
      "command": "/path/to/mediawiki-mcp-server",
      "env": {
        "MEDIAWIKI_URL": "https://your-wiki.com/api.php",
        "MEDIAWIKI_USERNAME": "YourWikiUsername@mcp-assistant",
        "MEDIAWIKI_PASSWORD": "your-bot-password-here"
      }
    }
  }
}
```

**Claude Code CLI:**
```bash
claude mcp add mediawiki /path/to/mediawiki-mcp-server \
  -e MEDIAWIKI_URL="https://your-wiki.com/api.php" \
  -e MEDIAWIKI_USERNAME="YourWikiUsername@mcp-assistant" \
  -e MEDIAWIKI_PASSWORD="your-bot-password-here"
```

---

## Example Prompts

> 📖 **More examples:** See [WIKI_USE_CASES.md](WIKI_USE_CASES.md) for detailed workflows by persona (content editors, documentation managers, developers).

### Search & Read
- *"What does our wiki say about deployment?"*
- *"Find all pages mentioning the API"*
- *"Show me the Getting Started guide"*
- *"List all pages in the Documentation category"*

### Sections & Related Content ✨
- *"Show me the sections of the Installation Guide"*
- *"Get the 'Troubleshooting' section from the FAQ page"*
- *"Find pages related to the API Reference"*
- *"What images are on the Product Overview page?"*

### Track Changes
- *"What pages were updated this week?"*
- *"Who edited the Release Notes page?"*
- *"Show me the diff between the last two versions"*
- *"Who are the most active editors this month?"* ✨
- *"Which pages get edited most frequently?"* ✨

### Check Quality
- *"Are there broken links on this page?"*
- *"Find orphaned pages with no links to them"*
- *"Check terminology consistency in the Product category"*
- *"Find pages similar to the Installation Guide"* ✨
- *"Compare how 'API version' is documented across pages"* ✨

### Quick Edits (requires auth)
- *"Strike out John Smith on the Team page"*
- *"Replace 'version 2.0' with 'version 3.0' on Release Notes"*
- *"Make 'API Gateway' bold on the Architecture page"*

### File Uploads (requires auth) ✨
- *"Upload this image from URL to the wiki"*
- *"Add the logo from https://example.com/logo.png as Company_Logo.png"*

### File Search ✨
- *"Search for 'budget' in File:Annual-Report.pdf"*
- *"Find mentions of 'API' in the changelog.txt file"*

> **Note:** PDF search requires `poppler-utils` installed. See [PDF Search Setup](#pdf-search-setup).

### Convert Markdown ✨
- *"Convert this README to wiki format"*
- *"Transform my release notes from Markdown to MediaWiki"*
- *"Convert with Tieto branding and CSS"* (use theme="tieto", add_css=true)

**Themes:**
- `tieto` - Tieto brand colors (Hero Blue headings, yellow code highlights)
- `neutral` - Clean output without custom colors (default)
- `dark` - Dark mode optimized

### Find Users
- *"Who are the wiki admins?"*
- *"List all bot accounts"*

---

## Troubleshooting

**"MEDIAWIKI_URL environment variable is required"**
→ Check your config file has the correct path and URL.

**"authentication failed"**
→ Check username format: `WikiUsername@BotName`
→ Verify bot password hasn't expired
→ Ensure bot has required permissions

**"page does not exist"**
→ Page titles are case-sensitive. Check the exact title on your wiki.

**Tools not appearing in Claude/Cursor**
→ Restart the application after config changes.

**ChatGPT can't connect**
→ Ensure your server is publicly accessible (not just localhost)
→ Check the bearer token matches exactly

**"PDF search requires 'pdftotext'"**
→ Install poppler-utils for your platform (see [PDF Search Setup](#pdf-search-setup))

---

## PDF Search Setup

PDF search requires the `pdftotext` tool from poppler-utils. Text file search (TXT, MD, CSV, etc.) works without any dependencies.

| Platform | Install Command |
|----------|-----------------|
| macOS | `brew install poppler` |
| Ubuntu/Debian | `apt install poppler-utils` |
| RHEL/CentOS | `yum install poppler-utils` |
| Windows | `choco install poppler` |

**Windows alternative:** Download binaries from [poppler-windows releases](https://github.com/oschwartz10612/poppler-windows/releases) and add to PATH.

**Verify installation:**
```bash
pdftotext -v
```

---

## Compatibility

| Platform | Transport | Status |
|----------|-----------|--------|
| Claude Desktop (Mac) | stdio | ✅ Supported |
| Claude Desktop (Windows) | stdio | ✅ Supported |
| Claude Code CLI | stdio | ✅ Supported |
| Cursor | stdio | ✅ Supported |
| VS Code + Cline | stdio | ✅ Supported |
| ChatGPT | HTTP | ✅ Supported |
| n8n | HTTP | ✅ Supported |
| Google ADK | stdio / HTTP | ✅ Supported |

**Works with any wiki:** Wikipedia, Fandom, corporate wikis, or any MediaWiki installation.

---

# Advanced Configuration

## HTTP Transport Options

For ChatGPT, n8n, and remote access, the server supports HTTP transport.

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-http` | (empty) | HTTP address (e.g., `:8080`). Empty = stdio mode |
| `-token` | (empty) | Bearer token for authentication |
| `-origins` | (empty) | Allowed CORS origins (comma-separated) |
| `-rate-limit` | 60 | Max requests per minute per IP |

### Examples

```bash
# Basic HTTP server
./mediawiki-mcp-server -http :8080

# With authentication
./mediawiki-mcp-server -http :8080 -token "your-secret-token"

# Restrict to specific origins
./mediawiki-mcp-server -http :8080 -token "secret" \
  -origins "https://chat.openai.com,https://n8n.example.com"

# Bind to localhost only (for use behind reverse proxy)
./mediawiki-mcp-server -http 127.0.0.1:8080 -token "secret"
```

---

## Security Best Practices

When exposing the server over HTTP:

### 1. Always Use Authentication

```bash
./mediawiki-mcp-server -http :8080 -token "$(openssl rand -hex 32)"
```

### 2. Use HTTPS via Reverse Proxy

Example nginx configuration:

```nginx
server {
    listen 443 ssl;
    server_name mcp.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 3. Restrict Origins

```bash
./mediawiki-mcp-server -http :8080 -token "secret" \
  -origins "https://chat.openai.com"
```

### Built-in Security Features

| Feature | Description |
|---------|-------------|
| Bearer Auth | Validates `Authorization: Bearer <token>` header |
| Origin Validation | Blocks requests from unauthorized domains |
| Rate Limiting | 60 requests/minute per IP (configurable) |
| Security Headers | X-Content-Type-Options, X-Frame-Options |

---

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `MEDIAWIKI_URL` | Yes | Wiki API endpoint (e.g., `https://wiki.com/api.php`) |
| `MEDIAWIKI_USERNAME` | No | Bot username (`User@BotName`) |
| `MEDIAWIKI_PASSWORD` | No | Bot password |
| `MEDIAWIKI_TIMEOUT` | No | Request timeout (default: `30s`) |
| `MCP_AUTH_TOKEN` | No | Bearer token for HTTP authentication |

---

## All Available Tools

<details>
<summary><strong>Read Operations</strong></summary>

| Tool | Description |
|------|-------------|
| `mediawiki_search` | Full-text search |
| `mediawiki_get_page` | Get page content |
| `mediawiki_get_sections` | Get section structure or specific section content ✨ |
| `mediawiki_get_related` | Find related pages via categories/links ✨ |
| `mediawiki_get_images` | Get images used on a page ✨ |
| `mediawiki_list_pages` | List all pages |
| `mediawiki_list_categories` | List categories |
| `mediawiki_get_category_members` | Get pages in category |
| `mediawiki_get_page_info` | Get page metadata |
| `mediawiki_get_wiki_info` | Wiki statistics |
| `mediawiki_list_users` | List users by group |
| `mediawiki_parse` | Preview wikitext |

</details>

<details>
<summary><strong>Link Analysis</strong></summary>

| Tool | Description |
|------|-------------|
| `mediawiki_get_external_links` | Get external URLs from page |
| `mediawiki_get_external_links_batch` | Get URLs from multiple pages |
| `mediawiki_check_links` | Check if URLs work |
| `mediawiki_find_broken_internal_links` | Find broken wiki links |
| `mediawiki_get_backlinks` | "What links here" |

</details>

<details>
<summary><strong>Content Quality</strong></summary>

| Tool | Description |
|------|-------------|
| `mediawiki_check_terminology` | Check naming consistency |
| `mediawiki_check_translations` | Find missing translations |
| `mediawiki_find_orphaned_pages` | Find unlinked pages |
| `mediawiki_audit` | Comprehensive health audit (parallel checks, health score) |

</details>

<details>
<summary><strong>Content Discovery ✨</strong></summary>

| Tool | Description |
|------|-------------|
| `mediawiki_find_similar_pages` | Find pages with similar content based on term overlap |
| `mediawiki_compare_topic` | Compare how a topic is described across multiple pages |

**find_similar_pages** - Identifies related content that should be cross-linked or potential duplicates:
```
"Find pages similar to the API Reference page"
→ Returns similarity scores, common terms, and linking recommendations
```

**compare_topic** - Detects inconsistencies in documentation (different values, conflicting info):
```
"Compare how 'timeout' is described across all pages"
→ Returns page mentions with context snippets and value mismatches
```

</details>

<details>
<summary><strong>History</strong></summary>

| Tool | Description |
|------|-------------|
| `mediawiki_get_revisions` | Page edit history |
| `mediawiki_compare_revisions` | Diff between versions |
| `mediawiki_get_user_contributions` | User's edit history |
| `mediawiki_get_recent_changes` | Recent wiki activity with aggregation ✨ |

**Aggregation ✨** - Use `aggregate_by` parameter to get compact summaries:
- `aggregate_by: "user"` → Most active editors
- `aggregate_by: "page"` → Most edited pages
- `aggregate_by: "type"` → Change type distribution (edit, new, log)

</details>

<details>
<summary><strong>Quick Edit Tools</strong></summary>

| Tool | Description |
|------|-------------|
| `mediawiki_find_replace` | Find and replace text |
| `mediawiki_apply_formatting` | Apply bold, italic, strikethrough |
| `mediawiki_bulk_replace` | Replace across multiple pages |
| `mediawiki_search_in_page` | Search within a page |
| `mediawiki_resolve_title` | Fuzzy title matching |

**Edit Response Info ✨** - All edit operations return revision tracking and undo instructions:
```json
{
  "revision": {
    "old_revision": 1234,
    "new_revision": 1235,
    "diff_url": "https://wiki.../index.php?diff=1235&oldid=1234"
  },
  "undo": {
    "instruction": "To undo: use wiki URL or revert to revision 1234",
    "wiki_url": "https://wiki.../index.php?title=...&action=edit&undo=1235"
  }
}
```

</details>

<details>
<summary><strong>Write Operations</strong></summary>

| Tool | Description |
|------|-------------|
| `mediawiki_edit_page` | Create or edit pages |
| `mediawiki_upload_file` | Upload files from URL ✨ |

</details>

<details>
<summary><strong>File Search ✨</strong></summary>

| Tool | Description |
|------|-------------|
| `mediawiki_search_in_file` | Search text in PDFs and text files |

**Supported formats:** PDF (text-based), TXT, MD, CSV, JSON, XML, HTML

**PDF requires:** `poppler-utils` installed (see [PDF Search Setup](#pdf-search-setup))

</details>

<details>
<summary><strong>Markdown Conversion ✨</strong></summary>

| Tool | Description |
|------|-------------|
| `mediawiki_convert_markdown` | Convert Markdown text to MediaWiki markup |

**Themes:**
- `tieto` - Tieto brand colors (Hero Blue #021e57 headings, yellow code highlights)
- `neutral` - Clean output without custom colors (default)
- `dark` - Dark mode optimized

**Options:**
- `add_css` - Include CSS styling block for branded appearance
- `reverse_changelog` - Reorder changelog entries newest-first
- `prettify_checks` - Replace plain checkmarks (✓) with emoji (✅)

**Example:**
```
Input:  "# Hello\n**bold** and *italic*"
Output: "= Hello =\n'''bold''' and ''italic''"
```

**Workflow for adding Markdown content to wiki:**
1. Convert: `mediawiki_convert_markdown` → get wikitext
2. Save: `mediawiki_edit_page` → publish to wiki

</details>

---

## Public 360° Wiki (Tieto)

<details>
<summary><strong>Setup for wiki.software-innovation.com</strong></summary>

### Get Your Bot Password

1. Go to [Special:BotPasswords](https://wiki.software-innovation.com/wiki/Special:BotPasswords)
2. Log in with your Tieto account
3. Create a bot named `wiki-MCP`
4. Enable: **Basic rights** + **Edit existing pages**
5. Save the generated password

Your username: `your.email@tietoevry.com#wiki-MCP`

### Configuration

Use this URL in your config:
```
MEDIAWIKI_URL=https://wiki.software-innovation.com/api.php
```

### Example Prompts

- *"Find all pages about eFormidling"*
- *"What does the wiki say about AutoSaver?"*
- *"Check for broken links on the API documentation"*

</details>

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
├── main.go                    # Server entry point, HTTP transport
├── main_test.go               # Server tests
├── wiki_editing_guidelines.go # AI guidance for editing
│
├── tools/                     # MCP tool definitions
│   ├── definitions.go         # Tool schemas and metadata
│   ├── handlers.go            # Tool request handlers
│   └── registry.go            # Tool registration
│
├── wiki/                      # MediaWiki API client
│   ├── client.go              # HTTP client with auth
│   ├── config.go              # Configuration management
│   ├── types.go               # Request/response types
│   ├── errors.go              # Error handling
│   ├── read.go                # Page reading operations
│   ├── write.go               # Page editing operations
│   ├── search.go              # Search functionality
│   ├── methods.go             # Core API methods
│   ├── history.go             # Revision history
│   ├── categories.go          # Category operations
│   ├── users.go               # User management
│   ├── links.go               # Link analysis
│   ├── quality.go             # Content quality checks
│   ├── audit.go               # Wiki health audits
│   ├── similarity.go          # Content similarity detection
│   ├── pdf.go                 # PDF text extraction
│   ├── security.go            # Input sanitization, SSRF protection
│   └── *_test.go              # Comprehensive test coverage
│
├── converter/                 # Markdown to MediaWiki converter
│   ├── converter.go           # Conversion logic
│   ├── converter_test.go      # Tests
│   └── themes.go              # Theme definitions (tieto, neutral, dark)
│
├── cmd/
│   └── benchmark/             # Performance benchmarking
│       └── main.go
│
├── ARCHITECTURE.md            # System design documentation
├── CONTRIBUTING.md            # Contribution guidelines
├── SECURITY.md                # Security policy
├── WIKI_USE_CASES.md          # Usage examples by persona
└── README.md
```

---

## License

MIT License

## Credits

- Built with [Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk)
- Powered by [MediaWiki API](https://www.mediawiki.org/wiki/API:Main_page)
