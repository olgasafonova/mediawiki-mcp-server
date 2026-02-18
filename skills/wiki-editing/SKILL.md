---
name: wiki-editing
description: Edit MediaWiki pages safely using the right tool for each change. Covers text replacement, formatting, bulk updates, and new page creation with preview-before-save workflow.
---

# Wiki Editing

## Trigger

Making changes to wiki pages: text corrections, formatting, bulk replacements, or creating new content.

## Tool Selection

Pick the simplest tool that handles the edit:

| Change type | Tool |
|-------------|------|
| Strike out or format text (bold, italic, underline) | `mediawiki_apply_formatting` |
| Replace specific text on one page | `mediawiki_find_replace` |
| Replace a term across multiple pages | `mediawiki_bulk_replace` |
| Add new content or complex multi-section edits | `mediawiki_edit_page` |
| Upload a file from URL | `mediawiki_upload_file` |

Do NOT use `mediawiki_edit_page` for simple text changes when `mediawiki_find_replace` or `mediawiki_apply_formatting` would work.

## Workflow

1. **Preview first** — Always use `preview=true` on `mediawiki_find_replace` and `mediawiki_bulk_replace` before making changes. Show the preview to the user and get confirmation.

2. **Make the edit** — Execute with `preview=false` after confirmation.

3. **Include an edit summary** — Describe what changed: "Updated version to 3.0", "Struck out former employee name", "Fixed broken internal links".

## Guardrails

- Always preview destructive edits before executing them.
- Use internal wiki links `[[Page Title]]`, never full URLs for wiki pages.
- Don't create email links for names; wiki doesn't support `mailto:` properly.
- Page titles are case-sensitive. Use `mediawiki_resolve_title` if unsure.
- Editing requires authentication (`MEDIAWIKI_USERNAME` and `MEDIAWIKI_PASSWORD` environment variables).
- All content must be in English on the wiki.
