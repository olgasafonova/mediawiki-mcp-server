package wiki

import (
	"bytes"
	"context"
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
