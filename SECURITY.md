# Security Guide for MediaWiki MCP Server

This document outlines the security architecture, protections, and recommendations for the MediaWiki MCP Server.

## Current Security Protections

### 1. HTML Sanitization (XSS Prevention)

The server includes comprehensive HTML sanitization to prevent Cross-Site Scripting (XSS) attacks:

**Removed Elements:**
- `<script>`, `<style>`, `<iframe>`, `<object>`, `<embed>`, `<applet>` tags
- `<form>`, `<meta>`, `<link>`, `<base>` tags

**Removed Attributes:**
- All event handlers (`onclick`, `onerror`, `onload`, `onmouseover`, etc.)
- `style` attributes (can contain CSS expressions)

**Removed URL Schemes:**
- `javascript:` URLs in href/src/action attributes
- `data:` URLs in href/src attributes

### 2. Rate Limiting

- **Concurrent Request Limit**: 3 simultaneous API requests (semaphore-based)
- **Exponential Backoff**: `attempt² × 100ms` for retries
- **Max Retries**: 3 (configurable via `MEDIAWIKI_MAX_RETRIES`)

### 3. Content Size Limits

- **Read Character Limit**: 150,000 characters (150KB) for page content display
- **Write Size Limit**: 200,000 characters (200KB) for edit operations
- **URL Batch Limit**: 20 URLs per `CheckLinks` call
- **Title Batch Limit**: 10 titles per `GetExternalLinksBatch` call
- **Page Limits**: Various tools capped at 50-500 results

### 4. HTTPS Enforcement

- **Required**: Wiki URLs must use HTTPS (credentials are transmitted)
- **Validation**: URL format checked for `api.php` endpoint
- **Escape Hatch**: `MEDIAWIKI_ALLOW_INSECURE=true` for development only

### 5. Content Validation for Writes

Dangerous patterns are blocked when editing pages (unless inside safe code blocks):

**Blocked Patterns:**
- `<script>`, `<html>`, `<iframe>`, `<object>`, `<embed>` tags
- `javascript:` URLs
- `{{#tag:script` parser functions

**Safe Code Block Tags** (patterns allowed inside these):
- `<syntaxhighlight lang="...">` - syntax-highlighted code examples
- `<source lang="...">` - older syntax highlighting
- `<pre>` - preformatted text
- `<code>` - inline code
- `<nowiki>` - prevents wiki parsing

This allows technical documentation with code examples while blocking actual executable content.

### 6. Authentication

- **Bot Password**: Uses MediaWiki bot password authentication (not user passwords)
- **CSRF Protection**: Obtains and uses CSRF tokens for write operations
- **Token Expiry**: Tokens refresh every 20 minutes

### 7. Input Validation

| Parameter | Validation |
|-----------|------------|
| `limit` | Normalized to default/max bounds |
| `timeout` | Bounded 1-30 seconds |
| `URLs` | HTTP/HTTPS scheme required |
| `pattern` | Must be "subpage", "suffix", or "prefix" |
| `category` | Auto-prefixed with "Category:" |

---

## Security Recommendations

### For Server Administrators

#### 1. Enforce HTTPS

**Current Gap**: The server accepts any URL scheme for `MEDIAWIKI_URL`.

**Recommendation**: Validate that `MEDIAWIKI_URL` uses HTTPS:

```go
// Add to config.go
func LoadConfig() (*Config, error) {
    baseURL := os.Getenv("MEDIAWIKI_URL")
    if baseURL == "" {
        return nil, errors.New("MEDIAWIKI_URL environment variable is required")
    }

    parsed, err := url.Parse(baseURL)
    if err != nil {
        return nil, fmt.Errorf("invalid MEDIAWIKI_URL: %w", err)
    }
    if parsed.Scheme != "https" {
        return nil, errors.New("MEDIAWIKI_URL must use HTTPS")
    }
    // ...
}
```

#### 2. Secure Credential Storage

**Current**: Credentials in plain environment variables.

**Recommendation**: Support encrypted credentials or secret managers:
- Use Docker secrets or Kubernetes secrets
- Support AWS Secrets Manager, HashiCorp Vault, or Azure Key Vault
- Consider encrypting credentials at rest

#### 3. Audit Logging

**Recommendation**: Log all write operations for compliance:

```go
// Add to EditPage method
c.logger.Info("Page edited",
    "title", args.Title,
    "user", c.config.Username,
    "timestamp", time.Now().UTC(),
    "ip", clientIP,
)
```

#### 4. Per-Tool Rate Limits

**Current**: Global 3-concurrent-request limit.

**Recommendation**: Implement different limits for read vs. write operations:
- Read operations: 10 concurrent
- Write operations: 1 concurrent (serialize edits)
- External link checks: 5 concurrent per batch

---

### For Wiki Administrators

#### 1. Bot Account Security

- Create dedicated bot accounts with minimal permissions
- Use MediaWiki's bot password feature (Special:BotPasswords)
- Grant only necessary rights (e.g., `editpage` but not `delete`)
- Enable 2FA for the parent account

#### 2. Page Protection

- Protect critical pages from bot edits
- Use MediaWiki's abuse filter for automated vandalism detection
- Monitor Recent Changes for bot activity
- Set up email alerts for sensitive page edits

#### 3. Namespace Restrictions

Consider restricting the bot to specific namespaces:
- Main namespace only (no Talk, User, or Template edits)
- Or create a dedicated namespace for bot-managed content

#### 4. Edit Rate Limits

Configure MediaWiki's `$wgRateLimits` to restrict bot edit frequency:

```php
$wgRateLimits['edit']['bot'] = [ 30, 60 ]; // 30 edits per 60 seconds
```

---

### For MCP Client Users (Claude, Cursor, etc.)

#### 1. Validate Content Before Editing

Before calling `mediawiki_edit_page`:
- Review the proposed content for unintended changes
- Check for potentially harmful wikitext patterns
- Verify the target page is correct

#### 2. Use Read-Only Mode When Possible

If you only need to query the wiki:
- Don't configure `MEDIAWIKI_USERNAME`/`MEDIAWIKI_PASSWORD`
- The server works in read-only mode without credentials

#### 3. Limit Scope of Requests

- Use specific page lists instead of entire categories
- Set reasonable `limit` values
- Avoid recursive operations on large page trees

---

## Threat Model

### Attacks Mitigated

| Attack | Protection |
|--------|------------|
| XSS via HTML content | HTML sanitization removes scripts, event handlers |
| CSRF attacks | Wiki-provided CSRF tokens for writes |
| DoS via large responses | Content truncation at 150KB |
| DoS via concurrent requests | Semaphore limits to 3 concurrent |
| Brute force login | Exponential backoff, MediaWiki's rate limits |
| Session fixation | Uses server-managed cookie jar |
| Malicious wikitext injection | Content validation blocks dangerous patterns outside code blocks |
| MITM credential theft | HTTPS enforcement for wiki URLs |

### Residual Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| ~~HTTPS not enforced~~ | ~~Medium~~ | ✓ **Fixed in v1.1** - URL scheme validated |
| Credentials in environment | Medium | Use secret managers |
| No per-user quotas | Low | Add API key-based rate limiting |
| Unicode normalization | Low | MediaWiki handles normalization |
| Wiki-level permissions | Info | Relies on MediaWiki ACLs |

---

## Content Protection Strategies

### Preventing Malicious Wiki Content

#### 1. Content Validation Before Write

Add wikitext content validation:

```go
func validateWikitext(content string) error {
    // Block external scripts
    if strings.Contains(content, "<script") {
        return errors.New("script tags not allowed")
    }

    // Block raw HTML in wikis that don't allow it
    if strings.Contains(content, "<html") {
        return errors.New("raw HTML not allowed")
    }

    // Block potentially dangerous templates
    dangerousTemplates := []string{"{{#invoke:", "{{#tag:"}
    for _, t := range dangerousTemplates {
        if strings.Contains(content, t) {
            return fmt.Errorf("template %q not allowed", t)
        }
    }

    return nil
}
```

#### 2. Block Dangerous Wikitext Patterns

Patterns to block or flag:
- External image references (`[[File:http://`]`)
- Raw HTML blocks (`<html>`, `<nowiki>` with scripts)
- Lua module invocations (if not trusted)
- Excessive transclusions

#### 3. Content Size Limits for Writes

```go
const MaxEditSize = 100 * 1024 // 100KB

func (c *Client) EditPage(ctx context.Context, args EditPageArgs) (EditResult, error) {
    if len(args.Content) > MaxEditSize {
        return EditResult{}, fmt.Errorf("content too large: %d bytes (max %d)",
            len(args.Content), MaxEditSize)
    }
    // ...
}
```

---

## Implementation Checklist

### High Priority
- [x] Add HTTPS enforcement for `MEDIAWIKI_URL` ✓ (v1.1)
- [x] Add content validation for write operations ✓ (v1.1)
- [ ] Implement audit logging for writes
- [x] Add content size limit for edit operations ✓ (v1.1)

### Medium Priority
- [ ] Support secret manager integration
- [x] Add per-tool rate limiting ✓ (semaphore-based, v1.0)
- [ ] Implement request signing/API keys
- [ ] Add IP allowlisting option

### Low Priority
- [ ] Add namespace restrictions configuration
- [ ] Implement content diff preview before write
- [x] Add configurable content blocklist patterns ✓ (v1.1 - safe code block support)
- [ ] Support OAuth instead of bot passwords

---

## Reporting Security Issues

If you discover a security vulnerability:

1. **Do not** open a public GitHub issue
2. Email the maintainer directly with details
3. Include steps to reproduce
4. Allow 90 days for a fix before public disclosure

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.1 | 2025-12 | HTTPS enforcement, content validation with safe code blocks, 150KB/200KB size limits, comprehensive test suite |
| 1.0 | 2024-12 | Initial security documentation |
