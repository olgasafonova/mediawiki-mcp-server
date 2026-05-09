# Architecture

This document describes the architecture of the MediaWiki MCP Server.

## Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        MCP Client                                │
│              (Claude Desktop, Cursor, n8n, etc.)                │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ MCP Protocol (stdio)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                         main.go                                  │
│                    Tool Registration Layer                       │
│         (33 tools registered with the MCP server)               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ Method calls
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                        wiki/ package                             │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│  │ read.go  │ │ write.go │ │search.go │ │ links.go │           │
│  │ GetPage  │ │ EditPage │ │ Search   │ │ Backlinks│           │
│  │ GetInfo  │ │ Upload   │ │ Similar  │ │ Broken   │           │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘           │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│  │history.go│ │quality.go│ │category  │ │ users.go │           │
│  │Revisions │ │Terminology│ │ .go     │ │ ListUsers│           │
│  │ Compare  │ │Translate │ │ Members  │ │          │           │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘           │
│  ┌──────────┐ ┌──────────┐                                      │
│  │security  │ │client.go │ ← Core HTTP, auth, caching           │
│  │  .go     │ │          │                                      │
│  │  SSRF    │ │          │                                      │
│  └──────────┘ └──────────┘                                      │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ HTTP requests
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                     MediaWiki API                                │
│                   (api.php endpoint)                             │
└─────────────────────────────────────────────────────────────────┘
```

## Directory Structure

```
mediawiki-mcp-server/
├── main.go                 # MCP server, tool registration
├── main_test.go            # Integration tests
├── wiki_editing_guidelines.go  # Wiki editing constants
├── wiki/                   # Core library
│   ├── client.go          # HTTP client, auth, caching, helpers
│   ├── config.go          # Configuration management
│   ├── types.go           # All type definitions (Args, Results)
│   ├── errors.go          # Error types and validation
│   ├── read.go            # Page content reads (GetPage, Parse, GetPageSummary)
│   ├── pageinfo.go        # Page metadata + listings
│   ├── sections.go        # Section operations
│   ├── related.go         # Related pages, categories, outgoing links
│   ├── images.go          # Image operations
│   ├── batch.go           # Batch operations + SearchAndRead
│   ├── write.go           # Page editing operations
│   ├── search.go          # Search operations
│   ├── links.go           # Link checking operations
│   ├── history.go         # Revision history operations
│   ├── categories.go      # Category operations
│   ├── quality.go         # Content quality checks (terminology, translations, audit, stale pages)
│   ├── users.go           # User operations
│   ├── security.go        # SSRF protection, input sanitization
│   ├── similarity.go      # Text similarity algorithms
│   ├── pdf.go             # PDF text extraction (via pdftotext)
│   ├── audit.go           # Audit logging for write operations
│   ├── dedup.go           # In-flight request deduplication
│   └── *_test.go          # Unit tests
├── converter/              # Markdown ↔ MediaWiki conversion
│   ├── md_to_wiki.go
│   ├── wiki_to_md.go
│   └── *_test.go
├── cmd/benchmark/          # Performance benchmarking tool
└── .github/workflows/      # CI/CD pipelines
```

## Key Components

### 1. MCP Server (main.go)

The entry point registers 43 tools with the MCP server (42 from `tools/definitions.go` plus `mediawiki_convert_markdown` registered in `main.go`):

| Category | Tools |
|----------|-------|
| Read (page content) | `get_page`, `get_page_summary`, `get_sections`, `get_images`, `get_related`, `parse`, `batch_get_pages`, `search_and_read` |
| Read (metadata + listings) | `get_page_info`, `batch_get_pages_info`, `list_pages`, `list_categories`, `get_category_members`, `list_users`, `get_wiki_info`, `resolve_title` |
| Write | `edit_page`, `upload_file`, `move_page`, `manage_categories` |
| Quick edits | `find_replace`, `apply_formatting`, `bulk_replace` |
| Search | `search`, `search_in_page`, `search_in_file`, `find_similar_pages`, `compare_topic` |
| Links | `get_backlinks`, `get_external_links`, `get_external_links_batch`, `check_links`, `find_broken_internal_links` |
| Quality | `audit`, `check_terminology`, `check_translations`, `find_orphaned_pages`, `get_stale_pages` |
| History | `get_revisions`, `compare_revisions`, `get_recent_changes`, `get_user_contributions` |
| Conversion | `convert_markdown` |

### 2. Wiki Client (wiki/client.go)

Core functionality:

- **Authentication**: Bot login with CSRF token management
- **Rate Limiting**: Configurable requests per second (default: 10)
- **Caching**: In-memory LRU cache with TTL (default: 5 minutes)
- **HTTP Client**: Custom transport with SSRF protection

### 3. Security Layer (wiki/security.go)

SSRF protection for link checking:

- Private IP blocking (RFC 1918, loopback, link-local)
- DNS rebinding prevention (validates IPs at connection time)
- Fail-closed DNS handling (blocks on DNS errors)

### 4. Content Validation (wiki/errors.go)

Validates all content before wiki edits:

- Size limits (2MB max edit size)
- Dangerous pattern detection (JavaScript injection, etc.)
- Unicode normalization (NFC)

## Data Flow

### Read Operation

```
User Request → MCP Tool → wiki.GetPage() → HTTP GET → MediaWiki API
                                ↓
                          Cache Check
                                ↓
                          Parse Response
                                ↓
                          Return PageContent
```

### Write Operation

```
User Request → MCP Tool → wiki.EditPage() → Validate Content
                                ↓
                          Get CSRF Token
                                ↓
                          HTTP POST → MediaWiki API
                                ↓
                          Return EditResult
```

## Configuration

Environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `MEDIAWIKI_URL` | Yes | Wiki API URL (e.g., `https://wiki.example.com/api.php`) |
| `MEDIAWIKI_USERNAME` | No | Bot username for editing |
| `MEDIAWIKI_PASSWORD` | No | Bot password for editing |

## Design Decisions

### Why split methods.go?

The original `methods.go` was 4,235 lines. Split into 9 modules for:
- Easier navigation and code review
- Logical grouping by functionality
- Smaller, focused files (100-1,100 lines each)

### Why custom HTTP client?

The default `http.Client` doesn't protect against SSRF. Our custom client:
- Validates destination IPs before connecting
- Blocks requests to private networks
- Prevents DNS rebinding attacks

### Why in-memory caching?

MediaWiki pages don't change frequently. Caching:
- Reduces API calls by ~70%
- Improves response time
- Respects wiki rate limits

## Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test ./... -coverprofile=coverage.out

# Run specific package
go test ./wiki/...
```

## Future Improvements

See [IMPROVEMENTS.md](IMPROVEMENTS.md) for planned enhancements.
