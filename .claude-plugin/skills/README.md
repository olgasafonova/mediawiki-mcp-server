# Skills

Markdown skills loaded by Claude Code when the plugin is installed. Each skill lives in its own subdirectory with a `SKILL.md` file at the root.

## Layout

```
skills/
├── wiki-publish/
│   └── SKILL.md
├── wiki-audit/
│   └── SKILL.md
├── wiki-lint/
│   └── SKILL.md
└── wiki-research/
    └── SKILL.md
```

Skill bodies follow the [Anthropic Skills format](https://docs.claude.com/en/docs/agents-and-tools/agent-skills): YAML frontmatter (`name`, `description`, `allowed-tools`) followed by markdown body with trigger phrases, steps, and gotchas.

## Skill contract

Every skill in this plugin shells out to the `wiki` CLI. The plugin never re-implements wiki logic in markdown. This keeps the knowledge layer in one place (the Go module) and the skill bodies small.

A typical skill body:
1. Resolve `wiki` CLI path (assume it's on `PATH`; bail out with install instructions if not).
2. Run the CLI subcommand with `--json` for machine-readable output.
3. Parse the JSON and present the relevant fields to the user.
4. Offer the obvious next action.

## Status

Empty. Skills are tracked as separate beads in the `mediawiki-mcp-server` project. See [`../../MULTI-SURFACE-DISTRIBUTION.md`](../../MULTI-SURFACE-DISTRIBUTION.md) for the roadmap.
