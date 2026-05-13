# MediaWiki Plugin for Claude Code

Skills for searching, reading, auditing, and publishing to MediaWiki wikis. Powered by the [`wiki` CLI](../cmd/wiki/) that ships with this repository.

## Status

**Stub.** The plugin manifest is committed. Skills are not yet written. See [`MULTI-SURFACE-DISTRIBUTION.md`](../MULTI-SURFACE-DISTRIBUTION.md) for the design and roadmap.

## Install (once skills land)

```
/plugin marketplace add olgasafonova/mediawiki-mcp-server
/plugin install mediawiki
```

The plugin assumes the `wiki` CLI is installed and on `PATH`. Install it from the latest GitHub release or build from source:

```bash
go install github.com/olgasafonova/mediawiki-mcp-server/cmd/wiki@latest
```

The CLI reads the same environment variables as the MCP server: `MEDIAWIKI_URL`, `MEDIAWIKI_USERNAME`, `MEDIAWIKI_PASSWORD`. See the main [README](../README.md#configure) for setup.

## Planned skills

| Skill | What it does | When it triggers |
|-------|--------------|------------------|
| `wiki-publish` | Convert markdown to wikitext and publish | "publish docs", "push to wiki" |
| `wiki-audit` | Run wiki health audit and interpret findings | "wiki health check", "audit wiki" |
| `wiki-lint` | Terminology and translation checks on a page | "lint this page", "check translations" |
| `wiki-research` | Search, read, and cross-reference workflow | "what does the wiki say", "search wiki for" |

Each skill is tracked as a separate bead in the `mediawiki-mcp-server` project.
