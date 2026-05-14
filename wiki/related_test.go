package wiki

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestGetRelated_Success(t *testing.T) {
	callCount := 0
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")
		callCount++

		if action == "query" {
			prop := r.FormValue("prop")
			if prop == "categories" {
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(1),
								"title":  "Test Page",
								"categories": []interface{}{
									map[string]interface{}{"title": "Category:Technology"},
									map[string]interface{}{"title": "Category:Software"},
								},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
			if r.FormValue("list") == "categorymembers" {
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"categorymembers": []interface{}{
							map[string]interface{}{"pageid": float64(2), "title": "Related Page 1"},
							map[string]interface{}{"pageid": float64(3), "title": "Related Page 2"},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":{}}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.GetRelated(ctx, GetRelatedArgs{Title: "Test Page", Limit: 5})

	if err != nil {
		t.Fatalf("GetRelated failed: %v", err)
	}
	if result.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", result.Title, "Test Page")
	}
}

func TestGetRelated_LinksMethod(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")
		if action == "query" {
			prop := r.FormValue("prop")
			if prop == "links" {
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(1),
								"title":  "Test Page",
								"links": []interface{}{
									map[string]interface{}{"ns": float64(0), "title": "Linked Page 1"},
									map[string]interface{}{"ns": float64(0), "title": "Linked Page 2"},
								},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":{}}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.GetRelated(ctx, GetRelatedArgs{
		Title:  "Test Page",
		Method: "links",
		Limit:  10,
	})

	if err != nil {
		t.Fatalf("GetRelated failed: %v", err)
	}
	if result.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", result.Title, "Test Page")
	}
	if len(result.RelatedPages) != 2 {
		t.Errorf("RelatedPages count = %d, want 2", len(result.RelatedPages))
	}
}

func TestGetRelated_AllMethod(t *testing.T) {
	requestCount := 0
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_ = r.ParseForm()
		action := r.FormValue("action")
		if action == "query" {
			prop := r.FormValue("prop")
			if strings.Contains(prop, "categories") {
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(1),
								"title":  "Test Page",
								"categories": []interface{}{
									map[string]interface{}{"title": "Category:Testing"},
								},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
			if prop == "links" {
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(1),
								"title":  "Test Page",
								"links": []interface{}{
									map[string]interface{}{"ns": float64(0), "title": "Related Topic"},
								},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
			list := r.FormValue("list")
			if list == "categorymembers" {
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"categorymembers": []interface{}{
							map[string]interface{}{"pageid": float64(2), "title": "Category Sibling"},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
			if list == "backlinks" {
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"backlinks": []interface{}{
							map[string]interface{}{"pageid": float64(3), "title": "Linking Page"},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":{}}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.GetRelated(ctx, GetRelatedArgs{
		Title:  "Test Page",
		Method: "all",
		Limit:  10,
	})

	if err != nil {
		t.Fatalf("GetRelated failed: %v", err)
	}
	if result.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", result.Title, "Test Page")
	}
}
