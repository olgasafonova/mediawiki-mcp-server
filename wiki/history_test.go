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

func TestAggregateChanges(t *testing.T) {
	tests := []struct {
		name       string
		changes    []RecentChange
		by         string
		wantNil    bool
		wantTotal  int
		wantBy     string
		checkItems func(items []AggregateCount) bool
	}{
		{
			name: "aggregates by user",
			changes: []RecentChange{
				{User: "Alice", Title: "Page1", Type: "edit"},
				{User: "Bob", Title: "Page2", Type: "edit"},
				{User: "Alice", Title: "Page3", Type: "edit"},
				{User: "Alice", Title: "Page4", Type: "new"},
			},
			by:        "user",
			wantNil:   false,
			wantTotal: 4,
			wantBy:    "user",
			checkItems: func(items []AggregateCount) bool {
				// Alice should be first with 3 edits
				if len(items) < 2 {
					return false
				}
				// Items should be sorted by count descending
				return items[0].Key == "Alice" && items[0].Count == 3
			},
		},
		{
			name: "aggregates by page",
			changes: []RecentChange{
				{User: "Alice", Title: "Main Page", Type: "edit"},
				{User: "Bob", Title: "Main Page", Type: "edit"},
				{User: "Carol", Title: "Other Page", Type: "edit"},
			},
			by:        "page",
			wantNil:   false,
			wantTotal: 3,
			wantBy:    "page",
			checkItems: func(items []AggregateCount) bool {
				return items[0].Key == "Main Page" && items[0].Count == 2
			},
		},
		{
			name: "aggregates by type",
			changes: []RecentChange{
				{User: "Alice", Title: "Page1", Type: "edit"},
				{User: "Bob", Title: "Page2", Type: "new"},
				{User: "Carol", Title: "Page3", Type: "edit"},
				{User: "Dave", Title: "Page4", Type: "edit"},
			},
			by:        "type",
			wantNil:   false,
			wantTotal: 4,
			wantBy:    "type",
			checkItems: func(items []AggregateCount) bool {
				return items[0].Key == "edit" && items[0].Count == 3
			},
		},
		{
			name: "returns nil for invalid aggregation type",
			changes: []RecentChange{
				{User: "Alice", Title: "Page1", Type: "edit"},
			},
			by:      "invalid",
			wantNil: true,
		},
		{
			name:       "handles empty changes",
			changes:    []RecentChange{},
			by:         "user",
			wantNil:    false,
			wantTotal:  0,
			wantBy:     "user",
			checkItems: func(items []AggregateCount) bool { return len(items) == 0 },
		},
		{
			name: "handles single change",
			changes: []RecentChange{
				{User: "Alice", Title: "Page1", Type: "edit"},
			},
			by:        "user",
			wantNil:   false,
			wantTotal: 1,
			wantBy:    "user",
			checkItems: func(items []AggregateCount) bool {
				return len(items) == 1 && items[0].Key == "Alice" && items[0].Count == 1
			},
		},
		{
			name: "sorts by count descending",
			changes: []RecentChange{
				{User: "Alice", Title: "Page1", Type: "edit"},
				{User: "Bob", Title: "Page2", Type: "edit"},
				{User: "Bob", Title: "Page3", Type: "edit"},
				{User: "Carol", Title: "Page4", Type: "edit"},
				{User: "Carol", Title: "Page5", Type: "edit"},
				{User: "Carol", Title: "Page6", Type: "edit"},
			},
			by:        "user",
			wantNil:   false,
			wantTotal: 6,
			wantBy:    "user",
			checkItems: func(items []AggregateCount) bool {
				if len(items) != 3 {
					return false
				}
				// Carol (3) > Bob (2) > Alice (1)
				return items[0].Count >= items[1].Count && items[1].Count >= items[2].Count
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := aggregateChanges(tt.changes, tt.by)

			if tt.wantNil {
				if result != nil {
					t.Errorf("aggregateChanges() = %v, want nil", result)
				}
				return
			}

			if result == nil {
				t.Fatalf("aggregateChanges() = nil, want non-nil")
			}

			if result.TotalChanges != tt.wantTotal {
				t.Errorf("TotalChanges = %d, want %d", result.TotalChanges, tt.wantTotal)
			}

			if result.By != tt.wantBy {
				t.Errorf("By = %q, want %q", result.By, tt.wantBy)
			}

			if tt.checkItems != nil && !tt.checkItems(result.Items) {
				t.Errorf("Items check failed: %+v", result.Items)
			}
		})
	}
}

// Helper function to create a mock server for history tests
func createHistoryMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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
			switch tokenType {
			case "login":
				tokens["logintoken"] = "test-login-token"
			case "csrf":
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

func createHistoryTestClient(t *testing.T, server *httptest.Server) *Client {
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

func TestGetRevisions_Success(t *testing.T) {
	server := createHistoryMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"123": map[string]interface{}{
						"pageid": float64(123),
						"title":  "Test Page",
						"revisions": []interface{}{
							map[string]interface{}{
								"revid":     float64(456),
								"parentid":  float64(455),
								"user":      "TestUser",
								"timestamp": "2024-01-15T12:00:00Z",
								"size":      float64(1000),
								"comment":   "Updated content",
							},
							map[string]interface{}{
								"revid":     float64(455),
								"parentid":  float64(454),
								"user":      "AnotherUser",
								"timestamp": "2024-01-14T12:00:00Z",
								"size":      float64(900),
								"comment":   "Previous edit",
								"minor":     "",
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

	client := createHistoryTestClient(t, server)
	defer client.Close()

	result, err := client.GetRevisions(context.Background(), GetRevisionsArgs{
		Title: "Test Page",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("GetRevisions failed: %v", err)
	}

	if result.PageID != 123 {
		t.Errorf("PageID = %d, want 123", result.PageID)
	}
	if len(result.Revisions) != 2 {
		t.Errorf("Expected 2 revisions, got %d", len(result.Revisions))
	}
	if result.Revisions[0].RevID != 456 {
		t.Errorf("First revision ID = %d, want 456", result.Revisions[0].RevID)
	}
}

func TestGetRevisions_EmptyTitle(t *testing.T) {
	config := &Config{
		BaseURL:    "https://test.wiki.com/api.php",
		Timeout:    5 * time.Second,
		MaxRetries: 1,
		UserAgent:  "TestClient/1.0",
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(config, logger)
	defer client.Close()

	_, err := client.GetRevisions(context.Background(), GetRevisionsArgs{
		Title: "",
	})
	if err == nil {
		t.Error("Expected error for empty title")
	}
}

func TestGetUserContributions_Success(t *testing.T) {
	server := createHistoryMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"usercontribs": []interface{}{
					map[string]interface{}{
						"pageid":    float64(101),
						"title":     "Article One",
						"ns":        float64(0),
						"revid":     float64(501),
						"parentid":  float64(500),
						"timestamp": "2024-01-15T12:00:00Z",
						"comment":   "Fixed typo",
						"size":      float64(5000),
						"sizediff":  float64(50),
						"minor":     "",
					},
					map[string]interface{}{
						"pageid":    float64(102),
						"title":     "Article Two",
						"ns":        float64(0),
						"revid":     float64(502),
						"parentid":  float64(0),
						"timestamp": "2024-01-14T12:00:00Z",
						"comment":   "Created page",
						"size":      float64(3000),
						"sizediff":  float64(3000),
						"new":       "",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createHistoryTestClient(t, server)
	defer client.Close()

	result, err := client.GetUserContributions(context.Background(), GetUserContributionsArgs{
		User:  "TestUser",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("GetUserContributions failed: %v", err)
	}

	if result.User != "TestUser" {
		t.Errorf("User = %q, want 'TestUser'", result.User)
	}
	if len(result.Contributions) != 2 {
		t.Errorf("Expected 2 contributions, got %d", len(result.Contributions))
	}
	if result.Contributions[0].Title != "Article One" {
		t.Errorf("First contribution title = %q, want 'Article One'", result.Contributions[0].Title)
	}
	if !result.Contributions[0].Minor {
		t.Error("First contribution should be minor")
	}
	if !result.Contributions[1].New {
		t.Error("Second contribution should be new")
	}
}

func TestGetUserContributions_EmptyUser(t *testing.T) {
	config := &Config{
		BaseURL:    "https://test.wiki.com/api.php",
		Timeout:    5 * time.Second,
		MaxRetries: 1,
		UserAgent:  "TestClient/1.0",
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(config, logger)
	defer client.Close()

	_, err := client.GetUserContributions(context.Background(), GetUserContributionsArgs{
		User: "",
	})
	if err == nil {
		t.Error("Expected error for empty user")
	}
}

func TestGetUserContributions_Continuation(t *testing.T) {
	server := createHistoryMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"usercontribs": []interface{}{
					map[string]interface{}{
						"pageid":    float64(101),
						"title":     "Article",
						"revid":     float64(501),
						"timestamp": "2024-01-15T12:00:00Z",
					},
				},
			},
			"continue": map[string]interface{}{
				"uccontinue": "2024-01-14T00:00:00Z|500",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createHistoryTestClient(t, server)
	defer client.Close()

	result, err := client.GetUserContributions(context.Background(), GetUserContributionsArgs{
		User: "TestUser",
	})
	if err != nil {
		t.Fatalf("GetUserContributions failed: %v", err)
	}

	if !result.HasMore {
		t.Error("Expected HasMore to be true")
	}
}

func TestCompareRevisions_Success(t *testing.T) {
	server := createHistoryMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"compare": map[string]interface{}{
				"fromtitle":     "Test Page",
				"fromrevid":     float64(455),
				"totitle":       "Test Page",
				"torevid":       float64(456),
				"*":             "<tr><td>-removed</td><td>+added</td></tr>",
				"fromuser":      "OldUser",
				"touser":        "NewUser",
				"fromtimestamp": "2024-01-14T12:00:00Z",
				"totimestamp":   "2024-01-15T12:00:00Z",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createHistoryTestClient(t, server)
	defer client.Close()

	result, err := client.CompareRevisions(context.Background(), CompareRevisionsArgs{
		FromRev: 455,
		ToRev:   456,
	})
	if err != nil {
		t.Fatalf("CompareRevisions failed: %v", err)
	}

	if result.FromRevID != 455 {
		t.Errorf("FromRevID = %d, want 455", result.FromRevID)
	}
	if result.ToRevID != 456 {
		t.Errorf("ToRevID = %d, want 456", result.ToRevID)
	}
	if result.Diff == "" {
		t.Error("Expected non-empty diff")
	}
	if result.FromUser != "OldUser" {
		t.Errorf("FromUser = %q, want 'OldUser'", result.FromUser)
	}
}

func TestCompareRevisions_MissingFromRev(t *testing.T) {
	config := &Config{
		BaseURL:    "https://test.wiki.com/api.php",
		Timeout:    5 * time.Second,
		MaxRetries: 1,
		UserAgent:  "TestClient/1.0",
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(config, logger)
	defer client.Close()

	_, err := client.CompareRevisions(context.Background(), CompareRevisionsArgs{
		ToRev: 456,
	})
	if err == nil {
		t.Error("Expected error for missing from_rev/from_title")
	}
}

func TestCompareRevisions_MissingToRev(t *testing.T) {
	config := &Config{
		BaseURL:    "https://test.wiki.com/api.php",
		Timeout:    5 * time.Second,
		MaxRetries: 1,
		UserAgent:  "TestClient/1.0",
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(config, logger)
	defer client.Close()

	_, err := client.CompareRevisions(context.Background(), CompareRevisionsArgs{
		FromRev: 455,
	})
	if err == nil {
		t.Error("Expected error for missing to_rev/to_title")
	}
}

func TestGetRevisions_WithAllOptions(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"1": map[string]interface{}{
						"pageid":    float64(1),
						"title":     "Test Page",
						"revisions": []interface{}{},
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

	_, err := client.GetRevisions(context.Background(), GetRevisionsArgs{
		Title: "Test Page",
		Limit: 5,
		User:  "TestUser",
		Start: "2024-01-01T00:00:00Z",
		End:   "2024-12-31T23:59:59Z",
	})
	if err != nil {
		t.Fatalf("GetRevisions failed: %v", err)
	}
}

func TestGetUserContributions_WithOptions(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"usercontribs": []interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	_, err := client.GetUserContributions(context.Background(), GetUserContributionsArgs{
		User:      "TestUser",
		Limit:     10,
		Namespace: 0,
		Start:     "2024-01-01T00:00:00Z",
		End:       "2024-12-31T23:59:59Z",
	})
	if err != nil {
		t.Fatalf("GetUserContributions failed: %v", err)
	}
}
