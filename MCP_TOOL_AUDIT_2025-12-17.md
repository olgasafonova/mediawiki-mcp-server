# MediaWiki MCP Server - Tool Audit Report
**Date**: 2025-12-17
**Tools Tested**: 32/32
**Status**: All tools functional

---

## Bugs Found

| Tool | Issue | Severity | Fix Complexity |
|------|-------|----------|----------------|
| `find_orphaned_pages` | Returns `page_id: 0`, `length: 0` for results | Medium | Medium |
| `convert_markdown` | Missing spaces around bold/italic formatting | Low | Quick |
| `parse` | Includes verbose HTML cache comments in output | Low | Quick |
| `check_terminology` | Flags code paths (e.g., `SI.Data`) as terminology violations | Medium | Medium |

---

## Token Optimization Opportunities

| Tool | Current State | Suggestion | Priority |
|------|---------------|------------|----------|
| `get_page` | Returns full page content (up to 25KB) | Add `max_length` or `summary` mode | High |
| `compare_revisions` | Returns HTML table diff | Add plain-text diff option | Medium |
| `parse` | Includes MediaWiki cache comments | Strip HTML comments | Low |
| `get_category_members` | Verbose base64 continuation token | Shorter tokens | Low |

---

## UX Improvements

| Tool | Current | Suggested | Priority |
|------|---------|-----------|----------|
| `list_pages` | `total_count` shows limit, not actual total | Show actual total matches | Medium |
| `search_in_page` | Multiple matches on same line returned separately | Consolidate same-line matches | Low |
| `check_terminology` | No code exclusion | Add `exclude_code_blocks` option | High |
| `check_translations` | Fixed subpage pattern | Auto-detect wiki's translation pattern | Low |
| `find_replace` | Replaces first match only by default | Make `all` behavior clearer in docs | Low |

---

## Missing Features (Nice to Have)

| Feature | Tool | Description | Priority |
|---------|------|-------------|----------|
| Preview mode | `get_page` | Add `first_n_lines` parameter | Medium |
| Link analysis | `get_related` | Use link structure, not just categories | Low |
| Freshness indicator | `search` | Add `last_modified` to results | Low |
| Unified diff | `compare_revisions` | Add plain text diff format option | Medium |

---

## Performance Winners (No Changes Needed)

These tools are well-designed:
- `get_page_info` - Lightweight metadata without full content
- `get_sections` - Fetch only needed sections (token-efficient)
- `external_links_batch` - Multi-page lookup in one call
- `find_similar_pages` - Actionable linking recommendations
- `resolve_title` - Fuzzy matching with similarity scores

---

## Implementation Priority

### Quick Wins (< 30 min each) ✅ COMPLETED
1. ✅ `convert_markdown` - Fixed spacing around bold/italic (captured boundary characters in regex)
2. ✅ `parse` - Strip HTML cache comments from output (added htmlCommentRegex to sanitizeHTML)

### Medium Priority (1-2 hrs each) ✅ COMPLETED
3. ✅ `check_terminology` - Added `exclude_code_blocks` option (default: true)
4. ✅ `find_orphaned_pages` - Fixed page_id/length by fetching actual page info
5. ✅ `list_pages` - Added `returned_count`, `total_estimate` fields for clarity

### Larger Features (2+ hrs)
6. `get_page` - Add `max_length` or `lines` parameter
7. `compare_revisions` - Add plain text diff format

---

## Test Commands Used

```bash
# All tools were tested with real wiki data
# Server: wiki.software-innovation.com
# MediaWiki version: 1.39.7
```

---

## Next Steps

1. Fix quick wins first (convert_markdown, parse)
2. Address check_terminology false positives
3. Fix find_orphaned_pages bug
4. Consider token optimization for get_page
