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

// Helper function to create a mock server for user tests
func createUserMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
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

func createUserTestClient(t *testing.T, server *httptest.Server) *Client {
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

func TestListUsers_Success(t *testing.T) {
	server := createUserMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"allusers": []interface{}{
					map[string]interface{}{
						"userid":       float64(1),
						"name":         "Admin",
						"editcount":    float64(500),
						"registration": "2020-01-01T00:00:00Z",
						"groups":       []interface{}{"administrator", "sysop"},
					},
					map[string]interface{}{
						"userid":       float64(2),
						"name":         "RegularUser",
						"editcount":    float64(50),
						"registration": "2021-06-15T00:00:00Z",
						"groups":       []interface{}{"user"},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createUserTestClient(t, server)
	defer client.Close()

	result, err := client.ListUsers(context.Background(), ListUsersArgs{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}

	if len(result.Users) != 2 {
		t.Errorf("Expected 2 users, got %d", len(result.Users))
	}
	if result.Users[0].Name != "Admin" {
		t.Errorf("Expected first user 'Admin', got %q", result.Users[0].Name)
	}
	if result.Users[0].EditCount != 500 {
		t.Errorf("Expected edit count 500, got %d", result.Users[0].EditCount)
	}
	if len(result.Users[0].Groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(result.Users[0].Groups))
	}
}

func TestListUsers_WithGroup(t *testing.T) {
	server := createUserMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		group := r.FormValue("augroup")

		if group != "sysop" {
			t.Errorf("Expected group 'sysop', got %q", group)
		}

		response := map[string]interface{}{
			"query": map[string]interface{}{
				"allusers": []interface{}{
					map[string]interface{}{
						"userid":    float64(1),
						"name":      "Admin",
						"editcount": float64(500),
						"groups":    []interface{}{"sysop"},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createUserTestClient(t, server)
	defer client.Close()

	result, err := client.ListUsers(context.Background(), ListUsersArgs{
		Group: "sysop",
	})
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}

	if result.Group != "sysop" {
		t.Errorf("Expected group filter 'sysop', got %q", result.Group)
	}
	if len(result.Users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(result.Users))
	}
}

func TestListUsers_ActiveOnly(t *testing.T) {
	server := createUserMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		activeUsers := r.FormValue("auactiveusers")

		if activeUsers != "1" {
			t.Errorf("Expected auactiveusers '1', got %q", activeUsers)
		}

		response := map[string]interface{}{
			"query": map[string]interface{}{
				"allusers": []interface{}{
					map[string]interface{}{
						"userid":    float64(1),
						"name":      "ActiveUser",
						"editcount": float64(100),
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createUserTestClient(t, server)
	defer client.Close()

	result, err := client.ListUsers(context.Background(), ListUsersArgs{
		ActiveOnly: true,
	})
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}

	if len(result.Users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(result.Users))
	}
}

func TestListUsers_Continuation(t *testing.T) {
	server := createUserMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"query": map[string]interface{}{
				"allusers": []interface{}{
					map[string]interface{}{
						"userid": float64(1),
						"name":   "User1",
					},
				},
			},
			"continue": map[string]interface{}{
				"aufrom": "User2",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createUserTestClient(t, server)
	defer client.Close()

	result, err := client.ListUsers(context.Background(), ListUsersArgs{})
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}

	if !result.HasMore {
		t.Error("Expected HasMore to be true")
	}
	if result.ContinueFrom != "User2" {
		t.Errorf("Expected ContinueFrom 'User2', got %q", result.ContinueFrom)
	}
}
