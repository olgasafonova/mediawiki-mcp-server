package wiki

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestGetSections_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")
		if action == "parse" {
			response := map[string]interface{}{
				"parse": map[string]interface{}{
					"title":  "Test Page",
					"pageid": float64(1),
					"sections": []interface{}{
						map[string]interface{}{
							"toclevel":   float64(1),
							"level":      "2",
							"line":       "Introduction",
							"number":     "1",
							"index":      "1",
							"fromtitle":  "Test_Page",
							"byteoffset": float64(0),
							"anchor":     "Introduction",
						},
						map[string]interface{}{
							"toclevel":   float64(1),
							"level":      "2",
							"line":       "Details",
							"number":     "2",
							"index":      "2",
							"fromtitle":  "Test_Page",
							"byteoffset": float64(100),
							"anchor":     "Details",
						},
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
	result, err := client.GetSections(ctx, GetSectionsArgs{Title: "Test Page"})

	if err != nil {
		t.Fatalf("GetSections failed: %v", err)
	}
	if result.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", result.Title, "Test Page")
	}
	if len(result.Sections) != 2 {
		t.Errorf("Sections count = %d, want 2", len(result.Sections))
	}
}

func TestGetSections_WithSection(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")
		if action == "parse" {
			section := r.FormValue("section")
			if section != "" {
				response := map[string]interface{}{
					"parse": map[string]interface{}{
						"title":  "Test Page",
						"pageid": float64(1),
						"wikitext": map[string]interface{}{
							"*": "== Introduction ==\nSection content here",
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"parse":{"sections":[]}}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.GetSections(ctx, GetSectionsArgs{Title: "Test Page", Section: 1})

	if err != nil {
		t.Fatalf("GetSections with section failed: %v", err)
	}
	if result.SectionContent == "" {
		t.Error("Expected section content, got empty")
	}
}
