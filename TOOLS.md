# Tool Reference

The MCP server registers 43 tools across nine categories. When running in HTTP mode, `curl http://localhost:8080/tools` returns the same list as a JSON tree.

## Read Operations

| Tool | Description |
|------|-------------|
| `mediawiki_search` | Full-text search |
| `mediawiki_get_page` | Get page content |
| `mediawiki_get_sections` | Get section structure or specific section content |
| `mediawiki_get_related` | Find related pages via categories/links |
| `mediawiki_get_images` | Get images used on a page |
| `mediawiki_list_pages` | List all pages |
| `mediawiki_list_categories` | List categories |
| `mediawiki_get_category_members` | Get pages in category |
| `mediawiki_get_page_info` | Get page metadata |
| `mediawiki_get_wiki_info` | Wiki statistics |
| `mediawiki_list_users` | List users by group |
| `mediawiki_parse` | Preview wikitext |
| `mediawiki_get_page_summary` | Lead section + metadata without full page load |
| `mediawiki_batch_get_pages` | Fetch multiple page contents in one API call |
| `mediawiki_batch_get_pages_info` | Get metadata for multiple pages at once |
| `mediawiki_search_and_read` | Search + read top results in one call |

**search_and_read** eliminates the most common two-call pattern (search then read):

```
"What does the wiki say about deployment?"
→ Searches, reads top result, returns content + remaining hits as summaries
```

**get_page_summary** is a quick overview without loading full page content:

```
"What is the API Reference page about?"
→ Returns intro section, categories, section list, and metadata
```

**batch_get_pages** fetches up to 50 pages in one call:

```
"Get the content of Main Page, FAQ, and Getting Started"
→ Returns all three pages in a single API request
```

## Link Analysis

| Tool | Description |
|------|-------------|
| `mediawiki_get_external_links` | Get external URLs from page |
| `mediawiki_get_external_links_batch` | Get URLs from multiple pages |
| `mediawiki_check_links` | Check if URLs work |
| `mediawiki_find_broken_internal_links` | Find broken wiki links |
| `mediawiki_get_backlinks` | "What links here" |

## Content Quality

| Tool | Description |
|------|-------------|
| `mediawiki_check_terminology` | Check naming consistency |
| `mediawiki_check_translations` | Find missing translations |
| `mediawiki_find_orphaned_pages` | Find unlinked pages |
| `mediawiki_audit` | Comprehensive health audit (parallel checks, health score) |
| `mediawiki_get_stale_pages` | Find pages not edited in N days |

**get_stale_pages** is wiki hygiene: find outdated content that needs review.

```
"Find pages not updated in 90 days"
→ Returns stale pages sorted oldest-first with days since last edit
```

## Content Discovery

| Tool | Description |
|------|-------------|
| `mediawiki_find_similar_pages` | Find pages with similar content based on term overlap |
| `mediawiki_compare_topic` | Compare how a topic is described across multiple pages |

**find_similar_pages** identifies related content that should be cross-linked or potential duplicates:

```
"Find pages similar to the API Reference page"
→ Returns similarity scores, common terms, and linking recommendations
```

**compare_topic** detects inconsistencies in documentation (different values, conflicting info):

```
"Compare how 'timeout' is described across all pages"
→ Returns page mentions with context snippets and value mismatches
```

## History

| Tool | Description |
|------|-------------|
| `mediawiki_get_revisions` | Page edit history |
| `mediawiki_compare_revisions` | Diff between versions |
| `mediawiki_get_user_contributions` | User's edit history |
| `mediawiki_get_recent_changes` | Recent wiki activity with aggregation |

Aggregation: use `aggregate_by` parameter to get compact summaries.

- `aggregate_by: "user"` → Most active editors
- `aggregate_by: "page"` → Most edited pages
- `aggregate_by: "type"` → Change type distribution (edit, new, log)

## Quick Edit Tools

| Tool | Description |
|------|-------------|
| `mediawiki_find_replace` | Find and replace text |
| `mediawiki_apply_formatting` | Apply bold, italic, strikethrough |
| `mediawiki_bulk_replace` | Replace across multiple pages |
| `mediawiki_search_in_page` | Search within a page |
| `mediawiki_resolve_title` | Fuzzy title matching |

Edit response info: all edit operations return revision tracking and undo instructions.

```json
{
  "revision": {
    "old_revision": 1234,
    "new_revision": 1235,
    "diff_url": "https://wiki.../index.php?diff=1235&oldid=1234"
  },
  "undo": {
    "instruction": "To undo: use wiki URL or revert to revision 1234",
    "wiki_url": "https://wiki.../index.php?title=...&action=edit&undo=1235"
  }
}
```

## Write Operations

| Tool | Description |
|------|-------------|
| `mediawiki_edit_page` | Create or edit pages |
| `mediawiki_upload_file` | Upload files from URL |
| `mediawiki_move_page` | Move (rename) pages with redirect |
| `mediawiki_manage_categories` | Add/remove categories without full edit |

**move_page** renames pages properly (don't delete and recreate):

```
"Rename 'Old Guide' to 'Updated Guide'"
→ Moves page, creates redirect, optionally moves talk page
```

**manage_categories** for quick category management:

```
"Add category 'API' and remove category 'Deprecated' from this page"
→ Modifies categories without touching page content
```

## File Search

| Tool | Description |
|------|-------------|
| `mediawiki_search_in_file` | Search text in PDFs and text files |

**Supported formats:** PDF (text-based), TXT, MD, CSV, JSON, XML, HTML.

PDF requires `poppler-utils` installed (see README's PDF Search Setup).

## Markdown Conversion

| Tool | Description |
|------|-------------|
| `mediawiki_convert_markdown` | Convert Markdown text to MediaWiki markup |

**Themes:**

- `tieto` — Tieto brand colors (Hero Blue #021e57 headings, yellow code highlights)
- `neutral` — Clean output without custom colors (default)
- `dark` — Dark mode optimized

**Options:**

- `add_css` — Include CSS styling block for branded appearance
- `reverse_changelog` — Reorder changelog entries newest-first
- `prettify_checks` — Replace plain checkmarks (✓) with emoji (✅)

**Example:**

```
Input:  "# Hello\n**bold** and *italic*"
Output: "= Hello =\n'''bold''' and ''italic''"
```

**Workflow for adding Markdown content to wiki:**

1. Convert: `mediawiki_convert_markdown` → get wikitext
2. Save: `mediawiki_edit_page` → publish to wiki
