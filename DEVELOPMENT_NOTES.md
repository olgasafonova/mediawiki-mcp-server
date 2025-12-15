# Development Notes - MediaWiki MCP Server

## Session: 2025-12-15 - Terminology Checking Tool

### What Was Implemented

Added `mediawiki_check_terminology` tool that scans wiki pages for brand/product naming inconsistencies using a glossary hosted on the wiki itself.

### Files Changed

| File | Changes |
|------|---------|
| `wiki/types.go` | Added 6 new types: `CheckTerminologyArgs`, `CheckTerminologyResult`, `PageTerminologyResult`, `TerminologyIssue`, `GlossaryTerm` |
| `wiki/methods.go` | Added ~240 lines: `CheckTerminology()`, `loadGlossary()`, `parseWikiTableGlossary()`, `parseTableRow()`, `checkPageTerminology()`, `extractContext()` |
| `main.go` | Registered new tool, bumped version to 1.2.0, updated instructions |
| `README.md` | Added Content Quality Tools section, documented glossary format |

### Glossary Page Created

**Page:** `Brand Terminology Glossary` on https://wiki.software-innovation.com

**Format:** Wiki tables with class `wikitable` or `mcp-glossary`

```
{| class="wikitable sortable mcp-glossary"
! Incorrect !! Correct !! regex pattern !! Notes
|-
| 360 || 360° || \b360\b(?!°) || Always use degree symbol
|}
```

**Current terms defined:**
- 10 product name rules (360°, Public 360°, Business 360°, Plan & Build 360°, etc.)
- 5 company name rules (SI, Software Innovation, Tietoevry → Tieto)
- 5 technical term rules (on-premises, SIF, BIF)

### How the Tool Works

1. Fetches glossary page from wiki (default: "Brand Terminology Glossary")
2. Parses wiki tables to extract term definitions
3. For each page to check:
   - Fetches page content
   - Scans each line against glossary terms
   - Uses regex pattern if provided, otherwise literal match
   - Returns issues with line number and context
4. Aggregates results across all pages

### Usage Examples

```
# Check specific pages
mediawiki_check_terminology(pages=["Installation Guide", "Release Notes"])

# Check entire category
mediawiki_check_terminology(category="Cloud Documentation", limit=20)

# Use custom glossary page
mediawiki_check_terminology(pages=["My Page"], glossary_page="Technical Writing Style Guide")
```

### Tool Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `pages` | []string | Page titles to check (optional if category provided) |
| `category` | string | Category to get pages from (optional if pages provided) |
| `glossary_page` | string | Wiki page with glossary (default: "Brand Terminology Glossary") |
| `limit` | int | Max pages to check (default 10, max 50) |

---

## Future Improvements

### Suggested Additional Tools

These were discussed but not yet implemented:

#### Authoring & Publishing
- `mediawiki_move_page` - Rename/move pages (preserves history)
- `mediawiki_delete_page` - Delete pages (admin rights)
- `mediawiki_upload_file` - Upload images/files
- `mediawiki_get_page_revisions` - View edit history
- `mediawiki_compare_revisions` - Diff between versions
- `mediawiki_rollback` - Revert to previous version

#### Content Quality
- `mediawiki_spellcheck` - Check spelling (integrate with LanguageTool or aspell)
- `mediawiki_find_broken_internal_links` - Pages linking to non-existent pages
- `mediawiki_find_orphaned_pages` - Pages with no inbound links
- `mediawiki_find_duplicate_content` - Detect similar/duplicate pages

#### Structure & Navigation
- `mediawiki_get_backlinks` - "What links here" functionality
- `mediawiki_list_templates` - List all templates
- `mediawiki_get_template_usage` - See where a template is used
- `mediawiki_get_user_contributions` - Track edits by user

### Terminology Tool Enhancements

- [ ] Add auto-fix mode (create corrected version of page)
- [ ] Support for multiple glossary pages (merge terms)
- [ ] Exclude certain sections (e.g., code blocks, quotes)
- [ ] Cache glossary to avoid re-fetching on each call
- [ ] Add severity levels to terms (error vs warning)
- [ ] Generate report in different formats (markdown, JSON, wiki table)

---

## Build & Test

```bash
cd /Users/olgasafonova/mediawiki-mcp-server
go build -o mediawiki-mcp-server .

# Test terminology checker (requires glossary page to exist)
# Use via Claude Code after rebuilding
```

## Git Status

Changes not yet committed. To commit:

```bash
git add -A
git commit -m "Add terminology checking tool (v1.2.0)

- New tool: mediawiki_check_terminology
- Reads glossary from wiki page with table format
- Supports regex patterns for precise matching
- Checks specific pages or entire categories
- Returns issues with line numbers and context"
git push
```
