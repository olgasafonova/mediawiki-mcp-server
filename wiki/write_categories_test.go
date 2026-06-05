package wiki

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

func TestParseExistingCategories(t *testing.T) {
	content := "Intro text\n[[Category:Docs]]\n[[Category:API|sortkey]]\nMore text [[Category:Guides]]"
	got := parseExistingCategories(content)

	for _, want := range []string{"Docs", "API", "Guides"} {
		if !got[want] {
			t.Errorf("expected category %q to be parsed, got %v", want, got)
		}
	}
	if got["Missing"] {
		t.Error("did not expect 'Missing' category")
	}
	if len(got) != 3 {
		t.Errorf("expected 3 categories, got %d (%v)", len(got), got)
	}
}

func TestParseExistingCategories_None(t *testing.T) {
	got := parseExistingCategories("Just some text with no categories.")
	if len(got) != 0 {
		t.Errorf("expected no categories, got %v", got)
	}
}

func TestRemoveCategoriesFromContent(t *testing.T) {
	content := "Body\n[[Category:Docs]]\n[[Category:API]]\n"
	existing := parseExistingCategories(content)

	newContent, removed, notFound := removeCategoriesFromContent(content, []string{"Docs", "Ghost"}, existing)

	if len(removed) != 1 || removed[0] != "Docs" {
		t.Errorf("removed = %v, want [Docs]", removed)
	}
	if len(notFound) != 1 || notFound[0] != "Ghost" {
		t.Errorf("notFound = %v, want [Ghost]", notFound)
	}
	if strings.Contains(newContent, "[[Category:Docs]]") {
		t.Errorf("Docs category should be removed from content, got: %q", newContent)
	}
	if !strings.Contains(newContent, "[[Category:API]]") {
		t.Errorf("API category should remain, got: %q", newContent)
	}
	// The existing set must be updated in place.
	if existing["Docs"] {
		t.Error("expected 'Docs' removed from existing set")
	}
}

func TestAddCategoriesToContent(t *testing.T) {
	content := "Body\n[[Category:Docs]]\n"
	existing := parseExistingCategories(content)

	newContent, added, alreadyPresent := addCategoriesToContent(content, []string{"API", "Docs"}, existing)

	if len(added) != 1 || added[0] != "API" {
		t.Errorf("added = %v, want [API]", added)
	}
	if len(alreadyPresent) != 1 || alreadyPresent[0] != "Docs" {
		t.Errorf("alreadyPresent = %v, want [Docs]", alreadyPresent)
	}
	if !strings.Contains(newContent, "[[Category:API]]") {
		t.Errorf("API category should be appended, got: %q", newContent)
	}
	if !existing["API"] {
		t.Error("expected 'API' added to existing set")
	}
}

func TestBuildCategoryEditSummary(t *testing.T) {
	tests := []struct {
		name    string
		added   []string
		removed []string
		want    string
	}{
		{"add only", []string{"Docs", "API"}, nil, "Added categories: Docs, API"},
		{"remove only", nil, []string{"Old"}, "Removed categories: Old"},
		{"both", []string{"New"}, []string{"Old"}, "Added categories: New. Removed categories: Old"},
		{"neither", nil, nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildCategoryEditSummary(tc.added, tc.removed)
			if got != tc.want {
				t.Errorf("buildCategoryEditSummary(%v, %v) = %q, want %q", tc.added, tc.removed, got, tc.want)
			}
		})
	}
}

func TestKeysOf(t *testing.T) {
	m := map[string]bool{"a": true, "b": true, "c": true}
	got := keysOf(m)
	sort.Strings(got)
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("keysOf = %v, want %v", got, want)
	}

	if len(keysOf(map[string]bool{})) != 0 {
		t.Error("expected empty slice for empty map")
	}
}

// categoriesMockServer returns a server that serves a wikitext page via the
// query API and accepts edits, mirroring the GetPage→EditPage flow that
// ManageCategories drives.
func categoriesMockServer(t *testing.T, pageContent string) *httptest.Server {
	t.Helper()
	return mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.FormValue("action") {
		case "query":
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
											"content": pageContent,
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
		case "edit":
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
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
}

func TestManageCategories_AddAndRemove(t *testing.T) {
	server := categoriesMockServer(t, "Body text\n[[Category:Old]]\n")
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.ManageCategories(context.Background(), ManageCategoriesArgs{
		Title:  "Test Page",
		Add:    []string{"New"},
		Remove: []string{"Old"},
	})
	if err != nil {
		t.Fatalf("ManageCategories failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if len(result.Added) != 1 || result.Added[0] != "New" {
		t.Errorf("Added = %v, want [New]", result.Added)
	}
	if len(result.Removed) != 1 || result.Removed[0] != "Old" {
		t.Errorf("Removed = %v, want [Old]", result.Removed)
	}
	if result.RevisionID != 101 {
		t.Errorf("RevisionID = %d, want 101", result.RevisionID)
	}
}

func TestManageCategories_Preview(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("action") == "edit" {
			t.Error("edit must not be called in preview mode")
		}
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
									"main": map[string]interface{}{"content": "Body\n"},
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

	result, err := client.ManageCategories(context.Background(), ManageCategoriesArgs{
		Title:   "Test Page",
		Add:     []string{"New"},
		Preview: true,
	})
	if err != nil {
		t.Fatalf("ManageCategories failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success in preview")
	}
	if !result.Preview {
		t.Error("expected Preview flag set")
	}
	if !strings.Contains(result.Message, "Preview") {
		t.Errorf("Message = %q, want a preview message", result.Message)
	}
}

func TestManageCategories_NoChangeNeeded(t *testing.T) {
	// Page already has the category we want to add: no edit, success with
	// "No changes needed".
	server := categoriesMockServer(t, "Body\n[[Category:Docs]]\n")
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.ManageCategories(context.Background(), ManageCategoriesArgs{
		Title: "Test Page",
		Add:   []string{"Docs"},
	})
	if err != nil {
		t.Fatalf("ManageCategories failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if result.Message != "No changes needed" {
		t.Errorf("Message = %q, want 'No changes needed'", result.Message)
	}
	if len(result.AlreadyPresent) != 1 || result.AlreadyPresent[0] != "Docs" {
		t.Errorf("AlreadyPresent = %v, want [Docs]", result.AlreadyPresent)
	}
}

func TestManageCategories_EmptyTitle(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.ManageCategories(context.Background(), ManageCategoriesArgs{
		Add: []string{"Docs"},
	})
	if err == nil {
		t.Error("expected error for empty title")
	}
}

func TestManageCategories_NoAddOrRemove(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.ManageCategories(context.Background(), ManageCategoriesArgs{
		Title: "Test Page",
	})
	if err == nil {
		t.Error("expected error when neither add nor remove is specified")
	}
}
