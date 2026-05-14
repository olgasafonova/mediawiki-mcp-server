package wiki

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestGetPagesBatch_EmptyTitles(t *testing.T) {
	client := &Client{}
	_, err := client.GetPagesBatch(context.Background(), GetPagesBatchArgs{
		Titles: []string{},
	})
	if err == nil {
		t.Error("Expected error for empty titles")
	}
}

func TestGetPagesBatch_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")

		if action == "query" {
			titles := r.FormValue("titles")
			if !strings.Contains(titles, "|") && strings.Contains(titles, "Page1") {
				response := map[string]interface{}{
					"query": map[string]interface{}{
						"pages": map[string]interface{}{
							"1": map[string]interface{}{
								"pageid": float64(1),
								"title":  "Page1",
								"revisions": []interface{}{
									map[string]interface{}{
										"revid":     float64(100),
										"timestamp": "2024-01-01T00:00:00Z",
										"slots": map[string]interface{}{
											"main": map[string]interface{}{
												"*": "Content of Page1",
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

			response := map[string]interface{}{
				"query": map[string]interface{}{
					"pages": map[string]interface{}{
						"1": map[string]interface{}{
							"pageid": float64(1),
							"title":  "Page1",
							"revisions": []interface{}{
								map[string]interface{}{
									"revid":     float64(100),
									"timestamp": "2024-01-01T00:00:00Z",
									"slots": map[string]interface{}{
										"main": map[string]interface{}{
											"*": "Content of Page1",
										},
									},
								},
							},
						},
						"2": map[string]interface{}{
							"pageid": float64(2),
							"title":  "Page2",
							"revisions": []interface{}{
								map[string]interface{}{
									"revid":     float64(200),
									"timestamp": "2024-01-02T00:00:00Z",
									"slots": map[string]interface{}{
										"main": map[string]interface{}{
											"*": "Content of Page2",
										},
									},
								},
							},
						},
						"-1": map[string]interface{}{
							"title":   "Missing Page",
							"missing": true,
						},
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
	defer client.Close()

	ctx := context.Background()
	result, err := client.GetPagesBatch(ctx, GetPagesBatchArgs{
		Titles: []string{"Page1", "Page2", "Missing Page"},
	})

	if err != nil {
		t.Fatalf("GetPagesBatch failed: %v", err)
	}

	if result.TotalCount != 3 {
		t.Errorf("TotalCount = %d, want 3", result.TotalCount)
	}

	if result.FoundCount != 2 {
		t.Errorf("FoundCount = %d, want 2", result.FoundCount)
	}

	if result.MissingCount != 1 {
		t.Errorf("MissingCount = %d, want 1", result.MissingCount)
	}

	if len(result.Pages) != 3 {
		t.Errorf("Pages count = %d, want 3", len(result.Pages))
	}

	foundPage1 := false
	foundMissing := false
	for _, page := range result.Pages {
		if page.Title == "Page1" && page.Exists && page.Content != "" {
			foundPage1 = true
		}
		if page.Title == "Missing Page" && !page.Exists {
			foundMissing = true
		}
	}

	if !foundPage1 {
		t.Error("Did not find Page1 with content")
	}
	if !foundMissing {
		t.Error("Did not find Missing Page marked as not existing")
	}
}

func TestGetPagesBatch_BatchSizeLimit(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		titles := r.FormValue("titles")

		titleCount := len(strings.Split(titles, "|"))
		if titleCount > MaxBatchSize {
			t.Errorf("Received %d titles, should be limited to %d", titleCount, MaxBatchSize)
		}

		response := map[string]interface{}{
			"query": map[string]interface{}{
				"pages": map[string]interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	titles := make([]string, 60)
	for i := 0; i < 60; i++ {
		titles[i] = fmt.Sprintf("Page%d", i)
	}

	ctx := context.Background()
	result, err := client.GetPagesBatch(ctx, GetPagesBatchArgs{
		Titles: titles,
	})

	if err != nil {
		t.Fatalf("GetPagesBatch failed: %v", err)
	}

	if result.TotalCount != MaxBatchSize {
		t.Errorf("TotalCount = %d, want %d (limited)", result.TotalCount, MaxBatchSize)
	}
}

func TestGetPagesInfoBatch_EmptyTitles(t *testing.T) {
	client := &Client{}
	_, err := client.GetPagesInfoBatch(context.Background(), GetPagesInfoBatchArgs{
		Titles: []string{},
	})
	if err == nil {
		t.Error("Expected error for empty titles")
	}
}

func TestGetPagesInfoBatch_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		action := r.FormValue("action")

		if action == "query" {
			response := map[string]interface{}{
				"query": map[string]interface{}{
					"pages": map[string]interface{}{
						"1": map[string]interface{}{
							"pageid":       float64(1),
							"title":        "Page1",
							"ns":           float64(0),
							"length":       float64(500),
							"contentmodel": "wikitext",
							"pagelanguage": "en",
							"touched":      "2024-01-01T00:00:00Z",
							"lastrevid":    float64(100),
							"categories": []interface{}{
								map[string]interface{}{"title": "Category:Test"},
							},
						},
						"2": map[string]interface{}{
							"pageid":       float64(2),
							"title":        "Page2",
							"ns":           float64(0),
							"length":       float64(300),
							"contentmodel": "wikitext",
							"pagelanguage": "en",
							"touched":      "2024-01-02T00:00:00Z",
							"lastrevid":    float64(200),
						},
						"-1": map[string]interface{}{
							"title":   "Missing",
							"missing": true,
						},
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
	defer client.Close()

	ctx := context.Background()
	result, err := client.GetPagesInfoBatch(ctx, GetPagesInfoBatchArgs{
		Titles: []string{"Page1", "Page2", "Missing"},
	})

	if err != nil {
		t.Fatalf("GetPagesInfoBatch failed: %v", err)
	}

	if result.TotalCount != 3 {
		t.Errorf("TotalCount = %d, want 3", result.TotalCount)
	}

	if result.ExistsCount != 2 {
		t.Errorf("ExistsCount = %d, want 2", result.ExistsCount)
	}

	if result.MissingCount != 1 {
		t.Errorf("MissingCount = %d, want 1", result.MissingCount)
	}

	for _, page := range result.Pages {
		if page.Title == "Page1" {
			if page.PageID != 1 {
				t.Errorf("Page1 PageID = %d, want 1", page.PageID)
			}
			if page.Length != 500 {
				t.Errorf("Page1 Length = %d, want 500", page.Length)
			}
			if len(page.Categories) != 1 {
				t.Errorf("Page1 categories count = %d, want 1", len(page.Categories))
			}
		}
		if page.Title == "Missing" && page.Exists {
			t.Error("Missing page should have Exists = false")
		}
	}
}
