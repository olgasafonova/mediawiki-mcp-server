# MediaWiki MCP Server - Remaining Improvements

This document tracks improvements identified during the code review session on 2025-12-19.

## Completed (v1.17.4 - v1.18.0)

| Version | Improvement | Description |
|---------|-------------|-------------|
| v1.17.4 | DNS Rebinding Fix | Added `safeDialer` with connection-time IP validation |
| v1.17.5 | Fail-Closed DNS | DNS errors now block requests instead of allowing |
| v1.17.5 | Structured Error Codes | Added `SSRFError` type with programmatic codes |
| v1.17.6 | Unicode Normalization | NFC normalization for page titles and content validation |
| v1.17.7 | Godoc Comments | Added documentation to all 80+ exported types in `types.go` |
| v1.17.8 | Split methods.go | Refactored 4,235-line file into 9 focused modules |
| v1.18.0 | Audit Logging | JSON-line logging for all write operations (edits, uploads) |

## Remaining Improvements

### Medium Priority (Code Quality)

#### 1. Split methods.go (~4 hours) - ✅ COMPLETED

**Final state:** methods.go reduced from 4,235 → 16 lines (just documentation)

**Completed (2025-12-19):**

| File | Lines | Methods |
|------|-------|---------|
| `security.go` | 158 | SSRF protection: `isPrivateIP`, `isPrivateHost`, `safeDialer`, `linkCheckClient` |
| `search.go` | 590 | `Search`, `SearchInPage`, `SearchInFile`, `FindSimilarPages`, `CompareTopic`, `normalizeValue`, `stripHTMLTags` |
| `categories.go` | 136 | `ListCategories`, `GetCategoryMembers` |
| `history.go` | 380 | `GetRecentChanges`, `GetRevisions`, `CompareRevisions`, `GetUserContributions`, `aggregateChanges` |
| `links.go` | 639 | `GetExternalLinks`, `GetExternalLinksBatch`, `CheckLinks`, `GetBacklinks`, `FindBrokenInternalLinks`, `FindOrphanedPages` |
| `quality.go` | 401 | `CheckTerminology`, `CheckTranslations`, `loadGlossary`, `parseWikiTableGlossary`, `parseTableRow`, `checkPageTerminology`, `extractContext`, `stripCodeBlocksForTerminology` |
| `users.go` | 90 | `ListUsers` |
| `read.go` | 1,128 | `GetPage`, `getPageWikitext`, `getPageHTML`, `GetSections`, `getSectionContent`, `GetPageInfo`, `GetRelated`, `getPageCategories`, `getPageLinks`, `GetImages`, `getImageInfo`, `ListPages`, `getNamespacePageCount`, `GetWikiInfo`, `Parse`, `ResolveTitle`, `calculateSimilarity` |
| `write.go` | 784 | `EditPage`, `FindReplace`, `ApplyFormatting`, `BulkReplace`, `UploadFile`, `uploadFromURL`, `uploadFromFile`, `readLocalFile`, `parseUploadResponse`, `parseJSONResponse`, `buildEditRevisionInfo`, `checkPagesExist`, `getFileURL`, `downloadFile`, `truncateString` |

**Final structure:**
```
wiki/
├── client.go         (873 lines - auth, caching, core HTTP, helper functions)
├── security.go       (158 lines - SSRF protection)
├── search.go         (590 lines - search operations)
├── read.go           (1,128 lines - page reading)
├── write.go          (784 lines - page editing)
├── history.go        (380 lines - revisions/changes)
├── links.go          (639 lines - link operations)
├── categories.go     (136 lines - category operations)
├── quality.go        (401 lines - terminology/translation checks)
├── users.go          (90 lines - user operations)
├── methods.go        (16 lines - just documentation)
├── types.go          (971 lines - unchanged)
├── errors.go         (395 lines - unchanged)
├── similarity.go     (310 lines - unchanged)
├── pdf.go            (188 lines - unchanged)
└── *_test.go         (tests - unchanged)
```

**Benefits achieved:**
- Easier navigation and code review
- Logical grouping by functionality
- Smaller, focused files (~100-1,100 lines each vs 4,235)
- Build and all tests pass

---

#### 2. Tool Registration Abstraction (~6 hours)
**Current state:** 33 repetitive tool registration blocks in `main.go` (~1,100 lines)

**Each registration looks like:**
```go
registerToolHandler(server, &ToolDefinition{
    Name: "mediawiki_search",
    Description: "...",
    // ... 20+ lines of metadata
}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.SearchArgs) (*mcp.CallToolResult, wiki.SearchResult, error) {
    // handler
})
```

**Proposed solution:** Metadata-driven registry
```go
type ToolSpec struct {
    Name        string
    Method      string  // e.g., "Search"
    Description string
    Category    string
    ReadOnly    bool
}

var toolRegistry = []ToolSpec{
    {"mediawiki_search", "Search", "Full-text search", "search", true},
    {"mediawiki_get_page", "GetPage", "Get page content", "read", true},
    // ... all tools defined declaratively
}

func registerAllTools(server *mcp.Server, client *wiki.Client) {
    for _, spec := range toolRegistry {
        registerTool(server, spec, client) // uses reflection
    }
}
```

**Benefits:**
- Reduce main.go by ~800 lines
- Easier to add new tools
- Consistent tool metadata
- Single source of truth

---

#### 3. ~~Add Godoc Comments~~ ✓ COMPLETED
**Status:** Added godoc comments to all 80+ exported types in `types.go`

All exported types now have proper documentation. The 34 Client methods in `methods.go` already had basic godoc comments.

---

#### 4. Bump Test Coverage (~8 hours)
**Current state:**
- `converter/`: 82.2% ✓
- `wiki/`: 26.6% → target 40%
- `main`: 12.7%

**Priority test additions:**
1. Cache eviction under load
2. Concurrent request handling
3. CSRF token refresh on expiry
4. Rate limiter race conditions
5. Content validation edge cases

---

### Low Priority

#### 5. Audit Logging for Writes (~3 hours) - ✅ COMPLETED (v1.18.0)

**Implemented features:**
- `AuditLogger` interface with JSON line output
- `AuditEntry` struct with timestamp, operation type, title, page ID, revision ID, content hash, content size, summary, and success status
- `NewFileAuditLogger` for file-based logging
- `NewWriterAuditLogger` for custom output destinations
- `NullAuditLogger` for disabling audit logging
- Integration into `EditPage` and `UploadFile` operations
- Environment variable `MEDIAWIKI_AUDIT_LOG` to configure log path

**Usage:**
```bash
export MEDIAWIKI_AUDIT_LOG=/var/log/mediawiki-mcp/audit.jsonl
./mediawiki-mcp-server
```

**Example log entry:**
```json
{"timestamp":"2024-01-15T10:30:00Z","operation":"edit","title":"Main Page","page_id":123,"revision_id":456,"content_hash":"abc123...","content_size":1024,"summary":"Fixed typo","minor":true,"wiki_url":"https://wiki.example.com/api.php","success":true}
```

---

## How to Use This Document

1. Pick an improvement from the list
2. Create a branch: `git checkout -b refactor/split-methods`
3. Implement the change
4. Run tests: `go test ./...`
5. Commit and push
6. Create PR or release

## Session Notes

The codebase evaluation found the server to be **production-quality** with:
- Strong security practices (SSRF, XSS, rate limiting)
- Excellent error handling with recovery suggestions
- Minimal dependencies (only MCP SDK)
- Good Go idioms (concurrency, error wrapping)

Main improvement areas are organizational (file splitting) and reducing boilerplate (tool registration).
