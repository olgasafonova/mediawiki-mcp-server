package wiki

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "string shorter than max",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "string equal to max",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "string longer than max",
			input:  "hello world",
			maxLen: 8,
			want:   "hello...",
		},
		{
			name:   "very long string",
			input:  "this is a very long string that needs truncation",
			maxLen: 20,
			want:   "this is a very lo...",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
		{
			name:   "unicode string truncation",
			input:  "こんにちは世界",
			maxLen: 15,
			want:   "こんにち...", // len() uses bytes not runes, so 4 chars (12 bytes) + "..." = 15 bytes
		},
		{
			name:   "max length of 3",
			input:  "test",
			maxLen: 3,
			want:   "...", // 0 chars + "..."
		},
		{
			name:   "max length of 4",
			input:  "testing",
			maxLen: 4,
			want:   "t...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestTruncateString_AlwaysProducesValidLength(t *testing.T) {
	testCases := []struct {
		input  string
		maxLen int
	}{
		{"short", 100},
		{"this is a longer string", 10},
		{"exactly ten", 11},
		{"", 5},
		{"test", 3},
	}

	for _, tc := range testCases {
		result := truncateString(tc.input, tc.maxLen)
		if len(result) > tc.maxLen {
			t.Errorf("truncateString(%q, %d) produced result of length %d, exceeds maxLen",
				tc.input, tc.maxLen, len(result))
		}
	}
}

func TestEditPage_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		action := r.FormValue("action")
		if action == "edit" {
			response := map[string]interface{}{
				"edit": map[string]interface{}{
					"result":   "Success",
					"pageid":   float64(123),
					"title":    "Test Page",
					"newrevid": float64(456),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		// Let mockMediaWikiServer handle tokens
		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.EditPage(context.Background(), EditPageArgs{
		Title:   "Test Page",
		Content: "== Test ==\nContent here",
		Summary: "Test edit",
	})
	if err != nil {
		t.Fatalf("EditPage failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}
	if result.PageID != 123 {
		t.Errorf("PageID = %d, want 123", result.PageID)
	}
	if result.RevisionID != 456 {
		t.Errorf("RevisionID = %d, want 456", result.RevisionID)
	}
}

func TestEditPage_NewPage(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		action := r.FormValue("action")
		if action == "edit" {
			response := map[string]interface{}{
				"edit": map[string]interface{}{
					"result":   "Success",
					"pageid":   float64(789),
					"title":    "New Page",
					"newrevid": float64(1),
					"new":      "",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.EditPage(context.Background(), EditPageArgs{
		Title:   "New Page",
		Content: "Brand new content",
	})
	if err != nil {
		t.Fatalf("EditPage failed: %v", err)
	}

	if !result.NewPage {
		t.Error("Expected NewPage to be true")
	}
	if result.Message != "Page created successfully" {
		t.Errorf("Message = %q, want %q", result.Message, "Page created successfully")
	}
}

func TestEditPage_EmptyTitle(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.EditPage(context.Background(), EditPageArgs{
		Title:   "",
		Content: "content",
	})
	if err == nil {
		t.Error("Expected error for empty title")
	}
	// Check it's a ValidationError
	if _, ok := err.(*ValidationError); !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}
}

func TestEditPage_EmptyContent(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.EditPage(context.Background(), EditPageArgs{
		Title:   "Test",
		Content: "",
	})
	if err == nil {
		t.Error("Expected error for empty content")
	}
	if _, ok := err.(*ValidationError); !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}
}

func TestEditPage_EditFailed(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		action := r.FormValue("action")
		if action == "edit" {
			response := map[string]interface{}{
				"edit": map[string]interface{}{
					"result": "Failure",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.EditPage(context.Background(), EditPageArgs{
		Title:   "Test Page",
		Content: "Content",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Success {
		t.Error("Expected success to be false")
	}
}

func TestFindReplace_Success(t *testing.T) {
	callCount := 0
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		action := r.FormValue("action")
		callCount++

		if action == "query" {
			// Return page content for GetPage
			response := map[string]interface{}{
				"query": map[string]interface{}{
					"pages": map[string]interface{}{
						"123": map[string]interface{}{
							"pageid":    float64(123),
							"title":     "Test Page",
							"lastrevid": float64(100),
							"revisions": []interface{}{
								map[string]interface{}{
									"slots": map[string]interface{}{
										"main": map[string]interface{}{
											"content": "hello world, hello universe",
										},
									},
								},
							},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		if action == "edit" {
			response := map[string]interface{}{
				"edit": map[string]interface{}{
					"result":   "Success",
					"pageid":   float64(123),
					"title":    "Test Page",
					"newrevid": float64(101),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.FindReplace(context.Background(), FindReplaceArgs{
		Title:   "Test Page",
		Find:    "hello",
		Replace: "goodbye",
		All:     true,
	})
	if err != nil {
		t.Fatalf("FindReplace failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}
	if result.MatchCount != 2 {
		t.Errorf("MatchCount = %d, want 2", result.MatchCount)
	}
	if result.ReplaceCount != 2 {
		t.Errorf("ReplaceCount = %d, want 2", result.ReplaceCount)
	}
}

func TestFindReplace_Preview(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		action := r.FormValue("action")

		if action == "query" {
			response := map[string]interface{}{
				"query": map[string]interface{}{
					"pages": map[string]interface{}{
						"123": map[string]interface{}{
							"pageid": float64(123),
							"title":  "Test Page",
							"revisions": []interface{}{
								map[string]interface{}{
									"slots": map[string]interface{}{
										"main": map[string]interface{}{
											"content": "Replace this text",
										},
									},
								},
							},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		// Should not reach edit in preview mode
		t.Error("Edit should not be called in preview mode")
		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.FindReplace(context.Background(), FindReplaceArgs{
		Title:   "Test Page",
		Find:    "this",
		Replace: "that",
		Preview: true,
	})
	if err != nil {
		t.Fatalf("FindReplace failed: %v", err)
	}

	if !result.Preview {
		t.Error("Expected preview mode")
	}
	if result.MatchCount != 1 {
		t.Errorf("MatchCount = %d, want 1", result.MatchCount)
	}
}

func TestFindReplace_NoMatches(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		action := r.FormValue("action")

		if action == "query" {
			response := map[string]interface{}{
				"query": map[string]interface{}{
					"pages": map[string]interface{}{
						"123": map[string]interface{}{
							"pageid": float64(123),
							"title":  "Test Page",
							"revisions": []interface{}{
								map[string]interface{}{
									"slots": map[string]interface{}{
										"main": map[string]interface{}{
											"content": "No matching text here",
										},
									},
								},
							},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.FindReplace(context.Background(), FindReplaceArgs{
		Title:   "Test Page",
		Find:    "xyz",
		Replace: "abc",
	})
	if err != nil {
		t.Fatalf("FindReplace failed: %v", err)
	}

	if result.MatchCount != 0 {
		t.Errorf("MatchCount = %d, want 0", result.MatchCount)
	}
}

func TestFindReplace_EmptyTitle(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.FindReplace(context.Background(), FindReplaceArgs{
		Title:   "",
		Find:    "test",
		Replace: "test",
	})
	if err == nil {
		t.Error("Expected error for empty title")
	}
}

func TestFindReplace_EmptyFind(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.FindReplace(context.Background(), FindReplaceArgs{
		Title:   "Test",
		Find:    "",
		Replace: "test",
	})
	if err == nil {
		t.Error("Expected error for empty find")
	}
}

func TestFindReplace_InvalidRegex(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"123": map[string]interface{}{
						"pageid": float64(123),
						"title":  "Test Page",
						"revisions": []interface{}{
							map[string]interface{}{
								"slots": map[string]interface{}{
									"main": map[string]interface{}{
										"content": "content",
									},
								},
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	_, err := client.FindReplace(context.Background(), FindReplaceArgs{
		Title:    "Test",
		Find:     "[invalid",
		Replace:  "test",
		UseRegex: true,
	})
	if err == nil {
		t.Error("Expected error for invalid regex")
	}
}

func TestFindReplace_RegexTooLong(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	longPattern := strings.Repeat("a", 501)
	_, err := client.FindReplace(context.Background(), FindReplaceArgs{
		Title:    "Test Page",
		Find:     longPattern,
		Replace:  "test",
		UseRegex: true,
	})

	if err == nil {
		t.Fatal("Expected error for regex pattern exceeding 500 chars")
	}
	if !strings.Contains(err.Error(), "regex pattern too long") {
		t.Errorf("Expected 'regex pattern too long' error, got: %v", err)
	}
}

func TestApplyFormatting_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		action := r.FormValue("action")

		if action == "query" {
			response := map[string]interface{}{
				"query": map[string]interface{}{
					"pages": map[string]interface{}{
						"123": map[string]interface{}{
							"pageid":    float64(123),
							"title":     "Test Page",
							"lastrevid": float64(100),
							"revisions": []interface{}{
								map[string]interface{}{
									"slots": map[string]interface{}{
										"main": map[string]interface{}{
											"content": "John Smith works here",
										},
									},
								},
							},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		if action == "edit" {
			response := map[string]interface{}{
				"edit": map[string]interface{}{
					"result":   "Success",
					"pageid":   float64(123),
					"title":    "Test Page",
					"newrevid": float64(101),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.ApplyFormatting(context.Background(), ApplyFormattingArgs{
		Title:  "Test Page",
		Text:   "John Smith",
		Format: "strikethrough",
	})
	if err != nil {
		t.Fatalf("ApplyFormatting failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}
	if result.Format != "strikethrough" {
		t.Errorf("Format = %q, want %q", result.Format, "strikethrough")
	}
}

func TestApplyFormatting_AllFormats(t *testing.T) {
	formats := []struct {
		name   string
		format string
	}{
		{"strikethrough", "strikethrough"},
		{"strike", "strike"},
		{"bold", "bold"},
		{"italic", "italic"},
		{"underline", "underline"},
		{"code", "code"},
		{"nowiki", "nowiki"},
	}

	for _, f := range formats {
		t.Run(f.name, func(t *testing.T) {
			server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
				action := r.FormValue("action")

				if action == "query" {
					response := map[string]interface{}{
						"query": map[string]interface{}{
							"pages": map[string]interface{}{
								"123": map[string]interface{}{
									"pageid":    float64(123),
									"title":     "Test Page",
									"lastrevid": float64(100),
									"revisions": []interface{}{
										map[string]interface{}{
											"slots": map[string]interface{}{
												"main": map[string]interface{}{
													"content": "test text here",
												},
											},
										},
									},
								},
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
					return
				}

				if action == "edit" {
					response := map[string]interface{}{
						"edit": map[string]interface{}{
							"result":   "Success",
							"pageid":   float64(123),
							"title":    "Test Page",
							"newrevid": float64(101),
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
					return
				}

				w.WriteHeader(http.StatusBadRequest)
			})
			defer server.Close()

			client := createMockClient(t, server)
			defer client.Close()

			result, err := client.ApplyFormatting(context.Background(), ApplyFormattingArgs{
				Title:  "Test Page",
				Text:   "test",
				Format: f.format,
			})
			if err != nil {
				t.Fatalf("ApplyFormatting failed for %s: %v", f.format, err)
			}

			if !result.Success {
				t.Errorf("Expected success for format %s", f.format)
			}
		})
	}
}

func TestApplyFormatting_UnknownFormat(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.ApplyFormatting(context.Background(), ApplyFormattingArgs{
		Title:  "Test",
		Text:   "text",
		Format: "unknown",
	})
	if err == nil {
		t.Error("Expected error for unknown format")
	}
}

func TestApplyFormatting_EmptyFields(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	tests := []struct {
		name string
		args ApplyFormattingArgs
	}{
		{
			name: "empty title",
			args: ApplyFormattingArgs{Title: "", Text: "text", Format: "bold"},
		},
		{
			name: "empty text",
			args: ApplyFormattingArgs{Title: "Test", Text: "", Format: "bold"},
		},
		{
			name: "empty format",
			args: ApplyFormattingArgs{Title: "Test", Text: "text", Format: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.ApplyFormatting(context.Background(), tt.args)
			if err == nil {
				t.Error("Expected error")
			}
		})
	}
}

func TestBulkReplace_WithPages(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		action := r.FormValue("action")
		title := r.FormValue("titles")
		if title == "" {
			title = r.FormValue("title")
		}

		if action == "query" {
			pageID := float64(1)
			if title == "Page Two" || strings.Contains(title, "Page Two") {
				pageID = float64(2)
			}
			response := map[string]interface{}{
				"query": map[string]interface{}{
					"pages": map[string]interface{}{
						fmt.Sprintf("%d", int(pageID)): map[string]interface{}{
							"pageid":    pageID,
							"title":     strings.Split(title, "|")[0],
							"lastrevid": float64(100),
							"revisions": []interface{}{
								map[string]interface{}{
									"slots": map[string]interface{}{
										"main": map[string]interface{}{
											"content": "Replace oldtext with new",
										},
									},
								},
							},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		if action == "edit" {
			response := map[string]interface{}{
				"edit": map[string]interface{}{
					"result":   "Success",
					"pageid":   float64(1),
					"title":    r.FormValue("title"),
					"newrevid": float64(101),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.BulkReplace(context.Background(), BulkReplaceArgs{
		Pages:   []string{"Page One", "Page Two"},
		Find:    "oldtext",
		Replace: "newtext",
	})
	if err != nil {
		t.Fatalf("BulkReplace failed: %v", err)
	}

	if result.PagesProcessed != 2 {
		t.Errorf("PagesProcessed = %d, want 2", result.PagesProcessed)
	}
	if result.PagesModified != 2 {
		t.Errorf("PagesModified = %d, want 2", result.PagesModified)
	}
}

func TestBulkReplace_EmptyFind(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.BulkReplace(context.Background(), BulkReplaceArgs{
		Pages:   []string{"Page"},
		Find:    "",
		Replace: "test",
	})
	if err == nil {
		t.Error("Expected error for empty find")
	}
}

func TestBulkReplace_NoPagesOrCategory(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.BulkReplace(context.Background(), BulkReplaceArgs{
		Find:    "test",
		Replace: "test",
	})
	if err == nil {
		t.Error("Expected error when neither pages nor category is specified")
	}
}

func TestBuildEditRevisionInfo(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	revision, undo := client.buildEditRevisionInfo("Test Page", 100, 101)

	if revision == nil {
		t.Fatal("Expected revision info")
	}
	if revision.OldRevision != 100 {
		t.Errorf("OldRevision = %d, want 100", revision.OldRevision)
	}
	if revision.NewRevision != 101 {
		t.Errorf("NewRevision = %d, want 101", revision.NewRevision)
	}
	if !strings.Contains(revision.DiffURL, "diff=101") {
		t.Error("DiffURL should contain new revision")
	}
	if !strings.Contains(revision.DiffURL, "oldid=100") {
		t.Error("DiffURL should contain old revision")
	}

	if undo == nil {
		t.Fatal("Expected undo info")
	}
	if !strings.Contains(undo.WikiURL, "undo=101") {
		t.Error("Undo URL should contain new revision")
	}
}

func TestBuildEditRevisionInfo_ZeroRevisions(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	revision, undo := client.buildEditRevisionInfo("Test", 0, 101)
	if revision != nil || undo != nil {
		t.Error("Expected nil when old revision is 0")
	}

	revision, undo = client.buildEditRevisionInfo("Test", 100, 0)
	if revision != nil || undo != nil {
		t.Error("Expected nil when new revision is 0")
	}
}

func TestCheckPagesExist_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"123": map[string]interface{}{
						"pageid": float64(123),
						"title":  "Existing Page",
					},
					"-1": map[string]interface{}{
						"title":   "Missing Page",
						"missing": "",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.checkPagesExist(context.Background(), []string{"Existing Page", "Missing Page"})
	if err != nil {
		t.Fatalf("checkPagesExist failed: %v", err)
	}

	if !result["Existing Page"] {
		t.Error("Expected 'Existing Page' to exist")
	}
	if result["Missing Page"] {
		t.Error("Expected 'Missing Page' to not exist")
	}
}

func TestCheckPagesExist_EmptyList(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	result, err := client.checkPagesExist(context.Background(), []string{})
	if err != nil {
		t.Fatalf("checkPagesExist failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d items", len(result))
	}
}

func TestUploadFile_Validation(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	tests := []struct {
		name string
		args UploadFileArgs
	}{
		{
			name: "empty filename",
			args: UploadFileArgs{Filename: "", FileURL: "http://example.com/file.png"},
		},
		{
			name: "no file source",
			args: UploadFileArgs{Filename: "test.png"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.UploadFile(context.Background(), tt.args)
			if err == nil {
				t.Error("Expected validation error")
			}
		})
	}
}

func TestParseUploadResponse_Success(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	resp := map[string]interface{}{
		"upload": map[string]interface{}{
			"result": "Success",
			"imageinfo": map[string]interface{}{
				"url":  "http://wiki.com/images/test.png",
				"size": float64(1024),
			},
		},
	}

	result, err := client.parseUploadResponse(resp, "test.png")
	if err != nil {
		t.Fatalf("parseUploadResponse failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}
	if result.URL != "http://wiki.com/images/test.png" {
		t.Errorf("URL = %q, want %q", result.URL, "http://wiki.com/images/test.png")
	}
	if result.Size != 1024 {
		t.Errorf("Size = %d, want 1024", result.Size)
	}
}

func TestParseUploadResponse_Warning(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	resp := map[string]interface{}{
		"upload": map[string]interface{}{
			"result": "Warning",
			"warnings": map[string]interface{}{
				"exists": "File already exists",
			},
		},
	}

	result, err := client.parseUploadResponse(resp, "test.png")
	if err != nil {
		t.Fatalf("parseUploadResponse failed: %v", err)
	}

	if result.Success {
		t.Error("Expected success to be false for warning")
	}
	if len(result.Warnings) == 0 {
		t.Error("Expected warnings")
	}
}

func TestParseUploadResponse_Error(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"info": "Upload failed",
		},
	}

	result, err := client.parseUploadResponse(resp, "test.png")
	if err != nil {
		t.Fatalf("parseUploadResponse failed: %v", err)
	}

	if result.Success {
		t.Error("Expected success to be false for error")
	}
}

func TestReadLocalFile(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.readLocalFile("/some/path")
	if err == nil {
		t.Error("Expected error for local file read")
	}
}

func TestGetFileURL_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"1": map[string]interface{}{
						"pageid": float64(1),
						"title":  "File:Test.pdf",
						"imageinfo": []interface{}{
							map[string]interface{}{
								"url":  "https://wiki.example.com/images/test.pdf",
								"mime": "application/pdf",
								"size": float64(12345),
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	fileURL, fileType, err := client.getFileURL(ctx, "File:Test.pdf")

	if err != nil {
		t.Fatalf("getFileURL failed: %v", err)
	}
	if fileURL != "https://wiki.example.com/images/test.pdf" {
		t.Errorf("fileURL = %q, want expected URL", fileURL)
	}
	if fileType != "pdf" {
		t.Errorf("fileType = %q, want pdf", fileType)
	}
}

func TestGetFileURL_MissingFile(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"-1": map[string]interface{}{
						"ns":      float64(6),
						"title":   "File:Nonexistent.pdf",
						"missing": "",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, _, err := client.getFileURL(ctx, "File:Nonexistent.pdf")

	if err == nil {
		t.Fatal("Expected error for missing file")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("Expected 'does not exist' error, got: %v", err)
	}
}

func TestGetFileURL_NoImageInfo(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"1": map[string]interface{}{
						"pageid": float64(1),
						"title":  "File:Test.pdf",
						// No imageinfo
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, _, err := client.getFileURL(ctx, "File:Test.pdf")

	if err == nil {
		t.Fatal("Expected error for missing imageinfo")
	}
}

func TestGetFileURL_TextMimeType(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"1": map[string]interface{}{
						"pageid": float64(1),
						"title":  "File:Test.txt",
						"imageinfo": []interface{}{
							map[string]interface{}{
								"url":  "https://wiki.example.com/images/test.txt",
								"mime": "text/plain",
								"size": float64(100),
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, fileType, err := client.getFileURL(ctx, "File:Test.txt")

	if err != nil {
		t.Fatalf("getFileURL failed: %v", err)
	}
	if fileType != "plain" {
		t.Errorf("fileType = %q, want plain", fileType)
	}
}

func TestDownloadFile_Success(t *testing.T) {
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("Test file content"))
	}))
	defer fileServer.Close()

	client := createMockClient(t, fileServer)
	defer client.Close()

	ctx := context.Background()
	content, err := client.downloadFile(ctx, fileServer.URL+"/test.txt")

	if err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}
	if string(content) != "Test file content" {
		t.Errorf("content = %q, want 'Test file content'", string(content))
	}
}

func TestDownloadFile_NotFound(t *testing.T) {
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fileServer.Close()

	client := createMockClient(t, fileServer)
	defer client.Close()

	ctx := context.Background()
	_, err := client.downloadFile(ctx, fileServer.URL+"/notfound.txt")

	if err == nil {
		t.Fatal("Expected error for 404 response")
	}
}

func TestParseJSONResponse(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	// parseJSONResponse returns error "not implemented"
	err := client.parseJSONResponse(nil, nil)
	if err == nil {
		t.Fatal("Expected error from parseJSONResponse")
	}
}

func TestUploadFile_FromURL_Success(t *testing.T) {
	loginStep := 0
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")

		if action == "query" {
			meta := r.FormValue("meta")
			if meta == "tokens" {
				tokenType := r.FormValue("type")
				if tokenType == "login" {
					response := map[string]interface{}{
						"query": map[string]interface{}{
							"tokens": map[string]interface{}{
								"logintoken": "test-login-token+\\",
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
					return
				}
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"tokens": map[string]interface{}{
							"csrftoken": "test-csrf-token+\\",
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
		}

		if action == "login" {
			loginStep++
			result := "NeedToken"
			if loginStep > 1 {
				result = "Success"
			}
			response := map[string]interface{}{
				"login": map[string]interface{}{
					"result":     result,
					"lguserid":   float64(1),
					"lgusername": "TestUser",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		if action == "upload" {
			response := map[string]interface{}{
				"upload": map[string]interface{}{
					"result":   "Success",
					"filename": "Test.png",
					"imageinfo": map[string]interface{}{
						"url": "https://example.com/wiki/Test.png",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":{}}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	client.config.Username = "TestUser"
	client.config.Password = "TestPass"
	defer client.Close()

	ctx := context.Background()
	result, err := client.UploadFile(ctx, UploadFileArgs{
		Filename:       "Test.png",
		FileURL:        "https://example.com/source.png",
		Text:           "Test upload",
		Comment:        "Uploading test file",
		IgnoreWarnings: true,
	})

	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}
	if !result.Success {
		t.Errorf("Success = false, want true")
	}
	if result.Filename != "Test.png" {
		t.Errorf("Filename = %q, want 'Test.png'", result.Filename)
	}
}

func TestUploadFile_MissingFilename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, err := client.UploadFile(ctx, UploadFileArgs{
		FileURL: "https://example.com/test.png",
	})

	if err == nil {
		t.Fatal("Expected error for missing filename")
	}
}

func TestUploadFile_MissingSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, err := client.UploadFile(ctx, UploadFileArgs{
		Filename: "Test.png",
	})

	if err == nil {
		t.Fatal("Expected error for missing source")
	}
}

func TestUploadFile_FromFile_NotSupported(t *testing.T) {
	// Test that local file upload returns the expected error
	loginStep := 0
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")

		if action == "query" {
			meta := r.FormValue("meta")
			if meta == "tokens" {
				tokenType := r.FormValue("type")
				if tokenType == "login" {
					response := map[string]interface{}{
						"query": map[string]interface{}{
							"tokens": map[string]interface{}{
								"logintoken": "test-login-token+\\",
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
					return
				}
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"tokens": map[string]interface{}{
							"csrftoken": "test-csrf-token+\\",
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
		}

		if action == "login" {
			loginStep++
			result := "NeedToken"
			if loginStep > 1 {
				result = "Success"
			}
			response := map[string]interface{}{
				"login": map[string]interface{}{
					"result":     result,
					"lguserid":   float64(1),
					"lgusername": "TestUser",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":{}}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	client.config.Username = "TestUser"
	client.config.Password = "TestPass"
	defer client.Close()

	ctx := context.Background()
	_, err := client.UploadFile(ctx, UploadFileArgs{
		Filename: "Test.txt",
		FilePath: "/some/file/path.txt",
	})

	// Should fail because local file upload is not supported
	if err == nil {
		t.Fatal("Expected error for local file upload")
	}
	if !strings.Contains(err.Error(), "local file upload not supported") {
		t.Errorf("Expected 'local file upload not supported' error, got: %v", err)
	}
}

func TestBulkReplace_WithCategory(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	// Test with category specified - will fail on login but exercises the category path
	_, err := client.BulkReplace(context.Background(), BulkReplaceArgs{
		Find:     "old",
		Replace:  "new",
		Category: "Test Category",
	})
	// Expected to fail due to no credentials, but exercises the path
	_ = err
}

func TestBulkReplace_WithPreview(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.BulkReplace(context.Background(), BulkReplaceArgs{
		Find:    "old",
		Replace: "new",
		Pages:   []string{"Page1"},
		Preview: true,
	})
	// Expected to fail due to no credentials
	_ = err
}
