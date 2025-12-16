# MediaWiki MCP Server - Use Cases

This document outlines practical use cases for the MediaWiki MCP Server, organized by user persona and workflow.

## Content Editors

### 1. Quick Text Corrections
**Scenario**: Fix typos, update terminology, correct outdated information.

```
User: "Change 'Public 360' to 'Public 360°' on the API Overview page"
Tool: mediawiki_find_replace (preview=true first, then execute)
```

### 2. Employee Offboarding
**Scenario**: Mark former employees in documentation as departed.

```
User: "Strike out John Smith's name - he left the company"
Tool: mediawiki_apply_formatting (format="strikethrough")
```

### 3. Brand Consistency Updates
**Scenario**: Update brand names across multiple pages after rebranding.

```
User: "Replace 'Tieto' with 'Tietoevry' on all Product Documentation pages"
Tool: mediawiki_bulk_replace (category="Product Documentation")
```

### 4. Content Discovery
**Scenario**: Find where specific information is documented.

```
User: "Where is the authentication flow documented?"
Tool: mediawiki_search (query="authentication flow")
```

---

## Technical Writers

### 5. Documentation Audits
**Scenario**: Identify and fix broken links before release.

```
User: "Check for broken external links in the Release Notes category"
Tools:
1. mediawiki_get_category_members (category="Release Notes")
2. mediawiki_get_external_links_batch (titles from step 1)
3. mediawiki_check_links (URLs from step 2)
```

### 6. Terminology Compliance
**Scenario**: Ensure documentation follows brand guidelines.

```
User: "Scan the API documentation for incorrect terminology"
Tool: mediawiki_check_terminology (category="API", glossary_page="Brand Terminology Glossary")
```

### 7. Translation Gap Analysis
**Scenario**: Identify pages missing translations.

```
User: "Which pages are missing Danish translations?"
Tool: mediawiki_check_translations (languages=["da"], pattern="subpage")
```

### 8. Orphan Page Cleanup
**Scenario**: Find pages that aren't linked from anywhere.

```
User: "Find orphaned pages that need linking or deletion"
Tool: mediawiki_find_orphaned_pages (namespace=0)
```

---

## Product Managers

### 9. Release Documentation
**Scenario**: Create and update release notes.

```
User: "Create a new page for the v6.8 release notes"
Tool: mediawiki_edit_page (title="Release Notes v6.8", content="...")
```

### 10. Feature Documentation Tracking
**Scenario**: Review what's documented for a feature.

```
User: "What pages mention the new SSO feature?"
Tool: mediawiki_search (query="SSO single sign-on")
```

### 11. Impact Analysis
**Scenario**: Before deprecating a feature, find dependent documentation.

```
User: "What pages link to the SOAP API documentation?"
Tool: mediawiki_get_backlinks (title="SOAP API")
```

---

## Developers

### 12. API Reference Lookup
**Scenario**: Quick access to API documentation.

```
User: "Show me the REST API authentication page"
Tool: mediawiki_get_page (title="REST API Authentication")
```

### 13. Code Example Updates
**Scenario**: Update code samples across documentation.

```
User: "Update all Python examples from v2 to v3 syntax"
Tool: mediawiki_bulk_replace (find="python2", replace="python3", category="Code Examples")
```

### 14. Integration Documentation
**Scenario**: Find integration guides for third-party systems.

```
User: "Find all pages about SharePoint integration"
Tool: mediawiki_search_in_page (title="Integrations Index", query="SharePoint")
```

---

## Wiki Administrators

### 15. Activity Monitoring
**Scenario**: Track recent changes to the wiki.

```
User: "What changed on the wiki in the last week?"
Tool: mediawiki_get_recent_changes (limit=100)
```

### 16. User Contribution Review
**Scenario**: Review edits by a specific contributor.

```
User: "What has john.smith@company.com edited recently?"
Tool: mediawiki_get_user_contributions (user="john.smith@company.com")
```

### 17. Page History Analysis
**Scenario**: Investigate changes to a critical page.

```
User: "Show the edit history for the Security Policy page"
Tool: mediawiki_get_revisions (title="Security Policy")
```

### 18. Diff Comparison
**Scenario**: Compare versions of a page.

```
User: "What changed between revision 1234 and 1250?"
Tool: mediawiki_compare_revisions (from_rev=1234, to_rev=1250)
```

---

## Quality Assurance

### 19. Internal Link Verification
**Scenario**: Find broken wiki links before publishing.

```
User: "Check for broken internal links in the User Guide section"
Tool: mediawiki_find_broken_internal_links (pages=["User Guide", "Getting Started", "Configuration"])
```

### 20. Content Search Within Page
**Scenario**: Verify specific content exists on a page.

```
User: "Does the Security page mention GDPR?"
Tool: mediawiki_search_in_page (title="Security", query="GDPR")
```

### 21. Category Inventory
**Scenario**: List all pages in a category for review.

```
User: "List all pages in the Deprecated category"
Tool: mediawiki_get_category_members (category="Deprecated")
```

---

## Automation Workflows

### 22. Scheduled Content Audits
**Scenario**: Regular automated checks for content quality.

```python
# Weekly audit script
1. Get all pages in "Production Documentation"
2. Check external links for each
3. Report broken links via email/Slack
```

### 23. Release Automation
**Scenario**: Automatically update version numbers across docs.

```python
# Release script
1. mediawiki_bulk_replace(find="v6.7", replace="v6.8", category="Version-sensitive")
2. mediawiki_edit_page(title="Current Version", content="v6.8")
```

### 24. Onboarding Documentation
**Scenario**: Programmatically create user-specific pages.

```python
# New team member script
1. Get template page content
2. Replace placeholders with user info
3. Create personalized onboarding page
```

---

## Best Practices

### Preview Before Editing
Always use `preview=true` for destructive operations:
```
1. mediawiki_find_replace(preview=true) - see what will change
2. Confirm with user
3. mediawiki_find_replace(preview=false) - execute
```

### Title Resolution
Wiki titles are case-sensitive. When a page isn't found:
```
1. mediawiki_resolve_title(title="module overview")
2. Returns: "Module Overview" (correct case)
3. mediawiki_get_page(title="Module Overview")
```

### Batch Operations
For large operations, work in batches:
- External link checks: max 20 URLs per call
- Title batches: max 10 per call
- Category scans: use pagination with `continue_from`

---

## Tool Selection Quick Reference

| I want to... | Use this tool |
|--------------|---------------|
| Search the whole wiki | `mediawiki_search` |
| Search within one page | `mediawiki_search_in_page` |
| Read a page | `mediawiki_get_page` |
| Fix a typo | `mediawiki_find_replace` |
| Format text | `mediawiki_apply_formatting` |
| Update across pages | `mediawiki_bulk_replace` |
| Check broken links | `mediawiki_check_links` + `mediawiki_find_broken_internal_links` |
| See edit history | `mediawiki_get_revisions` |
| Find what links here | `mediawiki_get_backlinks` |
| Handle wrong title case | `mediawiki_resolve_title` |
