# `wiki` CLI

The `wiki` CLI is a command-line companion to the MCP server. It shares the same API client, auth, and configuration. Reach for it when a prompt is overkill: shell pipelines, CI checks, batch edits, cron jobs.

## Install

```bash
git clone https://github.com/olgasafonova/mediawiki-mcp-server.git
cd mediawiki-mcp-server
go build -o ~/go/bin/wiki ./cmd/wiki
```

Requires Go 1.24+.

## Configure

The CLI reads the same environment variables as the MCP server.

```bash
export MEDIAWIKI_URL="https://your-wiki.com/api.php"

# Optional, for write operations
export MEDIAWIKI_USERNAME="User@BotName"
export MEDIAWIKI_PASSWORD="your-bot-password"

wiki config         # verify setup
```

You can also override the URL per-invocation with `--url`.

## Session caching

On auth-required wikis (private/corporate), the CLI caches the login session at `~/.config/wiki/sessions.json` (mode `0600`) so an agent running many `wiki` commands in a row doesn't re-authenticate on every invocation. The cache is keyed by a hash of the wiki API URL, so multiple wikis stay isolated. Sessions expire after 12 hours regardless of cookie expiry; server-side invalidation triggers a transparent re-login.

Set `WIKI_NO_SESSION_CACHE=1` to disable disk caching (CI, ephemeral containers, hosts with no writable home dir). Use `XDG_CONFIG_HOME` to relocate the cache directory.

## Commands

| Command | What it does |
|---------|--------------|
| `wiki search <query>` | Full-text search |
| `wiki page <title>` | Read a page |
| `wiki edit <title>` | Create or edit a page |
| `wiki replace <title> --find X --replace Y` | Find and replace in a page (add `--bulk --pages` or `--bulk --category` for multi-page) |
| `wiki lint <page>` | Check terminology and links (exit code 4 on findings) |
| `wiki audit` | Wiki-wide health check |
| `wiki recent` | Recent changes |
| `wiki history <page>` | Revision history |
| `wiki diff <page>` | Compare revisions |
| `wiki links [external\|backlinks\|broken\|check\|orphaned]` | Link analysis |
| `wiki list [pages\|categories\|members\|users]` | Listing queries |
| `wiki publish <file.md> <page-title>` | Convert Markdown to wikitext and publish (add `--preview` to skip publish) |
| `wiki similar <page>` | Find pages with similar content |
| `wiki stale-pages` | Find pages not edited in N days |
| `wiki resolve <title>` | Resolve inexact title (add `--fuzzy` for suggestions; exit `3` on no match) |
| `wiki move <from> <to>` | Rename a page (leaves a redirect; `--no-redirect` to suppress) |
| `wiki upload <filename>` | Upload from `--file` or `--url` |
| `wiki categories <page>` | Add/remove categories with `--add`/`--remove` (`--preview` to dry-run) |
| `wiki info` | Show wiki installation details and content statistics |
| `wiki grep page <title> <query>` | Find text in a single page (supports `--regex` and `--context`) |
| `wiki grep file <filename> <query>` | Find text in a wiki-hosted PDF or text file (needs `pdftotext` for PDFs) |
| `wiki compare-topic <topic>` | Compare how a topic is described across pages and surface inconsistencies |
| `wiki translations --base ... --languages ...` | Find missing language translations for a set of base pages |
| `wiki config` | Show or verify configuration |
| `wiki version` | Print CLI version |

Every command supports `--json` (machine-readable output) and `--quiet` (errors only).

## Exit codes

The CLI uses typed exit codes so shell scripts can branch on the failure category.

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Unknown / unclassified error |
| 2 | Usage error (unknown flag, missing required arg) |
| 3 | Not found (HTTP 404 from wiki API) |
| 4 | `wiki lint` found terminology issues or broken links |
| 5 | Wiki API error (other 4xx/5xx) |
| 6 | Auth error (HTTP 401 / 403) |
| 7 | Rate limit (HTTP 429) |
| 10 | Config error |

Note: code `4` is reserved for `wiki lint` findings (existing public API), so auth errors use `6`.

## Examples

```bash
# Pipe lint results into CI
wiki lint "Release Notes" --json > lint.json

# Find broken external links as structured data
wiki links broken --json | jq '.[] | select(.status >= 400)'

# Publish a Markdown file (page title is the second positional arg)
wiki publish docs/onboarding.md "Onboarding"

# Cron-friendly health check
wiki audit --quiet --json > /tmp/wiki-health.json
```

Run `wiki <command> --help` for full flags.
