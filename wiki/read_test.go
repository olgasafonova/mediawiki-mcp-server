package wiki

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestParse_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")
		if action == "parse" {
			response := map[string]interface{}{
				"parse": map[string]interface{}{
					"title":  "Test",
					"pageid": float64(0),
					"text": map[string]interface{}{
						"*": "<p><b>Hello</b> world</p>",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		}
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.Parse(ctx, ParseArgs{
		Wikitext: "'''Hello''' world",
		Title:    "Test",
	})

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if result.HTML == "" {
		t.Error("Expected HTML output, got empty")
	}
}

func TestParse_EmptyWikitext(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	ctx := context.Background()
	_, err := client.Parse(ctx, ParseArgs{
		Wikitext: "",
	})

	if err == nil {
		t.Error("Expected error for empty wikitext")
	}
}

func TestParse_WithCategoriesAndLinks(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")
		if action == "parse" {
			response := map[string]interface{}{
				"parse": map[string]interface{}{
					"title":  "Test",
					"pageid": float64(0),
					"text": map[string]interface{}{
						"*": "<p>Content with [[Link]] and [[Category:Test]]</p>",
					},
					"categories": []interface{}{
						map[string]interface{}{"*": "Test Category"},
						map[string]interface{}{"*": "Another Category"},
					},
					"links": []interface{}{
						map[string]interface{}{"*": "Link One", "ns": float64(0)},
						map[string]interface{}{"*": "Link Two", "ns": float64(0)},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		}
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.Parse(ctx, ParseArgs{
		Wikitext: "Content with [[Link]] and [[Category:Test]]",
	})

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(result.Categories) != 2 {
		t.Errorf("Expected 2 categories, got %d", len(result.Categories))
	}
	if len(result.Links) != 2 {
		t.Errorf("Expected 2 links, got %d", len(result.Links))
	}
}

func TestGetPageHTML_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")
		if action == "parse" {
			response := map[string]interface{}{
				"parse": map[string]interface{}{
					"title":  "Test Page",
					"pageid": float64(1),
					"text": map[string]interface{}{
						"*": "<p>This is the page content in HTML.</p>",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		}
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.GetPage(ctx, GetPageArgs{Title: "Test Page", Format: "html"})

	if err != nil {
		t.Fatalf("GetPage with HTML failed: %v", err)
	}
	if result.Content == "" {
		t.Error("Expected HTML content, got empty")
	}
}

func TestGetPage_HTMLFormat(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")

		if action == "parse" {
			response := map[string]interface{}{
				"parse": map[string]interface{}{
					"title":  "Test Page",
					"pageid": float64(1),
					"text": map[string]interface{}{
						"*": "<p>This is <b>HTML</b> content.</p>",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{
					"1": map[string]interface{}{
						"pageid": float64(1),
						"title":  "Test Page",
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
	result, err := client.GetPage(ctx, GetPageArgs{
		Title:  "Test Page",
		Format: "html",
	})

	if err != nil {
		t.Fatalf("GetPage HTML failed: %v", err)
	}

	if result.Format != "html" {
		t.Errorf("Format = %q, want 'html'", result.Format)
	}
}
