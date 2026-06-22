# MediaWiki MCP Server

Connect your AI assistant to any MediaWiki wiki, or script it directly from the terminal. Search, read, and edit wiki content using natural language or the `wiki` CLI.

[![CI](https://github.com/olgasafonova/mediawiki-mcp-server/actions/workflows/ci.yml/badge.svg)](https://github.com/olgasafonova/mediawiki-mcp-server/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/olgasafonova/mediawiki-mcp-server)](https://goreportcard.com/report/github.com/olgasafonova/mediawiki-mcp-server)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Three ways to use it:**

1. **MCP server** — Claude Desktop, Claude Code, Cursor, ChatGPT, n8n, VS Code, Google ADK, or any MCP-compatible tool. See [SETUP.md](SETUP.md).
2. **`wiki` CLI** — same API client, same auth, no AI needed. For shell pipelines, CI checks, cron jobs. See [CLI.md](CLI.md).
3. **Claude Code plugin** — `/plugin marketplace add olgasafonova/mediawiki-mcp-server` adds wiki skills directly to Claude Code. See [.claude-plugin/README.md](.claude-plugin/README.md).

---

## Documentation

| Document | What it covers |
|----------|----------------|
| [QUICKSTART.md](QUICKSTART.md) | Get running in 2 minutes |
| [SETUP.md](SETUP.md) | Per-tool configuration (Claude Desktop, Cursor, ChatGPT, n8n, VS Code, Google ADK) |
| [CLI.md](CLI.md) | `wiki` command-line reference |
| [TOOLS.md](TOOLS.md) | Full tool reference (43 tools by category) |
| [DEPLOYMENT.md](DEPLOYMENT.md) | HTTP transport, security, endpoints, env vars |
| [TIETO_SETUP.md](TIETO_SETUP.md) | Connect to Tieto's Public 360° Wiki (beginner-friendly) |
| [WIKI_USE_CASES.md](WIKI_USE_CASES.md) | Detailed workflows by persona |
| [ARCHITECTURE.md](ARCHITECTURE.md) | System design |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to contribute |
| [SECURITY.md](SECURITY.md) | Security policies |
| [CHANGELOG.md](CHANGELOG.md) | Version history |

---

## What Can You Do?

The same operation works from a prompt (via MCP) or directly in your shell (via the `wiki` CLI):

| Goal | Prompt your AI | From your terminal |
|------|---------------|--------------------|
| Search the wiki | *"What does our wiki say about onboarding?"* | `wiki search "onboarding"` |
| Read a page | *"Show me the Getting Started page"* | `wiki page "Getting Started"` |
| Find broken links | *"Are there broken links on the Docs page?"* | `wiki links broken --json` |
| Find stale content | *"Which pages haven't been updated in 90 days?"* | `wiki stale-pages --days 90` |
| Cross-link suggestions | *"What pages are similar to the API Reference?"* | `wiki similar "API Reference"` |
| Audit wiki health | *"Run a health check on the wiki"* | `wiki audit --json` |
| Publish markdown | *"Publish this README to the wiki"* | `wiki publish README.md "Page Title"` |
| Strike a name | *"Strike out John Smith on the Team page"* | `wiki replace "Team" --find "John Smith" --replace "<s>John Smith</s>"` |

For the full tool list, see [TOOLS.md](TOOLS.md). The CLI returns typed exit codes for CI-friendly branching — see [CLI.md](CLI.md#exit-codes).

---

## 30-Second Start

1. **Get the binary.** Download from [Releases](https://github.com/olgasafonova/mediawiki-mcp-server/releases) or `go build -o mediawiki-mcp-server .` (requires Go 1.24+).
2. **Find your wiki API URL** — usually `https://your-wiki.com/api.php`. Wikipedia is `https://en.wikipedia.org/w/api.php`. Visit `Special:Version` to confirm.
3. **Wire it up to your AI tool** — see [SETUP.md](SETUP.md) for the configuration that matches your client.

Reading public wikis works without login. Private/corporate wikis and editing require a bot password — [SETUP.md#editing-wiki-pages](SETUP.md#editing-wiki-pages).

---

## Example Prompts

> 📖 **More examples:** see [WIKI_USE_CASES.md](WIKI_USE_CASES.md) for detailed workflows by persona (content editors, documentation managers, developers).

**Search and read:**
- *"What does our wiki say about deployment?"*
- *"Give me a quick overview of the Configuration page"*
- *"Get the content of Main Page, FAQ, and Setup all at once"*

**Track changes:**
- *"What pages were updated this week?"*
- *"Show me the diff between the last two versions"*
- *"Who are the most active editors this month?"*

**Check quality:**
- *"Are there broken links on this page?"*
- *"Find orphaned pages with no links to them"*
- *"Find pages similar to the Installation Guide"*
- *"Find pages not updated in the last 90 days"*

**Page management** (requires auth):
- *"Rename 'Old Guide' to 'Updated Guide'"*
- *"Strike out John Smith on the Team page"*
- *"Replace 'version 2.0' with 'version 3.0' on Release Notes"*

**File uploads and search** (requires auth):
- *"Upload this image from URL to the wiki"*
- *"Search for 'budget' in File:Annual-Report.pdf"*

**Convert Markdown:**
- *"Convert this README to wiki format"*
- *"Convert with Tieto branding and CSS"* (use theme="tieto", add_css=true)

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
| Claude Desktop (Mac/Windows) | stdio | ✅ Supported |
| Claude Code CLI | stdio | ✅ Supported |
| Cursor | stdio | ✅ Supported |
| VS Code | stdio | ✅ Supported |
| ChatGPT | HTTP | ✅ Supported |
| n8n | HTTP | ✅ Supported |
| Google ADK | stdio / HTTP | ✅ Supported |

**Works with any wiki:** Wikipedia, Fandom, corporate wikis, or any MediaWiki installation.

---

## Troubleshooting

Common issues and fixes live in [SETUP.md#troubleshooting](SETUP.md#troubleshooting). For deeper diagnostics, the HTTP server exposes `/health`, `/ready`, and `/status` endpoints — see [DEPLOYMENT.md](DEPLOYMENT.md).

---

## Development

Build, test, and contribute: see [CONTRIBUTING.md](CONTRIBUTING.md). For the system design, package layout, and data flow, see [ARCHITECTURE.md](ARCHITECTURE.md).

Run the full check before pushing:

```bash
go test -race -failfast ./...
golangci-lint run ./...
```

Integration tests run against a real MediaWiki instance via `docker-compose.test.yml`. See [CONTRIBUTING.md](CONTRIBUTING.md) for the workflow.

---

## Like This Project?

If this server saved you time, consider giving it a ⭐ on GitHub. It helps others discover the project.

## More MCP Servers

Check out my other MCP servers:

| Server | Description | Stars |
|--------|-------------|-------|
| [gleif-mcp-server](https://github.com/olgasafonova/gleif-mcp-server) | Access GLEIF LEI database. Look up company identities, verify legal entities. | ![GitHub stars](https://img.shields.io/github/stars/olgasafonova/gleif-mcp-server?style=flat) |
| [miro-mcp-server](https://github.com/olgasafonova/miro-mcp-server) | Control Miro whiteboards with AI. Boards, diagrams, mindmaps, and more. | ![GitHub stars](https://img.shields.io/github/stars/olgasafonova/miro-mcp-server?style=flat) |
| [nordic-registry-mcp-server](https://github.com/olgasafonova/nordic-registry-mcp-server) | Access Nordic business registries. Look up companies across Norway, Denmark, Finland, Sweden. | ![GitHub stars](https://img.shields.io/github/stars/olgasafonova/nordic-registry-mcp-server?style=flat) |
| [productplan-mcp-server](https://github.com/olgasafonova/productplan-mcp-server) | Talk to your ProductPlan roadmaps. Query OKRs, ideas, launches. | ![GitHub stars](https://img.shields.io/github/stars/olgasafonova/productplan-mcp-server?style=flat) |
| [tilbudstrolden-mcp](https://github.com/olgasafonova/tilbudstrolden-mcp) | Nordic grocery deal hunting. Find offers, plan meals, track spending. | ![GitHub stars](https://img.shields.io/github/stars/olgasafonova/tilbudstrolden-mcp?style=flat) |
| [mcp-servercard-go](https://github.com/olgasafonova/mcp-servercard-go) | Go library for SEP-2127 Server Cards. Pre-connect discovery for MCP servers. | ![GitHub stars](https://img.shields.io/github/stars/olgasafonova/mcp-servercard-go?style=flat) |

---

## License

MIT License

## Credits

- Built with [Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk)
- Powered by [MediaWiki API](https://www.mediawiki.org/wiki/API:Main_page)
