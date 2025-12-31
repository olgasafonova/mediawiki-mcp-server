# CLAUDE.md

Import @README.md

## Build & Test
```bash
make check              # Lint + tests
go test ./...           # Run tests
go build .              # Build binary
```

## Project Notes
- MediaWiki API integration for wiki read/write operations
- Markdown → MediaWiki converter included
- Wiki editing guidelines embedded in `wiki_editing_guidelines.go`
- Evals suite in /evals directory
- Supports both authenticated and anonymous access
