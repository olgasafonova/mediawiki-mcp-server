package wiki

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestGetImages_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")
		prop := r.FormValue("prop")

		if action == "query" {
			if prop == "images" {
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(1),
								"title":  "Test Page",
								"images": []interface{}{
									map[string]interface{}{"title": "File:Logo.png"},
									map[string]interface{}{"title": "File:Icon.svg"},
								},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
			if prop == "imageinfo" {
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(100),
								"title":  "File:Logo.png",
								"imageinfo": []interface{}{
									map[string]interface{}{
										"url":      "https://wiki.example.com/images/logo.png",
										"thumburl": "https://wiki.example.com/images/thumb/logo.png",
										"width":    float64(200),
										"height":   float64(100),
										"size":     float64(5000),
										"mime":     "image/png",
									},
								},
							},
							"2": map[string]interface{}{
								"pageid": float64(101),
								"title":  "File:Icon.svg",
								"imageinfo": []interface{}{
									map[string]interface{}{
										"url":      "https://wiki.example.com/images/icon.svg",
										"thumburl": "https://wiki.example.com/images/thumb/icon.svg",
										"width":    float64(64),
										"height":   float64(64),
										"size":     float64(1200),
										"mime":     "image/svg+xml",
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
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":{}}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	result, err := client.GetImages(ctx, GetImagesArgs{Title: "Test Page"})

	if err != nil {
		t.Fatalf("GetImages failed: %v", err)
	}
	if result.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", result.Title, "Test Page")
	}
	if result.Count != 2 {
		t.Errorf("Count = %d, want 2", result.Count)
	}
}

func TestGetImages_NoImages(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	result, err := client.GetImages(ctx, GetImagesArgs{Title: "Test Page"})

	if err != nil {
		t.Fatalf("GetImages failed: %v", err)
	}
	if result.Count != 0 {
		t.Errorf("Count = %d, want 0", result.Count)
	}
}

func TestGetImages_EmptyTitle(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	ctx := context.Background()
	_, err := client.GetImages(ctx, GetImagesArgs{Title: ""})

	if err == nil {
		t.Fatal("Expected error for empty title")
	}
}
