package wiki

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// These tests complement the upload coverage in write_test.go. They target the
// branches not already exercised there: the SSRF/allowlist gates on
// uploadFromURL, the fall-through cases of parseUploadResponse, and the body
// handling in parseJSONResponse (the existing TestParseJSONResponse only checks
// the nil-response guard).

// newUploadBodyResponse builds an *http.Response whose Body wraps the given
// string, for exercising parseJSONResponse without a live server.
func newUploadBodyResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
	}
}

func TestUploadFromURL_RejectsNonAllowlistedDomain(t *testing.T) {
	// Empty allowlist => fail-closed. Even a public, non-private host is refused
	// before any network request is attempted.
	t.Setenv(UploadAllowlistEnv, "")

	client := createTestClient(t)
	defer client.Close()

	_, err := client.uploadFromURL(context.Background(), UploadFileArgs{
		Filename: "Example.png",
		FileURL:  "https://cdn.example.com/a.png",
	}, "test-csrf-token")
	if err == nil {
		t.Fatal("expected fail-closed rejection for empty allowlist")
	}
}

func TestUploadFromURL_RejectsPrivateIP(t *testing.T) {
	// Allowlist the host, but the URL still resolves to a private/link-local IP
	// and must be refused by the SSRF guard regardless of the domain allowlist.
	t.Setenv(UploadAllowlistEnv, "169.254.169.254")

	client := createTestClient(t)
	defer client.Close()

	_, err := client.uploadFromURL(context.Background(), UploadFileArgs{
		Filename: "Meta.png",
		FileURL:  "http://169.254.169.254/latest/meta-data/",
	}, "test-csrf-token")
	if err == nil {
		t.Fatal("expected SSRF rejection for link-local metadata address")
	}
}

func TestResolveFileData(t *testing.T) {
	const oneByte = "AA==" // base64 of a single 0x00 byte

	tests := []struct {
		name        string
		args        UploadFileArgs
		wantErr     bool
		errContains string
		wantBytes   []byte
	}{
		{
			name: "empty base64 is a no-op (CLI path)",
			args: UploadFileArgs{FileData: []byte("rawbytes")},
			// FileDataB64 empty: FileData must survive untouched.
			wantBytes: []byte("rawbytes"),
		},
		{
			name:      "valid base64 decodes into FileData",
			args:      UploadFileArgs{FileDataB64: base64.StdEncoding.EncodeToString([]byte("hello wiki"))},
			wantBytes: []byte("hello wiki"),
		},
		{
			name:      "surrounding whitespace is trimmed before decode",
			args:      UploadFileArgs{FileDataB64: "  " + oneByte + "\n"},
			wantBytes: []byte{0x00},
		},
		{
			name:        "invalid base64 is rejected with guidance",
			args:        UploadFileArgs{FileDataB64: "not!!base64"},
			wantErr:     true,
			errContains: "not valid base64",
		},
		{
			name:      "empty string is the no-op branch, leaves FileData nil",
			args:      UploadFileArgs{FileDataB64: ""},
			wantBytes: nil,
		},
		{
			name:        "both raw bytes and base64 is rejected",
			args:        UploadFileArgs{FileData: []byte("raw"), FileDataB64: oneByte},
			wantErr:     true,
			errContains: "both as raw bytes and base64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.args
			err := resolveFileData(&args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(args.FileData, tt.wantBytes) {
				t.Errorf("FileData = %q, want %q", args.FileData, tt.wantBytes)
			}
			if args.FileDataB64 != "" {
				t.Errorf("FileDataB64 should be cleared after decode, got %q", args.FileDataB64)
			}
		})
	}
}

func TestResolveFileData_ZeroBytesRejected(t *testing.T) {
	// The early "" check guards the no-op CLI path, so the zero-byte guard is
	// reached only when a non-empty field trims down to nothing: whitespace-only
	// input passes the != "" check, then TrimSpace yields "", which base64-decodes
	// to zero bytes without an error. That must be rejected, not silently uploaded.
	args := UploadFileArgs{FileDataB64: "   \n\t "}
	err := resolveFileData(&args)
	if err == nil {
		t.Fatal("expected zero-byte rejection for whitespace-only file_data")
	}
	if !strings.Contains(err.Error(), "zero bytes") {
		t.Errorf("error = %q, want zero-byte message", err.Error())
	}
}

// TestUploadFile_FromBase64_EndToEnd proves the full MCP path: a base64
// file_data string is decoded and POSTed to the wiki as multipart bytes,
// without any URL fetch (so the SSRF/allowlist gates are never consulted).
func TestUploadFile_FromBase64_EndToEnd(t *testing.T) {
	const payload = "fake-png-bytes"
	var gotBytes []byte
	var gotFilename string

	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		// The upload POST is multipart; the mock wrapper already parsed it via
		// FormValue, so the file part is available here.
		file, hdr, err := r.FormFile("file")
		if err != nil {
			t.Errorf("expected a multipart file part: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer func() { _ = file.Close() }()
		gotBytes, _ = io.ReadAll(file)
		gotFilename = hdr.Filename

		resp := map[string]interface{}{
			"upload": map[string]interface{}{
				"result": "Success",
				"imageinfo": map[string]interface{}{
					"url":  "http://wiki.test/images/Logo.png",
					"size": float64(len(payload)),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.UploadFile(context.Background(), UploadFileArgs{
		Filename:    "Logo.png",
		FileDataB64: base64.StdEncoding.EncodeToString([]byte(payload)),
		Comment:     "via base64",
	})
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got message: %s", result.Message)
	}
	if string(gotBytes) != payload {
		t.Errorf("server received %q, want %q", gotBytes, payload)
	}
	if gotFilename != "Logo.png" {
		t.Errorf("multipart filename = %q, want Logo.png", gotFilename)
	}
}

func TestResolveFileData_SizeCap(t *testing.T) {
	t.Setenv(MaxUploadDataBytesEnv, "8")

	// 9 raw bytes -> exceeds the 8-byte cap set above.
	args := UploadFileArgs{FileDataB64: base64.StdEncoding.EncodeToString([]byte("123456789"))}
	err := resolveFileData(&args)
	if err == nil {
		t.Fatal("expected size-cap rejection")
	}
	if !strings.Contains(err.Error(), MaxUploadDataBytesEnv) {
		t.Errorf("error should name the override env var, got: %v", err)
	}

	// 8 bytes exactly -> at the cap, allowed.
	args = UploadFileArgs{FileDataB64: base64.StdEncoding.EncodeToString([]byte("12345678"))}
	if err := resolveFileData(&args); err != nil {
		t.Fatalf("8 bytes is at the cap and should be allowed, got: %v", err)
	}
}

func TestMaxUploadDataBytes_Default(t *testing.T) {
	t.Setenv(MaxUploadDataBytesEnv, "")
	if got := maxUploadDataBytes(); got != defaultMaxUploadDataBytes {
		t.Errorf("maxUploadDataBytes() = %d, want default %d", got, defaultMaxUploadDataBytes)
	}
}

func TestMaxUploadDataBytes_IgnoresInvalidOverride(t *testing.T) {
	for _, raw := range []string{"notanumber", "-5", "0"} {
		t.Setenv(MaxUploadDataBytesEnv, raw)
		if got := maxUploadDataBytes(); got != defaultMaxUploadDataBytes {
			t.Errorf("override %q: maxUploadDataBytes() = %d, want default %d", raw, got, defaultMaxUploadDataBytes)
		}
	}
}

func TestValidateUploadArgs_MutualExclusion(t *testing.T) {
	err := validateUploadArgs(UploadFileArgs{
		Filename: "x.png",
		FileURL:  "https://cdn.example.com/x.png",
		FileData: []byte("bytes"),
	})
	if err == nil {
		t.Fatal("expected mutual-exclusion error when both file_url and file_data are set")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want mutual-exclusion message", err.Error())
	}
}

func TestParseUploadResponse_EdgeCases(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	tests := []struct {
		name        string
		resp        map[string]interface{}
		filename    string
		wantSuccess bool
		wantMsgPart string
	}{
		{
			name: "unexpected response shape",
			resp: map[string]interface{}{
				"something": "else",
			},
			filename:    "Weird.png",
			wantSuccess: false,
			wantMsgPart: "Unexpected response",
		},
		{
			name: "unknown status falls through to default",
			resp: map[string]interface{}{
				"upload": map[string]interface{}{
					"result": "Throttled",
				},
			},
			filename:    "Slow.png",
			wantSuccess: false,
			wantMsgPart: "Throttled",
		},
		{
			name: "success without imageinfo leaves url and size empty",
			resp: map[string]interface{}{
				"upload": map[string]interface{}{
					"result": "Success",
				},
			},
			filename:    "Bare.png",
			wantSuccess: true,
			wantMsgPart: "uploaded successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.parseUploadResponse(tt.resp, tt.filename)
			if err != nil {
				t.Fatalf("parseUploadResponse returned error: %v", err)
			}
			if result.Success != tt.wantSuccess {
				t.Errorf("Success = %v, want %v", result.Success, tt.wantSuccess)
			}
			if result.Filename != tt.filename {
				t.Errorf("Filename = %q, want %q", result.Filename, tt.filename)
			}
			if !strings.Contains(result.Message, tt.wantMsgPart) {
				t.Errorf("Message = %q, want substring %q", result.Message, tt.wantMsgPart)
			}
		})
	}
}

func TestParseJSONResponse_BodyHandling(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	t.Run("empty body", func(t *testing.T) {
		var target map[string]interface{}
		err := client.parseJSONResponse(newUploadBodyResponse(http.StatusOK, ""), &target)
		if err == nil {
			t.Fatal("expected error for empty body")
		}
		if !strings.Contains(err.Error(), "empty upload response") {
			t.Errorf("error should describe empty body, got: %v", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		var target map[string]interface{}
		err := client.parseJSONResponse(newUploadBodyResponse(http.StatusOK, "{not json"), &target)
		if err == nil {
			t.Fatal("expected error for invalid json")
		}
		if !strings.Contains(err.Error(), "decode upload response") {
			t.Errorf("error should describe decode failure, got: %v", err)
		}
	})

	t.Run("valid json decodes into target", func(t *testing.T) {
		var target map[string]interface{}
		body := `{"upload":{"result":"Success"}}`
		if err := client.parseJSONResponse(newUploadBodyResponse(http.StatusOK, body), &target); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		upload, ok := target["upload"].(map[string]interface{})
		if !ok {
			t.Fatalf("upload key missing or wrong type: %#v", target)
		}
		if upload["result"] != "Success" {
			t.Errorf("result = %v, want Success", upload["result"])
		}
	})
}
