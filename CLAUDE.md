# CLAUDE.md

Import @README.md

## Build & Test
```bash
make check              # Lint + tests
go test ./...           # Run tests
go build .              # Build binary
```

## Project Notes
- Nordic business registry integration (Norway, Finland, Sweden, Denmark)
- Each country has different API patterns
- Caching for repeated lookups
