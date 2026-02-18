---
name: wiki-operations
description: Search, read, and analyze MediaWiki content. Use for finding information, checking page quality, tracking changes, and exploring wiki structure.
---

# Wiki Operations

## Trigger

Searching, reading, or analyzing content on a MediaWiki wiki.

## Workflow

1. **Search for content** — Use `mediawiki_search` for full-text search across the wiki. Use `mediawiki_search_in_page` when you know the specific page.

2. **Read pages** — Use `mediawiki_get_page` with the exact page title. If the title might be wrong, use `mediawiki_resolve_title` first (handles case sensitivity and typos).

3. **Explore structure** — Use `mediawiki_get_sections` to see page structure, `mediawiki_list_categories` for topic browsing, `mediawiki_get_category_members` for pages in a category.

4. **Check quality** — Use `mediawiki_check_links` for broken external URLs, `mediawiki_find_broken_internal_links` for broken wiki links, `mediawiki_find_orphaned_pages` for unlinked pages, `mediawiki_check_terminology` for naming consistency.

5. **Track changes** — Use `mediawiki_get_revisions` for page history, `mediawiki_compare_revisions` for diffs, `mediawiki_get_recent_changes` for recent activity (supports aggregation by user, page, or type), `mediawiki_get_user_contributions` for a specific editor's work.

6. **Find related content** — Use `mediawiki_get_related` for pages connected by categories and links, `mediawiki_find_similar_pages` for content-based similarity, `mediawiki_compare_topic` to see how a term is described across pages.

7. **Convert content** — Use `mediawiki_convert_markdown` to transform Markdown to wikitext. Supports themes: tieto, neutral, dark.

## Guardrails

- Page titles are case-sensitive. Use `mediawiki_resolve_title` when unsure.
- Use `mediawiki_search_in_page` instead of fetching entire pages just to find text.
- For aggregated change reports, use the `aggregate_by` parameter on `mediawiki_get_recent_changes`.
