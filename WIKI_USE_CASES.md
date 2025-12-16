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
| Find wiki admins | `mediawiki_list_users` |

---

## Error Recovery Scenarios

When AI agents execute wiki operations, things can go wrong. This section documents common failure scenarios, recovery strategies, and preventive measures.

### Scenario 1: Wrong Page Edited

**What happened**: User says "update the API page" but there are multiple API-related pages. Agent edits the wrong one.

**Example**:
```
User: "Add a note about rate limiting to the API page"
Agent: Edits "API Overview" (wrong)
User: "No, I meant the REST API Authentication page!"
```

**Recovery**:
```
1. mediawiki_get_revisions(title="API Overview") → find the bad edit
2. mediawiki_compare_revisions(from_rev=before, to_rev=after) → see what changed
3. Manual revert via wiki UI, or re-edit to remove the addition
```

**Prevention**:
```
1. mediawiki_search(query="API") → show all matching pages
2. Ask user: "I found 5 pages mentioning API. Which one: API Overview, REST API, SOAP API...?"
3. Confirm before editing
```

**Implementation idea**: Add `mediawiki_search_pages_by_title` tool that finds pages with similar names before editing.

---

### Scenario 2: Find-Replace Hits Unexpected Matches

**What happened**: User says "replace X with Y" but X appears in contexts they didn't expect.

**Example**:
```
User: "Replace 'Smith' with 'Johnson' on the Team page"
Agent: Replaces all instances, including "Smithson" → "Johnsonson"
User: "You broke Smithson's name!"
```

**Recovery**:
```
1. mediawiki_get_revisions(title="Team") → get revision before edit
2. mediawiki_get_page(title="Team") → get current content
3. Manually fix or revert via wiki UI
```

**Prevention**:
```
1. mediawiki_search_in_page(title="Team", query="Smith") → show ALL matches in context
2. Display: "Found 3 matches: 'John Smith' (line 5), 'Smithson' (line 12), 'Smith & Co' (line 20)"
3. Ask: "Replace all, or just specific ones?"
4. mediawiki_find_replace(preview=true) → show exact changes before executing
```

**Implementation idea**: Enhanced `find_replace` response that shows line numbers and surrounding context for each match.

---

### Scenario 3: Bulk Replace Gone Wrong

**What happened**: User wanted to update terminology across docs, but the replacement had unintended side effects.

**Example**:
```
User: "Replace 'API' with 'REST API' across all documentation"
Agent: Replaces everywhere, including "SOAP API" → "SOAP REST API"
User: "You corrupted 50 pages!"
```

**Recovery**:
```
1. The bulk_replace response should include list of affected pages + revision IDs
2. For each page: revert to previous revision
3. Consider: mediawiki_bulk_undo (proposed tool)
```

**Prevention**:
```
1. mediawiki_bulk_replace(preview=true, limit=5) → preview on small sample first
2. Show: "Would change 47 pages. Sample changes: [show 5 examples with context]"
3. Ask: "Proceed with all 47, or refine the search?"
4. Use word boundaries: find="\bAPI\b" (regex) to avoid partial matches
```

**Implementation idea**:
- `bulk_replace` returns `{affected_pages: [{title, old_rev, new_rev}]}` for rollback
- Add `mediawiki_bulk_undo` tool that reverts multiple pages

---

### Scenario 4: Case Sensitivity Mismatch

**What happened**: User's text doesn't match the actual case on the wiki.

**Example**:
```
User: "Strike out john smith on the Team page"
Agent: No matches found (page has "John Smith")
User: "But he's right there on the page!"
```

**Recovery**: Not destructive, but frustrating. User has to retry with correct case.

**Prevention**:
```
1. mediawiki_search_in_page(title="Team", query="john smith") → case-insensitive search
2. Returns: "Found 'John Smith' on line 15 (case differs from your query)"
3. Ask: "Apply strikethrough to 'John Smith'?"
```

**Implementation idea**: Add `case_insensitive` flag to `find_replace` and `apply_formatting` tools.

---

### Scenario 5: Formatting Applied to Wrong Instance

**What happened**: Text appears multiple times; formatting applied to the first instance, not the intended one.

**Example**:
```
User: "Bold the price in the pricing section"
Agent: Bolds "$99" in the header instead of the pricing table
User: "Wrong one - I meant the enterprise price!"
```

**Recovery**:
```
1. mediawiki_find_replace to remove bold from wrong instance
2. mediawiki_apply_formatting on correct instance
```

**Prevention**:
```
1. mediawiki_search_in_page(title="Pricing", query="$99") → show all instances
2. "Found '$99' in 3 places: Header (line 2), Standard tier (line 15), Enterprise tier (line 28)"
3. Ask: "Which instance should I bold?"
4. Use line-specific or context-specific targeting
```

**Implementation idea**: Add `occurrence` parameter: `apply_formatting(text="$99", occurrence=2)` for "second instance".

---

### Scenario 6: Created Duplicate Page Instead of Editing

**What happened**: Page exists with slightly different name; agent creates a new page.

**Example**:
```
User: "Create a page about the REST API"
Agent: Creates "REST API" page
User: "We already have 'REST API Documentation'!"
```

**Recovery**:
```
1. Merge content from new page to existing page
2. Delete duplicate (requires admin)
3. Or add redirect
```

**Prevention**:
```
1. Before creating: mediawiki_resolve_title(title="REST API", fuzzy=true)
2. Returns: "Similar pages exist: 'REST API Documentation', 'REST API Guide'"
3. Ask: "Add content to existing page, or create new 'REST API' page?"
```

**Implementation idea**: `edit_page` tool auto-checks for similar titles before creating new pages.

---

## Context Rot Prevention

"Context rot" = AI loses track of what it did, what state the wiki is in, or what the user's working on.

### Problem 1: Agent Forgets Previous Edits

**Symptom**: User says "undo that" but agent doesn't know what "that" was.

**Current state**: Edit operations return success/failure but minimal context.

**Solution - Edit Receipts**:
Every edit tool should return:
```json
{
  "success": true,
  "page": "Team",
  "old_revision": 1234,
  "new_revision": 1235,
  "changes_summary": "Added strikethrough to 'John Smith'",
  "undo_instruction": "To undo: mediawiki_undo_edit(page='Team', revision=1234)",
  "wiki_undo_url": "https://wiki.example.com/index.php?title=Team&action=edit&undoafter=1234&undo=1235"
}
```

**Implementation**:
- Modify all edit tools to return `old_revision` and `new_revision`
- Add `undo_instruction` field with exact tool call to revert
- Add `wiki_undo_url` for manual revert

---

### Problem 2: No Session History

**Symptom**: Can't answer "what have I changed today?"

**Current state**: No tracking of operations within a session.

**Solution - Session Edit Log**:
```
New tool: mediawiki_get_session_edits()

Returns:
[
  {"time": "14:23", "page": "Team", "action": "strikethrough", "old_rev": 1234, "new_rev": 1235},
  {"time": "14:25", "page": "API Docs", "action": "find_replace", "old_rev": 5678, "new_rev": 5679},
  ...
]
```

**Implementation**:
- Server maintains in-memory log of edits made via MCP
- New tool `mediawiki_get_session_edits` returns the log
- Optional: persist to file for cross-session history

---

### Problem 3: Lost Context After Long Conversations

**Symptom**: After many messages, agent forgets which pages user was working on.

**Solution - Working Set Resource**:
```
New MCP resource: wiki://session/working_set

Returns pages the user has interacted with:
{
  "recently_viewed": ["Team", "API Docs", "Release Notes"],
  "recently_edited": ["Team"],
  "search_results": ["Found 5 pages for 'authentication'"]
}
```

**Implementation**:
- Track pages accessed via `get_page`, `search`, `edit`
- Expose as MCP resource for AI to reference
- Auto-summarize: "You've been working on the Team page and searched for authentication docs"

---

### Problem 4: No Undo Capability

**Symptom**: User says "undo that" and agent can't help.

**Current state**: Must manually revert via wiki UI.

**Solution - Undo Tool**:
```
New tool: mediawiki_undo_edit(
  page: "Team",
  revision: 1234  // revision to restore to
)

Or: mediawiki_undo_last_edit(page: "Team")  // convenience wrapper
```

**Implementation**:
- Use MediaWiki's `action=edit&undo=X&undoafter=Y` API
- Require the edit to be recent (prevent undoing others' work)
- Optionally restrict to edits made via MCP in current session

---

### Problem 5: Preview State Not Preserved

**Symptom**: Agent shows preview, user approves, but agent runs a different operation.

**Example**:
```
Agent: "Preview shows 3 replacements: [details]"
User: "OK, do it"
Agent: *runs slightly different parameters, changes 5 things*
```

**Solution - Preview Tokens**:
```
mediawiki_find_replace(preview=true) returns:
{
  "preview_token": "abc123",
  "matches": [...],
  "would_change": 3
}

mediawiki_find_replace(execute_preview="abc123")
// Executes EXACTLY what was previewed, no re-computation
```

**Implementation**:
- Server stores preview state with token
- Execute-preview mode replays exact operation
- Token expires after N minutes
- Prevents parameter drift between preview and execute

---

## Implementation Roadmap

### Phase 1: Better Responses (Low effort, high value)
- [ ] All edit tools return `old_revision`, `new_revision`
- [ ] Include `undo_instruction` in edit responses
- [ ] Include `wiki_undo_url` for manual revert
- [ ] `find_replace` shows line numbers and context for each match

### Phase 2: Session Tracking (Medium effort)
- [ ] Server-side edit log (in-memory)
- [ ] New tool: `mediawiki_get_session_edits`
- [ ] New resource: `wiki://session/working_set`

### Phase 3: Undo Capability (Medium effort)
- [ ] New tool: `mediawiki_undo_edit`
- [ ] Restrict to recent edits / own session
- [ ] New tool: `mediawiki_bulk_undo` for batch reversions

### Phase 4: Preview Tokens (Higher effort)
- [ ] Server stores preview state
- [ ] Preview returns token
- [ ] Execute-preview mode for exact replay

### Phase 5: Smart Prevention (Higher effort)
- [ ] `case_insensitive` flag for text matching
- [ ] `occurrence` parameter for targeting specific instances
- [ ] Auto-check for similar pages before create
- [ ] `word_boundary` flag to prevent partial matches

---

## Recovery Quick Reference

| Problem | Immediate Recovery | Prevention |
|---------|-------------------|------------|
| Wrong page edited | Get revisions, compare, revert | Search/confirm before editing |
| Bad find-replace | Get old revision, manual fix | Preview first, show all matches |
| Bulk replace disaster | Revert each affected page | Preview sample, use regex word boundaries |
| Case mismatch | Retry with correct case | Case-insensitive search first |
| Wrong instance formatted | Remove formatting, redo correct one | Show all instances, ask which one |
| Created duplicate | Merge or redirect | Check for similar titles first |

---

## Prompts That Reduce Errors

**Instead of**: "Update the API page"
**Say**: "Update the REST API Authentication page" (be specific)

**Instead of**: "Replace X with Y"
**Say**: "Replace 'X' with 'Y' on the Configuration page, preview first" (specify page + preview)

**Instead of**: "Bold the price"
**Say**: "Bold '$199' in the Enterprise tier section of the Pricing page" (specify context)

**Instead of**: "Create a new page about X"
**Say**: "Check if we have a page about X, then create one if not" (check first)

---

## Internal vs Open Source Strategy

This MCP server serves two audiences:
1. **Internal (Tieto)** - Public 360° Wiki at `wiki.software-innovation.com`
2. **Open Source** - Any MediaWiki installation (Wikipedia, Fandom, corporate wikis)

### Strategy Options Evaluated

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| **Separate repos** | Clean separation | Double maintenance, features drift | ❌ No |
| **Fork model** | Clear upstream/downstream | Merge conflicts, still 2 repos | ❌ No |
| **Single repo + config** | One codebase, stays in sync | Must externalize all differences | ✅ Yes |
| **Core + plugins** | Maximum flexibility | Over-engineered for this use case | ❌ No |

### Chosen: Single Repo + Configuration

**Principle**: The codebase is 100% open source. All Tieto-specific behavior comes from configuration, not code.

### What's Currently Tieto-Specific

| Component | Current State | How to Externalize |
|-----------|---------------|-------------------|
| Wiki URL in README | Hardcoded `wiki.software-innovation.com` | Keep as example, add generic examples |
| Bot password instructions | References Tieto SSO | Add generic instructions first |
| Example prompts | Mention "eFormidling", "AutoSaver" | Use generic examples in main docs |
| AI Instructions | Public 360° wiki guidelines | Load from config file or env var |
| Glossary page | `Brand Terminology Glossary` | Already configurable via parameter |

### Implementation Plan

#### Phase A: Clean Separation in README

```markdown
# README structure:

## Quick Start (Generic)
- Works with any MediaWiki
- Generic examples

## Platform Setup
- Claude Desktop, Cursor, VS Code, etc.
- No company-specific references

## Tieto Setup (Collapsible Section)
<details>
<summary>Public 360° Wiki Setup (Tieto employees)</summary>
- wiki.software-innovation.com specific instructions
- Bot password via Tieto SSO
- Tieto-specific example prompts
</details>
```

#### Phase B: Externalize AI Instructions

Current: Instructions hardcoded in `wiki_editing_guidelines.go`

Future:
```go
// Load instructions from:
// 1. MEDIAWIKI_INSTRUCTIONS_FILE env var (path to markdown file)
// 2. MEDIAWIKI_INSTRUCTIONS env var (inline text)
// 3. Default generic instructions (fallback)
```

This lets Tieto deploy with:
```json
{
  "env": {
    "MEDIAWIKI_URL": "https://wiki.software-innovation.com/api.php",
    "MEDIAWIKI_INSTRUCTIONS_FILE": "/path/to/tieto-wiki-guidelines.md"
  }
}
```

#### Phase C: Config File Support

```yaml
# mediawiki-mcp-config.yaml (optional)
wiki_url: https://wiki.software-innovation.com/api.php
instructions_file: ./tieto-guidelines.md
default_glossary: "Brand Terminology Glossary"
default_category_for_audits: "Product Documentation"

# Custom terminology rules
terminology:
  - wrong: "Tietoevry"
    correct: "Tieto"
  - wrong: "Public 360"
    correct: "Public 360°"
```

Server loads config from:
1. `MEDIAWIKI_CONFIG_FILE` env var
2. `./mediawiki-mcp-config.yaml` in current dir
3. Built-in defaults

#### Phase D: Separate Instruction Files

```
mediawiki-mcp-server/
├── instructions/
│   ├── generic.md          # Open source default
│   ├── tieto-internal.md   # Tieto-specific (gitignored or separate repo)
│   └── wikipedia.md        # Example for Wikipedia editing
├── main.go
└── ...
```

### Distribution Strategy

| Audience | Gets | How |
|----------|------|-----|
| Open source users | GitHub releases | Download binary, set URL |
| Tieto employees | Pre-configured package | Internal docs with config file |
| Other enterprises | GitHub + their config | Fork config, not code |

### What NEVER Goes in Open Source

- Actual credentials or tokens
- Internal wiki URLs in code (only in examples/docs)
- Customer names or internal project references
- Tieto-specific glossaries (those live in the wiki itself)

### Migration Checklist

- [ ] Move Tieto setup to collapsible section in README
- [ ] Add generic Quick Start as primary documentation
- [ ] Create `MEDIAWIKI_INSTRUCTIONS_FILE` env var support
- [ ] Create `instructions/generic.md` with universal wiki guidelines
- [ ] Create `instructions/tieto-example.md` as reference (gitignored)
- [ ] Add config file support (`mediawiki-mcp-config.yaml`)
- [ ] Update AI instructions to load from config
- [ ] Test with Wikipedia as example open source wiki
- [ ] Update release notes to emphasize "works with any wiki"

---

## Implementation Details (Phase 1)

### Enhanced Edit Responses

**Current response** (example from `find_replace`):
```json
{
  "success": true,
  "message": "Replaced 3 occurrences"
}
```

**Proposed response**:
```json
{
  "success": true,
  "page": "Team",
  "action": "find_replace",
  "changes": {
    "find": "Smith",
    "replace": "Johnson",
    "occurrences": 3,
    "matches": [
      {"line": 5, "before": "John Smith", "after": "John Johnson"},
      {"line": 12, "before": "Contact: Smith", "after": "Contact: Johnson"},
      {"line": 20, "before": "Smith's report", "after": "Johnson's report"}
    ]
  },
  "revision": {
    "old": 1234,
    "new": 1235,
    "diff_url": "https://wiki.example.com/index.php?diff=1235&oldid=1234"
  },
  "undo": {
    "instruction": "mediawiki_undo_edit(page='Team', to_revision=1234)",
    "wiki_url": "https://wiki.example.com/index.php?title=Team&action=edit&undoafter=1234&undo=1235"
  }
}
```

### Code Changes Required

**File: `wiki/types.go`**
```go
// Add to edit result types
type EditRevisionInfo struct {
    OldRevision int64  `json:"old_revision"`
    NewRevision int64  `json:"new_revision"`
    DiffURL     string `json:"diff_url"`
}

type UndoInfo struct {
    Instruction string `json:"instruction"`
    WikiURL     string `json:"wiki_url"`
}

// Update FindReplaceResult
type FindReplaceResult struct {
    Success  bool             `json:"success"`
    Page     string           `json:"page"`
    Action   string           `json:"action"`
    Changes  FindReplaceChanges `json:"changes"`
    Revision EditRevisionInfo `json:"revision"`
    Undo     UndoInfo         `json:"undo"`
}
```

**File: `wiki/methods.go`**
```go
// In FindReplace method, after successful edit:
func (c *Client) FindReplace(ctx context.Context, args FindReplaceArgs) (FindReplaceResult, error) {
    // ... existing code to find matches ...

    // Get current revision before edit
    pageInfo, _ := c.GetPageInfo(ctx, args.Title)
    oldRevision := pageInfo.LastRevisionID

    // ... perform edit ...

    // Get new revision after edit
    newPageInfo, _ := c.GetPageInfo(ctx, args.Title)
    newRevision := newPageInfo.LastRevisionID

    return FindReplaceResult{
        Success: true,
        Page:    args.Title,
        Action:  "find_replace",
        Revision: EditRevisionInfo{
            OldRevision: oldRevision,
            NewRevision: newRevision,
            DiffURL:     fmt.Sprintf("%s?diff=%d&oldid=%d", c.baseURL, newRevision, oldRevision),
        },
        Undo: UndoInfo{
            Instruction: fmt.Sprintf("mediawiki_undo_edit(page='%s', to_revision=%d)", args.Title, oldRevision),
            WikiURL:     fmt.Sprintf("%s?title=%s&action=edit&undoafter=%d&undo=%d",
                c.baseURL, url.QueryEscape(args.Title), oldRevision, newRevision),
        },
    }, nil
}
```

### Session Tracking Implementation

**File: `wiki/session.go`** (new file)
```go
package wiki

import (
    "sync"
    "time"
)

type EditLogEntry struct {
    Time       time.Time `json:"time"`
    Page       string    `json:"page"`
    Action     string    `json:"action"`
    OldRev     int64     `json:"old_revision"`
    NewRev     int64     `json:"new_revision"`
    Summary    string    `json:"summary"`
}

type Session struct {
    mu          sync.RWMutex
    edits       []EditLogEntry
    viewedPages map[string]time.Time
    searches    []string
}

func NewSession() *Session {
    return &Session{
        edits:       make([]EditLogEntry, 0),
        viewedPages: make(map[string]time.Time),
        searches:    make([]string, 0),
    }
}

func (s *Session) LogEdit(entry EditLogEntry) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.edits = append(s.edits, entry)
}

func (s *Session) LogView(page string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.viewedPages[page] = time.Now()
}

func (s *Session) GetEdits() []EditLogEntry {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return append([]EditLogEntry{}, s.edits...)
}

func (s *Session) GetWorkingSet() map[string]interface{} {
    s.mu.RLock()
    defer s.mu.RUnlock()

    viewed := make([]string, 0, len(s.viewedPages))
    for page := range s.viewedPages {
        viewed = append(viewed, page)
    }

    edited := make([]string, 0)
    for _, e := range s.edits {
        edited = append(edited, e.Page)
    }

    return map[string]interface{}{
        "recently_viewed": viewed,
        "recently_edited": edited,
        "edit_count":      len(s.edits),
    }
}
```

### Undo Tool Implementation

**File: `main.go`** (add to tool registration)
```go
// Undo edit tool
mcp.AddTool(server, &mcp.Tool{
    Name:        "mediawiki_undo_edit",
    Description: "Undo a recent edit by reverting to a previous revision. Use this when the user says 'undo that' or when an edit needs to be reverted. Only works for edits made in the current session by default.",
    Annotations: &mcp.ToolAnnotations{
        Title:           "Undo Edit",
        ReadOnlyHint:    false,
        DestructiveHint: ptr(true),
    },
}, func(ctx context.Context, req *mcp.CallToolRequest, args wiki.UndoEditArgs) (*mcp.CallToolResult, wiki.UndoEditResult, error) {
    // Verify this was a recent edit (safety check)
    // Perform undo via MediaWiki API
    // Return confirmation with new revision info
})
```

**File: `wiki/types.go`**
```go
type UndoEditArgs struct {
    Page       string `json:"page" jsonschema:"required,description=Page title to undo edit on"`
    ToRevision int64  `json:"to_revision" jsonschema:"required,description=Revision ID to restore to"`
    Summary    string `json:"summary" jsonschema:"description=Edit summary for the undo"`
}

type UndoEditResult struct {
    Success     bool   `json:"success"`
    Page        string `json:"page"`
    FromRev     int64  `json:"from_revision"`
    ToRev       int64  `json:"to_revision"`
    NewRev      int64  `json:"new_revision"`
    Message     string `json:"message"`
}
```

---

## Testing Strategy

### Unit Tests for Recovery Features

```go
func TestFindReplaceReturnsRevisionInfo(t *testing.T) {
    // Mock wiki client
    // Perform find_replace
    // Assert response includes old_revision, new_revision, undo info
}

func TestSessionTracksEdits(t *testing.T) {
    session := NewSession()
    session.LogEdit(EditLogEntry{Page: "Test", Action: "find_replace"})

    edits := session.GetEdits()
    assert.Len(t, edits, 1)
    assert.Equal(t, "Test", edits[0].Page)
}

func TestUndoEditRestoresRevision(t *testing.T) {
    // Create a page
    // Edit it
    // Undo
    // Verify content matches original
}
```

### Integration Tests

```go
func TestFullEditRecoveryWorkflow(t *testing.T) {
    // 1. Get page content
    // 2. Do find_replace
    // 3. Verify response has revision info
    // 4. Call undo_edit with the revision
    // 5. Verify page content restored
}
```
