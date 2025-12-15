# Development Notes - MediaWiki MCP Server

## Session: 2025-12-15 - Content Quality Tools Suite (v1.3.0)

### Summary

Added four new content quality tools based on team feedback:
1. **mediawiki_check_translations** - Find pages missing in specific languages
2. **mediawiki_find_broken_internal_links** - Find internal wiki links pointing to non-existent pages
3. **mediawiki_find_orphaned_pages** - Find pages with no incoming links
4. **mediawiki_get_backlinks** - Get pages linking to a specific page

These tools address real pain points identified by colleagues:
- "Product sheets missing in one language"
- "Implementation guidelines not there"
- "Broken links"
- "Names changed but not consistent across all places"

### Files Changed

| File | Changes |
|------|---------|
| `wiki/types.go` | Added ~90 lines: types for all 4 new tools |
| `wiki/methods.go` | Added ~330 lines: `CheckTranslations()`, `FindBrokenInternalLinks()`, `FindOrphanedPages()`, `GetBacklinks()` |
| `main.go` | Registered 4 new tools, bumped version to 1.3.0, updated instructions |
| `DEVELOPMENT_NOTES.md` | Full documentation |

### Build Status

✅ Binary rebuilt successfully at `/Users/olgasafonova/mediawiki-mcp-server/mediawiki-mcp-server`

**To activate new tools:** Restart Claude Code (the running MCP server uses the old binary until restart)

### Wiki Exploration Results

Explored the wiki categories during testing:

| Category | Pages | Notes |
|----------|-------|-------|
| Clipboard upload | 742 | Uploaded images (via paste), many with generic names like `1.png` |
| Cloud Operations | 183 | Operations documentation |
| 360° | 122 | Main product documentation |
| Cloud Operations How To | 70 | How-to guides |
| 360° Release information | 63 | Release notes |
| Core R&D | 45 | R&D documentation |
| Cloud Documentation | 34 | Cloud-specific docs |

**Finding:** "Clipboard upload" category contains uploaded image files, not content pages. Many have non-descriptive names.

### External Link Check Sample

Tested on Cloud Documentation pages:

| URL | Status |
|-----|--------|
| help.360online.com (PDF) | ✅ 200 OK |
| dev.azure.com | 401 Unauthorized (auth required) |
| tieto-si.visualstudio.com | 401 Unauthorized (auth required) |

Note: Azure DevOps links return 401 because they require authentication, not because they're actually broken.

### Next Session TODO

1. **Restart Claude Code** to activate v1.3.0 tools
2. **Test new tools on real wiki content:**
   - `mediawiki_check_translations` on product documentation
   - `mediawiki_find_broken_internal_links` on Cloud Documentation category
   - `mediawiki_find_orphaned_pages` to find hidden content
   - `mediawiki_get_backlinks` to understand page relationships
3. **Consider additional tools based on findings:**
   - Image audit tool (find poorly named uploads like `1.png`)
   - Required pages checker (verify checklist of pages exist)
   - Duplicate content finder

---

## Tool: mediawiki_check_translations

**Purpose:** Find localized pages that are missing. For example, if you have product sheets that should exist in English, Norwegian, and Swedish, this tool identifies which translations are missing.

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `base_pages` | []string | No* | Base page names to check (without language suffix) |
| `category` | string | No* | Category to get base pages from |
| `languages` | []string | Yes | Language codes to check (e.g., `["en", "no", "sv"]`) |
| `pattern` | string | No | Naming pattern: `subpage` (Page/lang), `suffix` (Page (lang)), or `prefix` (lang:Page). Default: `subpage` |
| `limit` | int | No | Max pages to check (default 20, max 100) |

*Either `base_pages` or `category` must be provided.

### Usage Examples

```
# Check specific product sheets for translations
mediawiki_check_translations(
  base_pages=["Product Sheet 360", "Product Sheet AutoSaver"],
  languages=["en", "no", "sv", "da"]
)

# Check entire category for missing translations
mediawiki_check_translations(
  category="Product Documentation",
  languages=["en", "no"],
  pattern="subpage",
  limit=50
)
```

### Output

```json
{
  "pages_checked": 2,
  "languages_checked": ["en", "no", "sv"],
  "missing_count": 2,
  "pattern": "subpage",
  "pages": [
    {
      "base_page": "Product Sheet 360",
      "translations": {
        "en": {"exists": true, "page_title": "Product Sheet 360/en", "length": 4521},
        "no": {"exists": true, "page_title": "Product Sheet 360/no", "length": 4102},
        "sv": {"exists": false, "page_title": "Product Sheet 360/sv"}
      },
      "missing_languages": ["sv"],
      "complete": false
    }
  ]
}
```

---

## Tool: mediawiki_find_broken_internal_links

**Purpose:** Find internal wiki links (`[[Page Name]]`) that point to pages that don't exist.

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `pages` | []string | No* | Page titles to check |
| `category` | string | No* | Category to get pages from |
| `limit` | int | No | Max pages to check (default 20, max 100) |

*Either `pages` or `category` must be provided.

### Usage Examples

```
# Check specific pages
mediawiki_find_broken_internal_links(
  pages=["Installation Guide", "Release Notes"]
)

# Check all pages in a category
mediawiki_find_broken_internal_links(
  category="Cloud Documentation",
  limit=50
)
```

### Output

```json
{
  "pages_checked": 2,
  "broken_count": 3,
  "pages": [
    {
      "title": "Installation Guide",
      "broken_links": [
        {"target": "Old Configuration Page", "line": 42, "context": "...see [[Old Configuration Page]] for details..."},
        {"target": "Deprecated Feature", "line": 78, "context": "...using [[Deprecated Feature]]..."}
      ],
      "broken_count": 2
    }
  ]
}
```

---

## Tool: mediawiki_find_orphaned_pages

**Purpose:** Find pages with no incoming links ("lonely pages"). These pages are hard to discover through normal wiki navigation.

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `namespace` | int | No | Namespace to check (0=main). Use -1 for all. |
| `limit` | int | No | Max pages to return (default 50, max 200) |
| `prefix` | string | No | Only check pages starting with this prefix |

### Usage Examples

```
# Find orphaned pages in main namespace
mediawiki_find_orphaned_pages(limit=100)

# Find orphaned pages with specific prefix
mediawiki_find_orphaned_pages(prefix="Product", limit=50)
```

### Output

```json
{
  "orphaned_pages": [
    {"title": "Old Product Guide", "page_id": 1234},
    {"title": "Deprecated API Reference", "page_id": 5678}
  ],
  "total_checked": 150,
  "orphaned_count": 2
}
```

---

## Tool: mediawiki_get_backlinks

**Purpose:** Get pages that link to a specific page ("What links here"). Useful for understanding relationships and impact before making changes.

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `title` | string | Yes | Page title to find backlinks for |
| `namespace` | int | No | Filter by namespace (-1 for all, 0 for main) |
| `limit` | int | No | Max backlinks to return (default 50, max 500) |
| `include_redirects` | bool | No | Include redirect pages in results |

### Usage Examples

```
# Find what links to a page
mediawiki_get_backlinks(title="360° Online")

# Find backlinks including redirects
mediawiki_get_backlinks(title="AutoSaver", include_redirects=true, limit=100)
```

### Output

```json
{
  "title": "360° Online",
  "backlinks": [
    {"page_id": 1234, "title": "Product Overview", "namespace": 0},
    {"page_id": 5678, "title": "Installation Guide", "namespace": 0}
  ],
  "count": 2,
  "has_more": false
}
```

---

## Previous Session: Terminology Checking Tool

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
- ~~`mediawiki_find_broken_internal_links`~~ ✅ Implemented in v1.3.0
- ~~`mediawiki_find_orphaned_pages`~~ ✅ Implemented in v1.3.0
- `mediawiki_find_duplicate_content` - Detect similar/duplicate pages
- ~~`mediawiki_check_translations`~~ ✅ Implemented in v1.3.0
- `mediawiki_check_required_pages` - Verify required pages exist (from a checklist)

#### Structure & Navigation
- ~~`mediawiki_get_backlinks`~~ ✅ Implemented in v1.3.0
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
