package tools

import (
	"log/slog"
	"os"
	"testing"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
)

func TestNewHandlerRegistry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	config := &wiki.Config{
		BaseURL: "https://test.wiki.com/api.php",
		Timeout: 5,
	}
	client := wiki.NewClient(config, logger)
	defer client.Close()

	registry := NewHandlerRegistry(client, logger)

	if registry == nil {
		t.Fatal("Expected non-nil registry")
	}
	if registry.client != client {
		t.Error("Registry should hold the client reference")
	}
	if registry.logger != logger {
		t.Error("Registry should hold the logger reference")
	}
}

func TestBuildTool(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	config := &wiki.Config{BaseURL: "https://test.wiki.com/api.php"}
	client := wiki.NewClient(config, logger)
	defer client.Close()

	registry := NewHandlerRegistry(client, logger)

	tests := []struct {
		name      string
		spec      ToolSpec
		wantName  string
		wantDesc  string
		wantRO    bool
		wantIdem  bool
		wantDestr bool
		wantOpen  bool
	}{
		{
			name: "read-only tool",
			spec: ToolSpec{
				Name:        "mediawiki_search",
				Title:       "Search Wiki",
				Description: "Search the wiki",
				Method:      "Search",
				ReadOnly:    true,
				Idempotent:  true,
			},
			wantName:  "mediawiki_search",
			wantDesc:  "Search the wiki",
			wantRO:    true,
			wantIdem:  true,
			wantDestr: false,
			wantOpen:  false,
		},
		{
			name: "destructive tool",
			spec: ToolSpec{
				Name:        "mediawiki_edit_page",
				Title:       "Edit Page",
				Description: "Edit a wiki page",
				Method:      "EditPage",
				ReadOnly:    false,
				Destructive: true,
			},
			wantName:  "mediawiki_edit_page",
			wantDesc:  "Edit a wiki page",
			wantRO:    false,
			wantDestr: true,
		},
		{
			name: "open world tool",
			spec: ToolSpec{
				Name:        "mediawiki_check_links",
				Title:       "Check Links",
				Description: "Check external links",
				Method:      "CheckLinks",
				OpenWorld:   true,
			},
			wantName: "mediawiki_check_links",
			wantDesc: "Check external links",
			wantOpen: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := registry.buildTool(tt.spec)

			if tool.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", tool.Name, tt.wantName)
			}
			if tool.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", tool.Description, tt.wantDesc)
			}
			if tool.Annotations == nil {
				t.Fatal("Expected annotations")
			}
			if tool.Annotations.ReadOnlyHint != tt.wantRO {
				t.Errorf("ReadOnlyHint = %v, want %v", tool.Annotations.ReadOnlyHint, tt.wantRO)
			}
			if tool.Annotations.IdempotentHint != tt.wantIdem {
				t.Errorf("IdempotentHint = %v, want %v", tool.Annotations.IdempotentHint, tt.wantIdem)
			}
			if tt.wantDestr && (tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint) {
				t.Error("Expected DestructiveHint to be true")
			}
			if tt.wantOpen && (tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint) {
				t.Error("Expected OpenWorldHint to be true")
			}
		})
	}
}

func TestRecoverPanic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	config := &wiki.Config{BaseURL: "https://test.wiki.com/api.php"}
	client := wiki.NewClient(config, logger)
	defer client.Close()

	registry := NewHandlerRegistry(client, logger)

	// Test that recoverPanic doesn't panic itself
	func() {
		defer registry.recoverPanic("test_tool")
		panic("test panic")
	}()

	// If we get here, panic was recovered successfully
}

func TestLogExecution(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	config := &wiki.Config{BaseURL: "https://test.wiki.com/api.php"}
	client := wiki.NewClient(config, logger)
	defer client.Close()

	registry := NewHandlerRegistry(client, logger)
	spec := ToolSpec{Name: "test_tool"}

	// Test with SearchArgs
	registry.logExecution(spec, wiki.SearchArgs{Query: "test"}, wiki.SearchResult{TotalHits: 5})

	// Test with GetPageArgs
	registry.logExecution(spec, wiki.GetPageArgs{Title: "Test", Format: "wikitext"}, wiki.PageContent{Content: "test"})

	// Test with EditPageArgs
	registry.logExecution(spec, wiki.EditPageArgs{Title: "Test", Content: "content"}, wiki.EditResult{Success: true})

	// Test with FindReplaceArgs
	registry.logExecution(spec, wiki.FindReplaceArgs{Title: "Test", Preview: true}, wiki.FindReplaceResult{MatchCount: 3})

	// Test with BulkReplaceArgs
	registry.logExecution(spec, wiki.BulkReplaceArgs{Pages: []string{"A", "B"}, Preview: true}, wiki.BulkReplaceResult{PagesModified: 2})

	// Test with SearchInPageArgs
	registry.logExecution(spec, wiki.SearchInPageArgs{Title: "Test", Query: "search"}, wiki.SearchInPageResult{})
}

func TestAllToolsNotEmpty(t *testing.T) {
	if len(AllTools) == 0 {
		t.Error("AllTools should not be empty")
	}

	// Verify each tool has required fields
	for i, spec := range AllTools {
		if spec.Name == "" {
			t.Errorf("Tool %d has empty Name", i)
		}
		if spec.Method == "" {
			t.Errorf("Tool %s has empty Method", spec.Name)
		}
		if spec.Description == "" {
			t.Errorf("Tool %s has empty Description", spec.Name)
		}
	}
}

func TestToolSpecMethods(t *testing.T) {
	knownMethods := map[string]bool{
		"Search": true, "SearchInPage": true, "SearchInFile": true, "ResolveTitle": true,
		"GetPage": true, "ListPages": true, "GetPageInfo": true, "GetSections": true,
		"GetRelated": true, "GetImages": true, "Parse": true, "GetWikiInfo": true,
		"ListCategories": true, "GetCategoryMembers": true,
		"GetRecentChanges": true, "GetRevisions": true, "CompareRevisions": true, "GetUserContributions": true,
		"GetExternalLinks": true, "GetExternalLinksBatch": true, "CheckLinks": true, "GetBacklinks": true,
		"FindBrokenInternalLinks": true, "FindOrphanedPages": true,
		"CheckTerminology": true, "CheckTranslations": true, "HealthAudit": true,
		"FindSimilarPages": true, "CompareTopic": true,
		"ListUsers": true,
		"EditPage":  true, "FindReplace": true, "ApplyFormatting": true, "BulkReplace": true, "UploadFile": true,
	}

	for _, spec := range AllTools {
		if !knownMethods[spec.Method] {
			t.Errorf("Tool %s has unknown method: %s", spec.Name, spec.Method)
		}
	}
}
