package wiki

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// createMockClient creates a client that talks to a mock server
func createMockClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	config := &Config{
		BaseURL:    server.URL,
		Username:   "TestUser",
		Password:   "TestPass",
		Timeout:    5 * time.Second,
		MaxRetries: 1,
		UserAgent:  "TestClient/1.0",
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewClient(config, logger)
}

// mockMediaWikiServer creates a test server that returns mock MediaWiki responses
// It automatically handles login/auth requests and delegates to handler for other requests
func mockMediaWikiServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")
		meta := r.FormValue("meta")

		// Handle userinfo query (session check)
		if action == "query" && meta == "userinfo" {
			response := map[string]interface{}{
				"query": map[string]interface{}{
					"userinfo": map[string]interface{}{
						"id":   float64(1),
						"name": "TestUser",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		// Handle login token request
		if action == "query" && (meta == "tokens") {
			tokenType := r.FormValue("type")
			response := map[string]interface{}{
				"query": map[string]interface{}{
					"tokens": map[string]interface{}{},
				},
			}
			tokens := response["query"].(map[string]interface{})["tokens"].(map[string]interface{})
			if tokenType == "login" {
				tokens["logintoken"] = "test-login-token"
			} else if tokenType == "csrf" {
				tokens["csrftoken"] = "test-csrf-token"
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		// Handle login action
		if action == "login" {
			response := map[string]interface{}{
				"login": map[string]interface{}{
					"result": "Success",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		// Delegate to custom handler
		handler(w, r)
	}))
}

func TestSearch_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"searchinfo": map[string]interface{}{
					"totalhits": float64(2),
				},
				"search": []interface{}{
					map[string]interface{}{
						"pageid":  float64(1),
						"title":   "Test Page",
						"snippet": "<b>Test</b> content",
						"size":    float64(100),
					},
					map[string]interface{}{
						"pageid":  float64(2),
						"title":   "Another Page",
						"snippet": "More <b>content</b>",
						"size":    float64(200),
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

	result, err := client.Search(context.Background(), SearchArgs{
		Query: "test",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if result.TotalHits != 2 {
		t.Errorf("TotalHits = %d, want 2", result.TotalHits)
	}
	if len(result.Results) != 2 {
		t.Errorf("len(Results) = %d, want 2", len(result.Results))
	}
	if result.Results[0].Title != "Test Page" {
		t.Errorf("Results[0].Title = %q, want %q", result.Results[0].Title, "Test Page")
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.Search(context.Background(), SearchArgs{
		Query: "",
	})
	if err == nil {
		t.Error("Expected error for empty query")
	}
}

func TestListPages_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"allpages": []interface{}{
					map[string]interface{}{
						"pageid": float64(1),
						"title":  "Page One",
					},
					map[string]interface{}{
						"pageid": float64(2),
						"title":  "Page Two",
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

	result, err := client.ListPages(context.Background(), ListPagesArgs{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListPages failed: %v", err)
	}

	if len(result.Pages) != 2 {
		t.Errorf("len(Pages) = %d, want 2", len(result.Pages))
	}
	if result.Pages[0].Title != "Page One" {
		t.Errorf("Pages[0].Title = %q, want %q", result.Pages[0].Title, "Page One")
	}
}

func TestListPages_WithContinuation(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"allpages": []interface{}{
					map[string]interface{}{
						"pageid": float64(1),
						"title":  "Page One",
					},
				},
			},
			"continue": map[string]interface{}{
				"apcontinue": "Page Two",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.ListPages(context.Background(), ListPagesArgs{
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("ListPages failed: %v", err)
	}

	if !result.HasMore {
		t.Error("Expected HasMore to be true")
	}
	if result.ContinueFrom != "Page Two" {
		t.Errorf("ContinueFrom = %q, want %q", result.ContinueFrom, "Page Two")
	}
}

func TestListCategories_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"allcategories": []interface{}{
					map[string]interface{}{
						"*":    "Category:Test",
						"size": float64(10),
					},
					map[string]interface{}{
						"*":    "Category:Another",
						"size": float64(5),
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

	result, err := client.ListCategories(context.Background(), ListCategoriesArgs{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListCategories failed: %v", err)
	}

	if len(result.Categories) != 2 {
		t.Errorf("len(Categories) = %d, want 2", len(result.Categories))
	}
}

func TestGetCategoryMembers_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"categorymembers": []interface{}{
					map[string]interface{}{
						"pageid": float64(1),
						"title":  "Member Page",
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

	result, err := client.GetCategoryMembers(context.Background(), CategoryMembersArgs{
		Category: "Test",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("GetCategoryMembers failed: %v", err)
	}

	if len(result.Members) != 1 {
		t.Errorf("len(Members) = %d, want 1", len(result.Members))
	}
	if result.Category != "Category:Test" {
		t.Errorf("Category = %q, want %q", result.Category, "Category:Test")
	}
}

func TestGetCategoryMembers_EmptyCategory(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.GetCategoryMembers(context.Background(), CategoryMembersArgs{
		Category: "",
	})
	if err == nil {
		t.Error("Expected error for empty category")
	}
}

func TestAPIRequest_Error(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"code": "badquery",
				"info": "Invalid query",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	_, err := client.Search(context.Background(), SearchArgs{
		Query: "test",
	})
	if err == nil {
		t.Error("Expected error from API")
	}
	if err.Error() != "API error [badquery]: Invalid query" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestAPIRequest_HTTPError(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	_, err := client.Search(context.Background(), SearchArgs{
		Query: "test",
	})
	if err == nil {
		t.Error("Expected error from server error")
	}
}

func TestAPIRequest_InvalidJSON(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	_, err := client.Search(context.Background(), SearchArgs{
		Query: "test",
	})
	if err == nil {
		t.Error("Expected error from invalid JSON")
	}
}

func TestAPIRequest_ContextCancellation(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Slow response
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.Search(ctx, SearchArgs{
		Query: "test",
	})
	if err == nil {
		t.Error("Expected error from context timeout")
	}
}

func TestAPIRequest_ClientError(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Bad Request"))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	_, err := client.Search(context.Background(), SearchArgs{
		Query: "test",
	})
	if err == nil {
		t.Error("Expected error from client error status")
	}
}

// Test malformed API responses
func TestSearch_MalformedResponse(t *testing.T) {
	tests := []struct {
		name     string
		response map[string]interface{}
	}{
		{
			name:     "missing query",
			response: map[string]interface{}{},
		},
		{
			name: "query not a map",
			response: map[string]interface{}{
				"query": "not a map",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(tt.response)
			})
			defer server.Close()

			client := createMockClient(t, server)
			defer client.Close()

			_, err := client.Search(context.Background(), SearchArgs{
				Query: "test",
			})
			if err == nil {
				t.Error("Expected error from malformed response")
			}
		})
	}
}

func TestGetPage_Success(t *testing.T) {
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
										"*":       "== Test ==\nContent here",
										"content": "== Test ==\nContent here",
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

	result, err := client.GetPage(context.Background(), GetPageArgs{
		Title:  "Test Page",
		Format: "wikitext",
	})
	if err != nil {
		t.Fatalf("GetPage failed: %v", err)
	}

	if result.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", result.Title, "Test Page")
	}
	if result.PageID != 123 {
		t.Errorf("PageID = %d, want 123", result.PageID)
	}
}

func TestGetPage_NotFound(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"-1": map[string]interface{}{
						"missing": "",
						"title":   "NonExistent Page",
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

	_, err := client.GetPage(context.Background(), GetPageArgs{
		Title: "NonExistent Page",
	})
	if err == nil {
		t.Error("Expected error for missing page")
	}
}

func TestGetPage_EmptyTitle(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.GetPage(context.Background(), GetPageArgs{
		Title: "",
	})
	if err == nil {
		t.Error("Expected error for empty title")
	}
}

func TestGetWikiInfo_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"general": map[string]interface{}{
					"sitename":       "Test Wiki",
					"mainpage":       "Main Page",
					"generator":      "MediaWiki 1.39.0",
					"phpversion":     "8.1.0",
					"dbtype":         "mysql",
					"dbversion":      "8.0.30",
					"lang":           "en",
					"langconversion": false,
					"server":         "https://test.wiki.com",
					"servername":     "test.wiki.com",
					"scriptpath":     "/w",
					"articlepath":    "/wiki/$1",
					"time":           "2024-01-15T12:00:00Z",
				},
				"statistics": map[string]interface{}{
					"pages":       float64(1000),
					"articles":    float64(500),
					"edits":       float64(5000),
					"users":       float64(100),
					"activeusers": float64(50),
					"admins":      float64(5),
				},
				"namespaces": map[string]interface{}{
					"0": map[string]interface{}{
						"id":        float64(0),
						"*":         "",
						"name":      "",
						"canonical": "",
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

	result, err := client.GetWikiInfo(context.Background(), WikiInfoArgs{})
	if err != nil {
		t.Fatalf("GetWikiInfo failed: %v", err)
	}

	if result.SiteName != "Test Wiki" {
		t.Errorf("SiteName = %q, want %q", result.SiteName, "Test Wiki")
	}
	if result.Generator != "MediaWiki 1.39.0" {
		t.Errorf("Generator = %q, want %q", result.Generator, "MediaWiki 1.39.0")
	}
	if result.Statistics.Pages != 1000 {
		t.Errorf("Statistics.Pages = %d, want 1000", result.Statistics.Pages)
	}
}

func TestGetPageInfo_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"123": map[string]interface{}{
						"pageid":       float64(123),
						"title":        "Test Page",
						"ns":           float64(0),
						"touched":      "2024-01-15T12:00:00Z",
						"lastrevid":    float64(456),
						"length":       float64(1000),
						"contentmodel": "wikitext",
						"pagelanguage": "en",
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

	result, err := client.GetPageInfo(context.Background(), PageInfoArgs{
		Title: "Test Page",
	})
	if err != nil {
		t.Fatalf("GetPageInfo failed: %v", err)
	}

	if result.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", result.Title, "Test Page")
	}
	if result.PageID != 123 {
		t.Errorf("PageID = %d, want 123", result.PageID)
	}
	if result.Length != 1000 {
		t.Errorf("Length = %d, want 1000", result.Length)
	}
}

func TestGetPageInfo_EmptyTitle(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.GetPageInfo(context.Background(), PageInfoArgs{
		Title: "",
	})
	if err == nil {
		t.Error("Expected error for empty title")
	}
}

func TestGetRecentChanges_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"recentchanges": []interface{}{
					map[string]interface{}{
						"type":      "edit",
						"title":     "Test Page",
						"pageid":    float64(123),
						"revid":     float64(456),
						"old_revid": float64(455),
						"user":      "TestUser",
						"timestamp": "2024-01-15T12:00:00Z",
						"comment":   "Test edit",
					},
					map[string]interface{}{
						"type":      "new",
						"title":     "New Page",
						"pageid":    float64(124),
						"revid":     float64(457),
						"user":      "AnotherUser",
						"timestamp": "2024-01-15T11:00:00Z",
						"comment":   "Created page",
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

	result, err := client.GetRecentChanges(context.Background(), RecentChangesArgs{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("GetRecentChanges failed: %v", err)
	}

	if len(result.Changes) != 2 {
		t.Errorf("len(Changes) = %d, want 2", len(result.Changes))
	}
	if result.Changes[0].Title != "Test Page" {
		t.Errorf("Changes[0].Title = %q, want %q", result.Changes[0].Title, "Test Page")
	}
	if result.Changes[0].User != "TestUser" {
		t.Errorf("Changes[0].User = %q, want %q", result.Changes[0].User, "TestUser")
	}
}
