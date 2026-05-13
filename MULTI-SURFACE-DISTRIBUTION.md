# Multi-Surface Distribution

Research and design decisions for shipping `mediawiki-mcp-server` as more than just an MCP stdio binary. The goal is for the same knowledge layer to power the MCP server, the `wiki` CLI, and a Claude Code plugin installable from a marketplace.

This document is a commitment, not a menu. It picks one bundling pattern, names the surfaces, and lists the open implementation work as separate beads.

## Decision

**Bundle-with-server (Memtrace pattern), not separate-knowledge-package (n8n-as-code pattern).**

The Go module `github.com/olgasafonova/mediawiki-mcp-server` is the knowledge layer. Every surface lives in this repo and is published from the same release pipeline. There is no separate `@mediawiki-mcp/knowledge` artifact and no monorepo of packages.

### Why this pattern, not the n8n-as-code pattern

| Trait | n8n-as-code (npm) | mediawiki-mcp-server (Go) |
|-------|-------------------|---------------------------|
| Distribution unit | npm package consumed by 5 wrappers | Go module + binary releases |
| Cross-surface code sharing | `@n8n-as-code/skills` imported by each surface | `import "wiki"` from `cmd/wiki/`, `tools/`, future packages |
| Daily upstream regeneration | Required (n8n ships 1+ versions/week) | Not required (MediaWiki API is stable) |
| Schema bundle | 537 node schemas + 7,702 templates | Tool descriptions + wikitext converter rules |
| Editor extensions | VS Code + Open VSX | Not in scope |

The n8n-as-code pattern earns its complexity because n8n's API churn forces daily knowledge-base rebuilds and TypeScript schema transformations. MediaWiki has none of that. Adopting a monorepo of npm packages here would be ceremony without payoff.

The Memtrace pattern (ship skills inside the server repo, no separate package) fits the actual problem: stable upstream API, Go-native consumers, one release pipeline. The shared knowledge already exists as the `wiki/` and `converter/` Go packages; no extraction work is needed.

## Knowledge Layer Inventory

The shared layer is already in place. Listed here for explicit reference, not as new work.

| Component | Location | Surfaces that consume it |
|-----------|----------|--------------------------|
| MediaWiki HTTP client + auth | `wiki/client.go` | MCP, CLI, future plugin |
| Read operations | `wiki/read.go`, `wiki/search.go`, `wiki/methods.go` | MCP, CLI |
| Write operations | `wiki/write.go` | MCP, CLI |
| Link analysis | `wiki/links.go` | MCP, CLI |
| Quality checks | `wiki/quality.go`, `wiki/audit.go` | MCP, CLI |
| Similarity detection | `wiki/similarity.go` | MCP, CLI |
| Markdown → wikitext | `converter/converter.go`, `converter/themes.go` | MCP (`mediawiki_convert_markdown`), CLI (`wiki publish`) |
| Categories, history, users | `wiki/categories.go`, `wiki/history.go`, `wiki/users.go` | MCP, CLI |
| Tool descriptions and dispatcher | `tools/definitions.go`, `tools/handlers.go` | MCP only |

No code extraction is needed for surface multiplication. New surfaces import the same Go packages.

## Surface Map

Three surfaces, one repo, one release pipeline. None of these are new builds; one is new packaging.

### Surface 1: MCP server (`mediawiki-mcp-server` binary)

Status: shipped. Transports: stdio + HTTP. Tool count: 40+. Consumed by Claude Desktop, Claude Code, Cursor, ChatGPT, n8n, VS Code, Google ADK, and any MCP-compatible runtime.

### Surface 2: CLI (`wiki` binary)

Status: shipped. Entry point: `cmd/wiki/main.go`. Subcommand surface:

```
wiki search <query>          full-text search
wiki page <title>            read a page
wiki edit <title>            create or edit
wiki replace <find> <repl>   find and replace across pages
wiki lint <page>             terminology + link checks
wiki audit                   wiki health check
wiki recent                  recent changes
wiki history <page>          revision history
wiki diff <page>             compare revisions
wiki links [external|backlinks|broken|orphans|batch]
wiki list [pages|categories|members|users]
wiki publish <file.md>       markdown → wikitext → publish
wiki config                  show or verify configuration
wiki version
```

All commands support `--json` (machine-readable) and `--quiet` (errors only). Shares `wiki/` package with the MCP server, so behaviour is identical across surfaces.

### Surface 3: Claude Code plugin (this bead's new artifact)

Status: stub only after this bead. Installable via `/plugin marketplace add olgasafonova/mediawiki-mcp-server` once skills are written and a release tagged. The plugin packages four things:

| Element | Purpose |
|---------|---------|
| `.claude-plugin/marketplace.json` | Top-level marketplace manifest (one plugin, this one) |
| `.claude-plugin/plugin.json` | Per-plugin manifest with version, author, skills directory |
| `.claude-plugin/skills/` | Markdown skills that shell out to the `wiki` CLI |
| `.claude-plugin/README.md` | Plugin install instructions for users |

Skills shell out to the published `wiki` CLI binary rather than re-implementing logic in markdown. The CLI is the contract between the plugin and the knowledge layer; the plugin never imports Go code directly.

## CLI Surface Skeleton: What Stays, What Changes

The bead's CLI candidate list (`mw publish`, `mw audit-links`, `mw stale-pages`, `mw search`, `mw replace`) was written before the CLI shipped. The current `wiki` surface already covers `publish`, `audit`, `search`, `replace`. Two gaps remain:

| Candidate | Status | Action |
|-----------|--------|--------|
| `wiki stale-pages` | Missing | Add as top-level command (wraps `mediawiki_get_stale_pages`). New bead. |
| `wiki similar <page>` | Missing | Add as top-level command (wraps `mediawiki_find_similar_pages`). New bead. |

The CLI command name `wiki` (not `mw`) is already in shipped releases. Not changing it.

## Claude Code Plugin Skeleton

The plugin manifest format used here is the same one shipped by [EtienneLescot/n8n-as-code](https://github.com/EtienneLescot/n8n-as-code/tree/main/.claude-plugin) and validated against Claude Code's `/plugin marketplace add` flow.

### Files

```
.claude-plugin/
├── marketplace.json     # Top-level: this repo as a single-plugin marketplace
├── plugin.json          # Per-plugin metadata + skills pointer
├── README.md            # Install instructions
└── skills/
    └── wiki-publish/    # Stub skill (placeholder, see Skills Roadmap below)
        └── SKILL.md
```

### marketplace.json (stub committed alongside this doc)

```json
{
  "name": "mediawiki-mcp-marketplace",
  "owner": { "name": "Olga Safonova" },
  "metadata": {
    "description": "MediaWiki knowledge skills for Claude Code, powered by mediawiki-mcp-server and the wiki CLI.",
    "version": "0.1.0"
  },
  "plugins": [
    {
      "name": "mediawiki",
      "source": "./.claude-plugin",
      "description": "Skills for searching, reading, auditing, and publishing to MediaWiki wikis via the wiki CLI.",
      "version": "0.1.0",
      "author": { "name": "Olga Safonova" },
      "repository": "https://github.com/olgasafonova/mediawiki-mcp-server",
      "license": "MIT",
      "category": "knowledge",
      "keywords": ["mediawiki", "wiki", "skill", "claude-code"]
    }
  ]
}
```

### plugin.json (stub committed alongside this doc)

```json
{
  "name": "mediawiki",
  "version": "0.1.0",
  "description": "Claude Code plugin for MediaWiki. Skills shell out to the wiki CLI for search, audit, lint, and publish workflows.",
  "author": { "name": "Olga Safonova" },
  "license": "MIT",
  "homepage": "https://github.com/olgasafonova/mediawiki-mcp-server",
  "repository": "https://github.com/olgasafonova/mediawiki-mcp-server",
  "skills": "./skills/",
  "keywords": ["mediawiki", "wiki", "claude-code", "plugin"]
}
```

## Skills Roadmap (separate beads, not this one)

| Skill | Trigger phrases | CLI calls |
|-------|----------------|-----------|
| `wiki-publish` | "publish docs", "push to wiki", "publish this markdown" | `wiki publish <file>` |
| `wiki-audit` | "wiki health check", "audit the wiki", "check wiki quality" | `wiki audit --json` |
| `wiki-lint` | "lint this page", "terminology check", "check translations" | `wiki lint <page> --json` |
| `wiki-research` | "what does the wiki say", "search wiki for", "look up on wiki" | `wiki search`, `wiki page` chain |

Each skill is a separate implementation bead. This research bead only commits to the manifest format and the directory structure.

## Release Pipeline (no change required for this bead)

Today's release flow (binary publication via GitHub Actions matrix) does not need modification to ship the plugin. The `.claude-plugin/` directory is checked into the repo and discovered by Claude Code via `/plugin marketplace add <owner>/<repo>`. The marketplace URL resolves to the repo's default branch at install time.

When skills are added (future beads), the plugin version in `marketplace.json` and `plugin.json` is bumped manually as part of the release commit. No new CI workflow is needed.

## Open beads after this one

This research bead commits to a pattern and ships stubs. Three implementation beads follow:

1. **Write `wiki-publish` skill.** Markdown skill that calls `wiki publish` for a file path argument. First end-to-end test of the plugin install path.
2. **Add `wiki stale-pages` and `wiki similar` CLI subcommands.** Closes the gap between MCP tool coverage and CLI coverage. Required before any wiki-audit skill ships.
3. **Plugin v0.1.0 release.** Tag, GitHub release, install test via `/plugin marketplace add olgasafonova/mediawiki-mcp-server`. Validates the manifest end-to-end.

## References

- Pattern source: [EtienneLescot/n8n-as-code](https://github.com/EtienneLescot/n8n-as-code) (multi-distribution npm pattern, rejected here)
- Pattern source: [syncable-dev/memtrace-public](https://github.com/syncable-dev/memtrace-public) (`plugins/memtrace-skills/` bundled with server, adopted here)
- Companion skill bundle pattern: [kepano/obsidian-skills](https://github.com/kepano/obsidian-skills) (spec-first markdown bundle, considered, not adopted because this repo already publishes a binary)
- Internal: `~/Documents/remote-v/AI-Knowledge/n8n-as-code - Etienne Lescot.md` (full sift analysis)
- Internal: `~/Documents/remote-v/Career/SoMe-Content/Drafts/Substack - Multi-Surface MCP Distribution.md` (content brief, defer-until first surface ships)
