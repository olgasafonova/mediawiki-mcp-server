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

// Helper function to create a mock server for category tests
func createCategoryMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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

func createCategoryTestClient(t *testing.T, server *httptest.Server) *Client {
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

func TestListCategories_WithPrefix(t *testing.T) {
	server := createCategoryMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		prefix := r.FormValue("acprefix")

		// Verify prefix is set
		if prefix != "Test" {
			t.Errorf("Expected prefix 'Test', got %q", prefix)
		}

		response := map[string]interface{}{
			"query": map[string]interface{}{
				"allcategories": []interface{}{
					map[string]interface{}{
						"*":    "Test Category",
						"size": float64(5),
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createCategoryTestClient(t, server)
	defer client.Close()

	result, err := client.ListCategories(context.Background(), ListCategoriesArgs{
		Prefix: "Test",
	})
	if err != nil {
		t.Fatalf("ListCategories failed: %v", err)
	}

	if len(result.Categories) != 1 {
		t.Errorf("Expected 1 category, got %d", len(result.Categories))
	}
}

func TestListCategories_Continuation(t *testing.T) {
	server := createCategoryMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"allcategories": []interface{}{
					map[string]interface{}{
						"*":    "Category",
						"size": float64(1),
					},
				},
			},
			"continue": map[string]interface{}{
				"accontinue": "next-page-token",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createCategoryTestClient(t, server)
	defer client.Close()

	result, err := client.ListCategories(context.Background(), ListCategoriesArgs{})
	if err != nil {
		t.Fatalf("ListCategories failed: %v", err)
	}

	if !result.HasMore {
		t.Error("Expected HasMore to be true")
	}
	if result.ContinueFrom != "next-page-token" {
		t.Errorf("Expected ContinueFrom 'next-page-token', got %q", result.ContinueFrom)
	}
}

func TestGetCategoryMembers_WithType(t *testing.T) {
	server := createCategoryMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		cmtype := r.FormValue("cmtype")

		// Verify type filter is set
		if cmtype != "page" {
			t.Errorf("Expected cmtype 'page', got %q", cmtype)
		}

		response := map[string]interface{}{
			"query": map[string]interface{}{
				"categorymembers": []interface{}{
					map[string]interface{}{
						"pageid": float64(1),
						"title":  "Some Page",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createCategoryTestClient(t, server)
	defer client.Close()

	result, err := client.GetCategoryMembers(context.Background(), CategoryMembersArgs{
		Category: "Test",
		Type:     "page",
	})
	if err != nil {
		t.Fatalf("GetCategoryMembers failed: %v", err)
	}

	if len(result.Members) != 1 {
		t.Errorf("Expected 1 member, got %d", len(result.Members))
	}
}

func TestGetCategoryMembers_Continuation(t *testing.T) {
	server := createCategoryMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"categorymembers": []interface{}{
					map[string]interface{}{
						"pageid": float64(1),
						"title":  "Page",
					},
				},
			},
			"continue": map[string]interface{}{
				"cmcontinue": "continue-token",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createCategoryTestClient(t, server)
	defer client.Close()

	result, err := client.GetCategoryMembers(context.Background(), CategoryMembersArgs{
		Category: "Test",
	})
	if err != nil {
		t.Fatalf("GetCategoryMembers failed: %v", err)
	}

	if !result.HasMore {
		t.Error("Expected HasMore to be true")
	}
	if result.ContinueFrom != "continue-token" {
		t.Errorf("Expected ContinueFrom 'continue-token', got %q", result.ContinueFrom)
	}
}
