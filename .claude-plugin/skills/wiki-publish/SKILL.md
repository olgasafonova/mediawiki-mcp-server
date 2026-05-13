---
name: wiki-publish
description: Convert a markdown file to wikitext and publish it to a MediaWiki wiki using the wiki CLI. Use when user says "publish to wiki", "push this to the wiki", "wiki publish", or asks to publish a markdown file as a wiki page.
allowed-tools: Bash Read
---

# wiki-publish

Publishes a markdown file to a MediaWiki wiki by shelling out to the `wiki` CLI. The plugin never re-implements wiki logic; the CLI is the contract.

## Prerequisites

The `wiki` CLI must be on `PATH` and configured.

```bash
wiki version
wiki config
```

If `wiki` is not installed, install from the latest release at https://github.com/olgasafonova/mediawiki-mcp-server/releases or build from source:

```bash
go install github.com/olgasafonova/mediawiki-mcp-server/cmd/wiki@latest
```

If `wiki config` reports missing `MEDIAWIKI_URL`, stop and ask the user to set it (plus `MEDIAWIKI_USERNAME` + `MEDIAWIKI_PASSWORD` for write access). Do not attempt to publish.

## Workflow

1. **Resolve the file path.** If the user gave one, use it. If not, ask.
2. **Resolve the page title.** If the user gave one, use it. Otherwise propose one from the markdown's first H1 heading; confirm before proceeding.
3. **Preview by default for unfamiliar files.** Run `wiki publish <file> "<title>" --preview --json` and show the user the wikitext. Skip the preview only if the user said "publish without preview" or "just push it."
4. **Publish.** Run `wiki publish <file> "<title>" --json` with optional flags:
   - `--summary "<text>"` if the user supplied one
   - `--minor` if the user flagged the change as minor
   - `--theme tieto|neutral|dark` if the user specified a theme
   - `--css` to include the CSS styling block
5. **Report the result.** Parse the JSON response and surface the revision ID and (if a new page was created) the new_page flag.

## CLI contract

```
wiki publish <file.md> <page-title> [flags]
  --summary string   Edit summary (default: "Published from <basename>")
  --minor            Mark as minor edit
  --theme string     Conversion theme: neutral|tieto|dark (default: neutral)
  --css              Include CSS styling block
  --preview          Convert and print wikitext without publishing
  --json             Machine-readable output
```

JSON response on successful publish:

```json
{
  "success": true,
  "title": "Page Title",
  "page_id": 12345,
  "revision_id": 67890,
  "new_page": false,
  "message": "..."
}
```

## Error handling

- **File not found.** Surface the file path and ask the user to correct it.
- **`wiki` CLI not on PATH.** Show the install instructions above; do not proceed.
- **`wiki publish` exits non-zero.** Show stderr verbatim. Common cases: missing credentials (CLI says "authentication failed"), page-protection conflict, network failure.
- **Page title collision with an unrelated page.** If the user did not confirm the page title, stop and confirm before re-running.

## Do not

- Do not call MCP tools directly. The plugin's contract is to use the CLI.
- Do not chain a write after a failed preview. If preview fails, fix the source markdown first.
- Do not retry the publish more than once on a transient error. Report it and let the user decide.
