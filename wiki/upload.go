package wiki

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// MaxUploadDataBytesEnv names the env var that overrides the default cap on the
// decoded size of a base64 file_data upload. Value is a positive byte count.
const MaxUploadDataBytesEnv = "MEDIAWIKI_MAX_UPLOAD_DATA_BYTES"

// defaultMaxUploadDataBytes caps the decoded size of an MCP file_data upload.
// It matches MediaWiki's own default $wgMaxUploadSize (100 MiB), so the gate
// doesn't reject what the wiki itself would accept. Base64 in an MCP request
// is held in the model's context at full token cost, so callers who want a
// tighter bound (or whose wiki raised $wgMaxUploadSize) can adjust it via
// MaxUploadDataBytesEnv.
const defaultMaxUploadDataBytes = 100 << 20 // 100 MiB

// UploadFile uploads a file to the wiki
func (c *Client) UploadFile(ctx context.Context, args UploadFileArgs) (UploadFileResult, error) {
	if err := resolveFileData(&args); err != nil {
		return UploadFileResult{}, err
	}
	if err := validateUploadArgs(args); err != nil {
		return UploadFileResult{}, err
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return UploadFileResult{}, fmt.Errorf("authentication required for uploads: %w", err)
	}

	result, err := c.performUpload(ctx, args)
	if err != nil && strings.Contains(err.Error(), "badtoken") {
		c.invalidateCSRFToken()
		result, err = c.performUpload(ctx, args)
	}

	c.logUploadOutcome(args, result, err)
	return result, err
}

// validateUploadArgs enforces required upload inputs.
func validateUploadArgs(args UploadFileArgs) error {
	if args.Filename == "" {
		return fmt.Errorf("filename is required")
	}
	if !hasUploadSource(args) {
		return fmt.Errorf("either file_path, file_url, or file_data is required")
	}
	if hasConflictingSources(args) {
		return fmt.Errorf("file_url and file_data are mutually exclusive; supply only one source")
	}
	return nil
}

// hasUploadSource reports whether at least one content source was supplied.
func hasUploadSource(args UploadFileArgs) bool {
	return args.FilePath != "" || args.FileURL != "" || len(args.FileData) > 0
}

// hasConflictingSources reports whether the caller supplied both a URL and
// inline bytes, which is ambiguous (we will not guess which one to use).
func hasConflictingSources(args UploadFileArgs) bool {
	return args.FileURL != "" && len(args.FileData) > 0
}

// resolveFileData decodes the base64 file_data field that MCP callers supply
// into the FileData byte slice that performUpload consumes. The `wiki` CLI sets
// FileData directly and leaves FileDataB64 empty, so this is a no-op for that
// path. Returns actionable errors for malformed or oversized input.
func resolveFileData(args *UploadFileArgs) error {
	if args.FileDataB64 == "" {
		return nil
	}
	if len(args.FileData) > 0 {
		return fmt.Errorf("file_data supplied both as raw bytes and base64; provide only one")
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(args.FileDataB64))
	if err != nil {
		return fmt.Errorf("file_data is not valid base64 (%w); encode the raw file bytes with standard base64, RFC 4648", err)
	}
	if len(decoded) == 0 {
		return fmt.Errorf("file_data decoded to zero bytes; supply non-empty base64 content")
	}
	if limit := maxUploadDataBytes(); len(decoded) > limit {
		return fmt.Errorf("file_data is %d bytes after decoding, over the %d-byte limit; upload a smaller file or raise %s", len(decoded), limit, MaxUploadDataBytesEnv)
	}

	args.FileData = decoded
	args.FileDataB64 = "" // release the base64 copy now that bytes are resolved
	return nil
}

// maxUploadDataBytes returns the decoded-size cap for base64 uploads, honoring
// a positive integer override in MaxUploadDataBytesEnv.
func maxUploadDataBytes() int {
	if raw := strings.TrimSpace(os.Getenv(MaxUploadDataBytesEnv)); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return defaultMaxUploadDataBytes
}

// logUploadOutcome records an audit entry for an upload attempt, whether it
// failed (err != nil) or completed (with its own success flag).
func (c *Client) logUploadOutcome(args UploadFileArgs, result UploadFileResult, err error) {
	contentHash := hashContent(args.FileURL + args.FilePath + string(args.FileData))
	entry := AuditEntry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Operation:   AuditOpUpload,
		ContentHash: contentHash,
		Summary:     args.Comment,
		WikiURL:     c.config.BaseURL,
	}
	if err != nil {
		entry.Title = "File:" + args.Filename
		entry.Success = false
		entry.Error = err.Error()
		c.logAudit(entry)
		return
	}

	entry.Title = "File:" + result.Filename
	entry.ContentSize = result.Size
	entry.Success = result.Success
	if !result.Success {
		entry.Error = result.Message
	}
	c.logAudit(entry)
}

// performUpload executes a single upload attempt with a fresh CSRF token.
//
// Branch order matters:
//  1. FileURL → wiki fetches the URL itself (subject to host allowlist + SSRF
//     guards in uploadFromURL).
//  2. FileData → caller supplied bytes directly: the `wiki` CLI sets them, or
//     an MCP caller passes base64 via file_data (decoded into FileData by
//     resolveFileData). Uploaded via multipart POST without touching the local
//     filesystem.
//  3. FilePath → falls through to uploadFromFile, where readLocalFile rejects
//     the request. Preserves the "MCP server doesn't read arbitrary local
//     files" stance.
func (c *Client) performUpload(ctx context.Context, args UploadFileArgs) (UploadFileResult, error) {
	token, err := c.getCSRFToken(ctx)
	if err != nil {
		return UploadFileResult{}, fmt.Errorf("failed to get edit token: %w", err)
	}

	if args.FileURL != "" {
		return c.uploadFromURL(ctx, args, token)
	}
	if len(args.FileData) > 0 {
		return c.uploadFromBytes(ctx, args, args.FileData, token)
	}
	return c.uploadFromFile(ctx, args, token)
}

// uploadFromURL uploads a file from a URL.
//
// SECURITY (HG-3): two independent gates protect the upload-from-URL path
// from being used as a server-side request forgery primitive against the
// wiki's network neighbors:
//
//  1. validateFileURL refuses URLs whose host (or any DNS-resolved IP)
//     is private/internal — protects against wiki-as-SSRF-proxy targeting
//     169.254.169.254 (cloud metadata), RFC1918 ranges, link-local, etc.
//  2. validateUploadDomain refuses any host not present on the operator's
//     positive allowlist (MEDIAWIKI_UPLOAD_ALLOWED_DOMAINS). Fail-closed
//     when unset — the wiki is the entity performing the fetch on the
//     bot's behalf, and the agent caller is fully URL-controlling, so
//     the operator must explicitly enumerate trusted source hosts.
//
// Without these gates, an adversarial MCP caller could pass
// args.FileURL = "https://attacker.example/poisoned.svg" and have the
// wiki fetch it (and on wikis that allow SVG, this becomes stored XSS
// via the bot account when ignore_warnings overwrites an existing file).
func (c *Client) uploadFromURL(ctx context.Context, args UploadFileArgs, token string) (UploadFileResult, error) {
	if err := validateFileURL(args.FileURL); err != nil {
		return UploadFileResult{}, err
	}
	if err := validateUploadDomain(args.FileURL); err != nil {
		return UploadFileResult{}, err
	}

	params := url.Values{}
	params.Set("action", "upload")
	params.Set("filename", args.Filename)
	params.Set("url", args.FileURL)
	params.Set("token", token)

	if args.Text != "" {
		params.Set("text", args.Text)
	}
	if args.Comment != "" {
		params.Set("comment", args.Comment)
	}
	if args.IgnoreWarnings {
		params.Set("ignorewarnings", "1")
	}

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return UploadFileResult{}, err
	}

	return c.parseUploadResponse(resp, args.Filename)
}

// uploadFromFile refuses local-path uploads. The MCP server is not permitted
// to read arbitrary local files; callers that need to upload bytes go through
// uploadFromBytes (via UploadFileArgs.FileData), which is what the `wiki` CLI
// uses after reading the file on the user's behalf.
func (c *Client) uploadFromFile(_ context.Context, _ UploadFileArgs, _ string) (UploadFileResult, error) {
	return UploadFileResult{}, fmt.Errorf("failed to read file: local file upload not supported - use file_url or pass bytes via file_data instead")
}

// uploadFromBytes uploads file content from an in-memory byte slice via
// multipart/form-data POST. Used by callers that already have the bytes
// (notably the `wiki` CLI, which reads the local file on the user's behalf
// and passes them through UploadFileArgs.FileData).
func (c *Client) uploadFromBytes(ctx context.Context, args UploadFileArgs, fileData []byte, token string) (UploadFileResult, error) {
	// Create multipart request
	boundary := "----WikiUploadBoundary" + strconv.FormatInt(time.Now().UnixNano(), 36)

	var body strings.Builder
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"action\"\r\n\r\n")
	body.WriteString("upload\r\n")

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"format\"\r\n\r\n")
	body.WriteString("json\r\n")

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"filename\"\r\n\r\n")
	body.WriteString(args.Filename + "\r\n")

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"token\"\r\n\r\n")
	body.WriteString(token + "\r\n")

	if args.Text != "" {
		body.WriteString("--" + boundary + "\r\n")
		body.WriteString("Content-Disposition: form-data; name=\"text\"\r\n\r\n")
		body.WriteString(args.Text + "\r\n")
	}

	if args.Comment != "" {
		body.WriteString("--" + boundary + "\r\n")
		body.WriteString("Content-Disposition: form-data; name=\"comment\"\r\n\r\n")
		body.WriteString(args.Comment + "\r\n")
	}

	if args.IgnoreWarnings {
		body.WriteString("--" + boundary + "\r\n")
		body.WriteString("Content-Disposition: form-data; name=\"ignorewarnings\"\r\n\r\n")
		body.WriteString("1\r\n")
	}

	body.WriteString("--" + boundary + "\r\n")
	body.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\n", args.Filename))
	body.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	body.Write(fileData)
	body.WriteString("\r\n")
	body.WriteString("--" + boundary + "--\r\n")

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL, strings.NewReader(body.String()))
	if err != nil {
		return UploadFileResult{}, err
	}

	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	// Use HTTP client to make request
	resp, err := c.httpClient.Do(req) // #nosec G704 -- URL is the configured wiki API endpoint, not user-controlled
	if err != nil {
		return UploadFileResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse JSON response
	var result map[string]interface{}
	if err := c.parseJSONResponse(resp, &result); err != nil {
		return UploadFileResult{}, err
	}

	return c.parseUploadResponse(result, args.Filename)
}

// parseUploadResponse parses the upload API response
func (c *Client) parseUploadResponse(resp map[string]interface{}, filename string) (UploadFileResult, error) {
	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		return UploadFileResult{
			Success:  false,
			Filename: filename,
			Message:  fmt.Sprintf("Upload failed: %s", errInfo["info"]),
		}, nil
	}

	upload, ok := resp["upload"].(map[string]interface{})
	if !ok {
		return UploadFileResult{
			Success:  false,
			Filename: filename,
			Message:  "Unexpected response format",
		}, nil
	}

	result := UploadFileResult{
		Filename: filename,
	}

	switch status := getString(upload["result"]); status {
	case "Success":
		applyUploadSuccess(upload, &result)
	case "Warning":
		applyUploadWarning(upload, &result)
	default:
		result.Success = false
		result.Message = fmt.Sprintf("Upload status: %s", status)
	}

	return result, nil
}

// applyUploadSuccess fills a successful upload result, including image metadata.
func applyUploadSuccess(upload map[string]interface{}, result *UploadFileResult) {
	result.Success = true
	result.Message = "File uploaded successfully"
	if imageinfo, ok := upload["imageinfo"].(map[string]interface{}); ok {
		result.URL = getString(imageinfo["url"])
		result.Size = getInt(imageinfo["size"])
	}
}

// applyUploadWarning fills a warning upload result with the reported warnings.
func applyUploadWarning(upload map[string]interface{}, result *UploadFileResult) {
	result.Success = false
	result.Message = "Upload has warnings - set ignore_warnings=true to proceed"
	if warnings, ok := upload["warnings"].(map[string]interface{}); ok {
		for k, v := range warnings {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", k, v))
		}
	}
}

// parseJSONResponse decodes an *http.Response body as JSON into target.
// Used by the multipart upload path, which cannot reuse the form-encoded
// apiRequest helper. Bounded read to defend against runaway bodies.
func (c *Client) parseJSONResponse(resp *http.Response, target interface{}) error {
	if resp == nil || resp.Body == nil {
		return fmt.Errorf("nil response body")
	}
	// Cap the response body to a sane size. Upload responses are small (a few
	// hundred bytes of metadata) — multi-megabyte responses signal a wrong-
	// content-type or attacker-shaped server reply.
	const maxRespBytes = 1 << 20 // 1 MiB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	if err != nil {
		return fmt.Errorf("read upload response: %w", err)
	}
	if len(body) == 0 {
		return fmt.Errorf("empty upload response (status %d)", resp.StatusCode)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode upload response: %w (body: %.200q)", err, string(body))
	}
	return nil
}
