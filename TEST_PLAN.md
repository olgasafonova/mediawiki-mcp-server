# MediaWiki MCP Server - Comprehensive Test Plan

## Overview

This document outlines the testing strategy for the MediaWiki MCP Server v1.5.0. The server exposes 24 tools and 2 resources for AI assistants to interact with MediaWiki wikis.

## Test Categories

### 1. Unit Tests

#### 1.1 Configuration Tests (`wiki/config_test.go`)
- Valid URL parsing
- Missing required URL (should error)
- Invalid timeout format handling
- Negative retry count handling
- Default value application
- Environment variable precedence

#### 1.2 Input Validation Tests (`wiki/validation_test.go`)
- **Search**: Empty query rejection, limit normalization (0→20, 1000→500)
- **GetPage**: Empty title rejection, format validation
- **ListPages**: Limit normalization, namespace validation
- **CategoryMembers**: Category prefix normalization, empty category rejection
- **EditPage**: Empty title/content rejection
- **CheckLinks**: URL limit enforcement (max 20), timeout bounds (1-30s)
- **ExternalLinksBatch**: Title limit enforcement (max 10)
- **CheckTerminology**: Page limit enforcement (max 50)
- **CheckTranslations**: Pattern validation, language requirement
- **FindBrokenInternalLinks**: Page limit (max 100)
- **FindOrphanedPages**: Limit normalization (max 200)
- **GetBacklinks**: Limit normalization (max 500)
- **GetRevisions**: Limit normalization (max 100)
- **GetUserContributions**: Limit normalization (max 500)

#### 1.3 Authentication Tests (`wiki/auth_test.go`)
- Login token acquisition
- Login with valid credentials
- Login with invalid credentials
- CSRF token acquisition
- Token expiry and refresh
- Cookie jar management

#### 1.4 HTML Sanitization Tests (`wiki/sanitize_test.go`)
- Script tag removal
- Style tag removal
- Event handler removal (onclick, onerror, etc.)
- JavaScript URL removal
- Data URL removal
- Nested malicious content
- Edge cases (uppercase tags, spaces in attributes)

### 2. Integration Tests

#### 2.1 API Method Tests (`wiki/methods_test.go`)
- Search with valid query
- GetPage with existing/non-existing page
- ListPages pagination
- CategoryMembers with valid category
- GetPageInfo for existing page
- GetWikiInfo
- GetExternalLinks
- GetBacklinks
- GetRevisions
- GetRecentChanges
- CompareRevisions
- Parse wikitext

#### 2.2 Content Quality Tools
- CheckTerminology with glossary
- CheckTranslations across languages
- FindBrokenInternalLinks
- FindOrphanedPages
- CheckLinks with mix of valid/invalid URLs

### 3. Security Tests

#### 3.1 Injection Tests (`wiki/security_test.go`)
- SQL injection in search queries
- XSS in page titles
- Command injection in parameters
- Path traversal in page titles
- CRLF injection in headers
- Unicode normalization attacks

#### 3.2 Authentication Security
- Brute force protection (rate limiting)
- Session fixation prevention
- Token leakage prevention
- Credential exposure in logs

#### 3.3 Resource Exhaustion
- Large content handling (>25KB truncation)
- Excessive pagination requests
- Concurrent request limiting (max 3)
- Timeout enforcement

### 4. Error Handling Tests

#### 4.1 Network Errors
- Connection timeout
- DNS resolution failure
- SSL certificate errors
- Malformed responses

#### 4.2 API Errors
- Invalid API responses
- Rate limiting responses (429)
- Server errors (5xx)
- Unauthorized responses (401/403)

### 5. Performance Tests

#### 5.1 Response Time
- Search response time <2s
- GetPage response time <1s
- Batch operations response time

#### 5.2 Concurrency
- Semaphore enforcement (3 concurrent)
- No race conditions in token refresh
- Proper context cancellation

## Test Data

### Mock Wiki Responses
```json
{
  "query": {
    "pages": {
      "123": {
        "pageid": 123,
        "title": "Test Page",
        "revisions": [{"content": "Test content"}]
      }
    }
  }
}
```

### Malicious Inputs
```
SQL Injection: "'; DROP TABLE pages; --"
XSS: "<script>alert('xss')</script>"
Path Traversal: "../../../etc/passwd"
Command Injection: "; rm -rf /"
CRLF: "test\r\nSet-Cookie: evil=value"
Unicode: "test\u202Ereverse"
```

## Security Recommendations

### Current Protections
1. HTML sanitization for XSS prevention
2. CSRF token validation for writes
3. Rate limiting via semaphore (3 concurrent)
4. Content truncation (25KB limit)
5. Timeout enforcement (30s default)

### Recommended Enhancements

#### Input Validation
1. **URL Scheme Enforcement**: Require HTTPS for MEDIAWIKI_URL
2. **Title Sanitization**: Block control characters and path traversal
3. **Content Length Limits**: Reject oversized write payloads
4. **Query Sanitization**: Escape special MediaWiki search characters

#### Authentication
1. **Credential Encryption**: Support encrypted environment variables
2. **Token Rotation**: Shorter token expiry for sensitive operations
3. **Audit Logging**: Log all write operations with user context

#### Rate Limiting
1. **Per-Tool Limits**: Different limits for read vs write operations
2. **Backoff on Auth Failures**: Exponential backoff after failed logins
3. **Request Quotas**: Daily/hourly limits per client

#### Content Protection
1. **Write Validation**: Validate content doesn't contain malicious wikitext
2. **Protected Pages**: Respect MediaWiki page protection levels
3. **Namespace Restrictions**: Allow/deny list for editable namespaces

## Test Execution

### Running Tests
```bash
cd mediawiki-mcp-server
go test ./... -v -cover
```

### Coverage Target
- Minimum 80% code coverage
- 100% coverage for security-critical functions (sanitize, auth)

### CI/CD Integration
```yaml
# .github/workflows/test.yml
name: Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - run: go test ./... -v -cover -race
```
