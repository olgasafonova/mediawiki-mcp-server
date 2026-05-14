# Setup Guide

Configure the MediaWiki MCP server for your AI tool. Pick your tool below.

| I use... | Jump to |
|----------|---------|
| Claude Desktop (Mac/Windows) | [Claude Desktop](#claude-desktop) |
| Claude Code CLI | [Claude Code CLI](#claude-code-cli) |
| Claude Code plugin (skills via marketplace) | [Claude Code plugin](#claude-code-plugin) |
| Cursor | [Cursor](#cursor) |
| ChatGPT | [ChatGPT](#chatgpt) |
| n8n | [n8n](#n8n) |
| VS Code | [VS Code](#vs-code) |
| Google ADK (Go/Python) | [Google ADK](#google-adk) |
| Public 360° wiki (Tieto) | [Public 360° Wiki](#public-360-wiki-tieto) — also see [TIETO_SETUP.md](TIETO_SETUP.md) |

Shell scripts and CI use the `wiki` CLI instead — see [CLI.md](CLI.md).

## Prerequisites

You need the server binary and your wiki's API URL.

**Get the binary:** Download from [Releases](https://github.com/olgasafonova/mediawiki-mcp-server/releases) or build from source:

```bash
git clone https://github.com/olgasafonova/mediawiki-mcp-server.git
cd mediawiki-mcp-server
go build -o mediawiki-mcp-server .
```

Requires Go 1.24+.

**Find your wiki API URL:**

| Wiki type | API URL |
|-----------|---------|
| Standard MediaWiki | `https://your-wiki.com/api.php` |
| Wikipedia | `https://en.wikipedia.org/w/api.php` |
| Fandom | `https://your-wiki.fandom.com/api.php` |

Tip: visit `Special:Version` on your wiki to find the exact endpoint.

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

## Claude Code plugin

Add wiki skills directly to Claude Code via the plugin marketplace:

```
/plugin marketplace add olgasafonova/mediawiki-mcp-server
```

See [.claude-plugin/README.md](.claude-plugin/README.md) for details.

---

## Cursor

Cursor has built-in MCP support. Open **Cursor Settings** > **MCP** and add a new server, or edit the config file directly:

<details open>
<summary><strong>Mac</strong></summary>

1. **Open the config file:**
   ```
   ~/.cursor/mcp.json
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
   %USERPROFILE%\.cursor\mcp.json
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

For production HTTPS setup, see [DEPLOYMENT.md](DEPLOYMENT.md).

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

VS Code has built-in MCP support via Copilot Chat.

1. Open the Command Palette: **Ctrl+Shift+P** (Windows) or **Cmd+Shift+P** (Mac)
2. Type **"MCP: Add Server"** and select it
3. Choose **"Stdio"** as the transport type
4. Enter the path to the binary when prompted
5. Name the server: `mediawiki`

This creates a `.vscode/mcp.json` file. Add the environment variables:

```json
{
  "servers": {
    "mediawiki": {
      "command": "/path/to/mediawiki-mcp-server",
      "env": {
        "MEDIAWIKI_URL": "https://your-wiki.com/api.php"
      }
    }
  }
}
```

On Windows, use the `.exe` path with double backslashes.

Reload VS Code, then use the wiki tools through Copilot Chat.

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

## Editing Wiki Pages

Reading works without login on public wikis. **Private/corporate wikis often require authentication for all operations, including reading.** Editing always requires a bot password.

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

## Public 360° Wiki (Tieto)

**New to this?** The [Tieto Setup Guide](TIETO_SETUP.md) walks through every step assuming no technical knowledge.

<details>
<summary><strong>Quick setup for wiki.software-innovation.com</strong></summary>

### Get Your Bot Password

1. Go to [Special:BotPasswords](https://wiki.software-innovation.com/wiki/Special:BotPasswords)
2. Log in with your Tieto account
3. Create a bot named `wiki-MCP`
4. Enable: **Basic rights** + **Edit existing pages**
5. Save the generated password

Your username: `your.email@tietoevry.com#wiki-MCP`

### Configuration

This wiki requires authentication for all operations (including reading).

**Claude Code CLI:**
```bash
claude mcp add mediawiki /path/to/mediawiki-mcp-server \
  -e MEDIAWIKI_URL="https://wiki.software-innovation.com/api.php" \
  -e MEDIAWIKI_USERNAME="your.email@tietoevry.com#wiki-MCP" \
  -e MEDIAWIKI_PASSWORD="your-bot-password-here"
```

**Claude Desktop / Cursor:**
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

### Example Prompts

- *"Find all pages about eFormidling"*
- *"What does the wiki say about AutoSaver?"*
- *"Check for broken links on the API documentation"*

</details>

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
→ Install poppler-utils. See README's [PDF Search Setup](README.md#pdf-search-setup).
