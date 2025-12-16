package main

// WikiEditingGuidelines contains the Public 360° Wiki editing policies.
// Source: https://wiki.software-innovation.com/wiki/Editing_the_wiki_-_Guidelines_and_tips
// These are embedded in MCP server instructions to guide AI editing behavior.
const WikiEditingGuidelines = `
## WIKI EDITING GUIDELINES (Public 360° Wiki)

### Language Requirement
- ALL content MUST be in English
- No Norwegian/Danish/Swedish articles allowed

### Article Naming
Article titles must be:
1. **Recognizable** - Name readers would search for
2. **Natural** - What the subject is actually called
3. **Precise** - Unambiguously identifies the subject
4. **Concise** - No longer than necessary
5. **Consistent** - Matches similar article patterns

❌ DON'T create complex naming systems
✅ DO name articles after what they're actually about

### Category Naming
- Be specific, neutral, and inclusive
- Follow patterns like "Hospitals in Denmark" or "Australian journalists"
- Use existing categories when possible

### Content Guidelines
1. **Be specific** with page titles and section titles
2. **Be concise** in everything you write
3. **Article size** - not too big, not too small
4. If content gives no value or is unreadable, it may be deleted

### Internal Links
NEVER use full URLs for internal wiki links!
- ✅ Correct: [[Page Title]] or [[Page Title|Display Text]]
- ❌ Wrong: https://wiki.software-innovation.com/wiki/Page_Title

### Contact Information
DON'T create names as email links - wiki doesn't support this properly!
- ✅ Correct: "John Smith (john.smith@tietoevry.com)"
- ❌ Wrong: [[mailto:john.smith@tietoevry.com|John Smith]]

### File Uploads
Supported formats: doc, docx, xlsx, xls, mpp, pdf, ppt, pptx, jpg, tiff, odt, odg, ods, odp, svg, zip
Maximum size: 10MB

### Housekeeping

**For outdated content (keep for history):**
1. Add {{Expired}} at top of page
2. Add [[Category:Expired]] at bottom

**For content to be deleted:**
- Add [[Category:Page for deletion]]
- Only admins can delete pages

**For renaming pages:**
- Use "More" → "Move" (not delete and recreate)
- Keep "Leave a redirect behind" checked

### Edit Summaries
Always provide clear summaries explaining changes:
- "Updated product version to 3.0"
- "Struck out former employee name"
- "Fixed broken internal links"
- "Added new section on authentication"
`

// MCP2025BestPractices contains guidelines from the MCP specification
// for security, consent, and proper tool usage.
const MCP2025BestPractices = `
## MCP SECURITY & CONSENT (2025 Spec)

### User Consent Requirements
- Users must explicitly consent to all data access and operations
- Users retain control over what data is shared and what actions are taken
- Provide clear information about what each operation will do

### Preview-Before-Save Workflow
For any edit operation:
1. Use preview=true to show what will change
2. Get user confirmation
3. Only then execute the actual edit

This is MANDATORY for:
- mediawiki_find_replace
- mediawiki_apply_formatting
- mediawiki_bulk_replace
- mediawiki_edit_page

### Tool Safety
- Tool descriptions are metadata guidance, not security guarantees
- Always verify operations before executing
- Obtain explicit user consent before making changes

### Data Privacy
- Don't transmit wiki data elsewhere without user consent
- Be transparent about what data is being accessed
- Protect user credentials and session data
`
