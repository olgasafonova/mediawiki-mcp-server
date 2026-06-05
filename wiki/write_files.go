package wiki

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func extractNormalizedTitleMap(query map[string]interface{}) map[string]string {
	normalized := make(map[string]string)
	normList, ok := query["normalized"].([]interface{})
	if !ok {
		return normalized
	}
	for _, n := range normList {
		norm, ok := n.(map[string]interface{})
		if !ok {
			continue
		}
		normalized[getString(norm["to"])] = getString(norm["from"])
	}
	return normalized
}

// recordExistenceFromBatchPages walks the "pages" map from a query response
// and writes existence flags into result, including the un-normalized form
// if applicable.
func recordExistenceFromBatchPages(pages map[string]interface{}, normalized map[string]string, result map[string]bool) {
	for _, pageData := range pages {
		page, ok := pageData.(map[string]interface{})
		if !ok {
			continue
		}
		title := getString(page["title"])
		_, missing := page["missing"]
		exists := !missing
		result[title] = exists
		if originalTitle, ok := normalized[title]; ok {
			result[originalTitle] = exists
		}
	}
}

// queryPageExistence runs one batch existence query and merges the outcome
// into result. Returns the API error (network/auth) without aborting on
// missing-from-response cases — those are caught by the caller's fallback loop.
func (c *Client) queryPageExistence(ctx context.Context, batch []string, result map[string]bool) error {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", strings.Join(batch, "|"))
	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return err
	}
	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return nil
	}
	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return nil
	}
	recordExistenceFromBatchPages(pages, extractNormalizedTitleMap(query), result)
	return nil
}

// checkPagesExist checks if multiple pages exist using MediaWiki's multi-value API
// Returns a map of page title -> exists (bool)
// This is much more efficient than calling GetPageInfo for each page individually
func (c *Client) checkPagesExist(ctx context.Context, titles []string) (map[string]bool, error) {
	if len(titles) == 0 {
		return make(map[string]bool), nil
	}

	const maxTitlesPerRequest = 50
	result := make(map[string]bool, len(titles))

	for i := 0; i < len(titles); i += maxTitlesPerRequest {
		end := i + maxTitlesPerRequest
		if end > len(titles) {
			end = len(titles)
		}
		if err := c.queryPageExistence(ctx, titles[i:end], result); err != nil {
			return nil, err
		}
	}

	// Default any titles that didn't appear in any response to non-existent.
	for _, title := range titles {
		if _, ok := result[title]; !ok {
			result[title] = false
		}
	}
	return result, nil
}

// fileTypeFromMIME returns a friendly file type label from a MIME string.
func fileTypeFromMIME(mimeType string) string {
	if mimeType == "application/pdf" {
		return "pdf"
	}
	if strings.HasPrefix(mimeType, "text/") {
		return strings.TrimPrefix(mimeType, "text/")
	}
	return mimeType
}

// extractFileURLFromPageEntry pulls the download URL and file type from one
// entry in the imageinfo response. Returns an error if the entry is malformed,
// represents a missing file, or has no URL.
func extractFileURLFromPageEntry(page map[string]interface{}, filename string) (fileURL, fileType string, err error) {
	if _, missing := page["missing"]; missing {
		return "", "", fmt.Errorf("file '%s' does not exist", filename)
	}
	imageinfo, ok := page["imageinfo"].([]interface{})
	if !ok || len(imageinfo) == 0 {
		return "", "", fmt.Errorf("no file info available for '%s'", filename)
	}
	info, ok := imageinfo[0].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("unexpected API response: invalid imageinfo format for '%s'", filename)
	}
	fileURL = getString(info["url"])
	if fileURL == "" {
		return "", "", fmt.Errorf("no download URL for '%s'", filename)
	}
	return fileURL, fileTypeFromMIME(getString(info["mime"])), nil
}

func (c *Client) getFileURL(ctx context.Context, filename string) (string, string, error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", filename)
	params.Set("prop", "imageinfo")
	params.Set("iiprop", "url|mime|size")

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return "", "", err
	}
	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("unexpected response format")
	}
	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("no pages in response")
	}

	for _, pageData := range pages {
		page, ok := pageData.(map[string]interface{})
		if !ok {
			continue
		}
		return extractFileURLFromPageEntry(page, filename)
	}
	return "", "", fmt.Errorf("file '%s' not found", filename)
}

// downloadFile downloads a file from the given URL.
//
// SECURITY: The fileURL is validated against SSRF (private IPs, DNS rebinding,
// redirect bypass) via validateFileURL before any network I/O. The actual
// HTTP request goes through downloadClient, which uses safeDialer +
// CheckRedirect for defense in depth. Callers in production today (only
// SearchInFile) get fileURL from the wiki's own imageinfo API, so the URL is
// already trusted. The validation closes the gap for any future caller that
// might accept user-supplied URLs.
//
// Tests can set c.allowPrivateDownloadForTest to skip validation against
// httptest servers (which bind to 127.0.0.1).
func (c *Client) downloadFile(ctx context.Context, fileURL string) ([]byte, error) {
	httpClient := downloadClient
	if !c.allowPrivateDownloadForTest {
		if err := validateFileURL(fileURL); err != nil {
			return nil, fmt.Errorf("download URL rejected: %w", err)
		}
	} else {
		// Test path: bypass SSRF guards so httptest (loopback) servers work.
		httpClient = c.httpClient
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", c.config.UserAgent)

	// Forward session cookies from the API client's jar onto the download
	// request. Required for MediaWiki instances that gate file access behind
	// a login (the $wgUploadDirectory + img_auth.php pattern common on
	// corporate wikis): the file-serving endpoint refuses anonymous reads
	// even when the bot is authenticated for api.php. The standard cookie jar
	// is domain-scoped (RFC 6265), so cookies set for the wiki host are only
	// forwarded when fileURL is on the same host — no credential leakage to
	// arbitrary external hosts.
	if c.httpClient != nil && c.httpClient.Jar != nil {
		for _, cookie := range c.httpClient.Jar.Cookies(req.URL) {
			req.AddCookie(cookie)
		}
	}

	resp, err := httpClient.Do(req) // #nosec G107 G704 -- URL validated by validateFileURL; request goes through downloadClient (safeDialer + CheckRedirect)
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Surface the known bot-password + img_auth.php gotcha with an
		// actionable message rather than a bare status code. Wikis that gate
		// file access via img_auth.php check user-level auth (UserID/UserName/
		// Token cookies), which the legacy `action=login` bot-password flow
		// doesn't establish — only a BPsession cookie. The session is fine
		// for api.php but the file-serving endpoint rejects it. Tracked as
		// mediawiki-mcp-server-xum.
		if resp.StatusCode == http.StatusForbidden &&
			c.config.HasCredentials() &&
			strings.Contains(req.URL.Path, "/img_auth.php") {
			return nil, fmt.Errorf(
				"download failed with status 403: %s is gated behind img_auth.php and refused the bot-password session. "+
					"Bot-password logins authenticate against api.php but img_auth.php enforces user-level auth this flow does not establish. "+
					"This is a wiki-side limitation, not a CLI bug. Workarounds: download the file manually from the wiki UI, "+
					"or ask a wiki admin to grant additional rights to your bot password at Special:BotPasswords",
				req.URL.Host)
		}
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Limit download size to 50MB
	const maxSize = 50 * 1024 * 1024
	limitedReader := &io.LimitedReader{R: resp.Body, N: maxSize}

	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file data: %w", err)
	}

	if limitedReader.N <= 0 {
		return nil, fmt.Errorf("file exceeds maximum size of 50MB")
	}

	return data, nil
}
