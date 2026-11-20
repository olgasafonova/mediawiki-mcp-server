package tools

import (
	"log/slog"
	"os"
	"strings"
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

// boolPtrString renders an optional hint for test messages: "omitted" for
// nil, otherwise the value.
func boolPtrString(p *bool) string {
	if p == nil {
		return "omitted"
	}
	if *p {
		return "true"
	}
	return "false"
}

// TestBuildToolEmitsFalseHints verifies buildTool emits the spec's real value
// for destructiveHint and openWorldHint instead of omitting false. An omitted
// hint means true under the MCP spec, so omission would advertise an additive
// tool as destructive and a local tool as open-world.
func TestBuildToolEmitsFalseHints(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client := wiki.NewClient(&wiki.Config{BaseURL: "https://test.wiki.com/api.php"}, logger)
	defer client.Close()
	registry := NewHandlerRegistry(client, logger)

	tests := []struct {
		name      string
		spec      ToolSpec
		wantDestr string // "omitted", "true" or "false"
		wantOpen  string
	}{
		{
			name:      "additive local write tool emits both hints false",
			spec:      ToolSpec{Name: "t_additive", ReadOnly: false, Destructive: false, OpenWorld: false},
			wantDestr: "false",
			wantOpen:  "false",
		},
		{
			name:      "destructive open-world write tool emits both hints true",
			spec:      ToolSpec{Name: "t_destructive", ReadOnly: false, Destructive: true, OpenWorld: true},
			wantDestr: "true",
			wantOpen:  "true",
		},
		{
			name:      "read-only local tool omits destructive, emits openWorld false",
			spec:      ToolSpec{Name: "t_readonly_local", ReadOnly: true, OpenWorld: false},
			wantDestr: "omitted",
			wantOpen:  "false",
		},
		{
			name:      "read-only open-world tool omits destructive, emits openWorld true",
			spec:      ToolSpec{Name: "t_readonly_open", ReadOnly: true, OpenWorld: true},
			wantDestr: "omitted",
			wantOpen:  "true",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ann := registry.buildTool(tt.spec).Annotations
			if ann == nil {
				t.Fatal("Expected annotations")
			}
			if got := boolPtrString(ann.DestructiveHint); got != tt.wantDestr {
				t.Errorf("DestructiveHint = %s, want %s", got, tt.wantDestr)
			}
			if got := boolPtrString(ann.OpenWorldHint); got != tt.wantOpen {
				t.Errorf("OpenWorldHint = %s, want %s", got, tt.wantOpen)
			}
		})
	}
}

// TestAllToolsEmittedAnnotations checks the annotations every real spec in
// AllTools emits after buildTool. All tools talk to a wiki, so openWorldHint
// must be true everywhere. Every current write tool edits, moves or replaces
// content, so destructiveHint must be true on each; a new write tool has to be
// added to writeTools here, which forces a deliberate Destructive decision
// instead of inheriting the Go zero value (false).
func TestAllToolsEmittedAnnotations(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client := wiki.NewClient(&wiki.Config{BaseURL: "https://test.wiki.com/api.php"}, logger)
	defer client.Close()
	registry := NewHandlerRegistry(client, logger)

	writeTools := map[string]struct{ destructive, idempotent bool }{
		"mediawiki_edit_page":         {destructive: true},
		"mediawiki_find_replace":      {destructive: true},
		"mediawiki_apply_formatting":  {destructive: true},
		"mediawiki_bulk_replace":      {destructive: true},
		"mediawiki_move_page":         {destructive: true},
		"mediawiki_manage_categories": {destructive: true, idempotent: true},
		"mediawiki_upload_file":       {destructive: true},
	}

	seenWrites := 0
	for _, spec := range AllTools {
		ann := registry.buildTool(spec).Annotations
		if ann == nil {
			t.Errorf("%s: nil annotations", spec.Name)
			continue
		}
		if ann.Title == "" {
			t.Errorf("%s: empty title", spec.Name)
		}
		if got := boolPtrString(ann.OpenWorldHint); got != "true" {
			t.Errorf("%s: openWorldHint = %s, want true", spec.Name, got)
		}
		if ann.ReadOnlyHint {
			if ann.DestructiveHint != nil {
				t.Errorf("%s: read-only tool emits destructiveHint = %s, want omitted", spec.Name, boolPtrString(ann.DestructiveHint))
			}
			if _, listed := writeTools[spec.Name]; listed {
				t.Errorf("%s: listed as a write tool but spec is ReadOnly", spec.Name)
			}
			continue
		}
		want, ok := writeTools[spec.Name]
		if !ok {
			t.Errorf("%s: write tool not listed in writeTools; decide its Destructive/Idempotent values and add it", spec.Name)
			continue
		}
		seenWrites++
		if got, w := boolPtrString(ann.DestructiveHint), boolPtrString(&want.destructive); got != w {
			t.Errorf("%s: destructiveHint = %s, want %s", spec.Name, got, w)
		}
		if ann.IdempotentHint != want.idempotent {
			t.Errorf("%s: idempotentHint = %v, want %v", spec.Name, ann.IdempotentHint, want.idempotent)
		}
	}
	if seenWrites != len(writeTools) {
		t.Errorf("saw %d write tools in AllTools, writeTools lists %d", seenWrites, len(writeTools))
	}
}

func TestRecoverPanic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	config := &wiki.Config{BaseURL: "https://test.wiki.com/api.php"}
	client := wiki.NewClient(config, logger)
	defer client.Close()

	registry := NewHandlerRegistry(client, logger)

	// Test that recoverPanic doesn't panic itself, and that it writes a
	// structured error to *errPtr so the MCP caller sees an error rather
	// than a silent (nil, zero, nil) fake-success response. This is the
	// HG-1 contract for destructive write tools (EditPage, BulkReplace,
	// MovePage) where masked failure is silently destructive.
	var gotErr error
	func() {
		defer registry.recoverPanic("test_tool", &gotErr)
		panic("test panic")
	}()

	if gotErr == nil {
		t.Fatal("recoverPanic must write a non-nil error to *errPtr; got nil (silent-nil dispatcher bug)")
	}
	if !strings.Contains(gotErr.Error(), "test_tool") {
		t.Errorf("error should mention the tool name; got %q", gotErr.Error())
	}
	if !strings.Contains(gotErr.Error(), "correlation_id=") {
		t.Errorf("error should include correlation_id for log correlation; got %q", gotErr.Error())
	}

	// Test the no-panic path: errPtr must not be touched when no panic.
	var noPanicErr error
	func() {
		defer registry.recoverPanic("test_tool", &noPanicErr)
		// no panic here
	}()
	if noPanicErr != nil {
		t.Errorf("recoverPanic must not write to *errPtr when there is no panic; got %v", noPanicErr)
	}

	// Test nil errPtr safety: should not crash even if caller passes nil.
	func() {
		defer registry.recoverPanic("test_tool", nil)
		panic("test panic with nil errPtr")
	}()
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
	registry.logExecution(spec, wiki.FindReplaceArgs{Title: "Test", Preview: ptr(true)}, wiki.FindReplaceResult{MatchCount: 3})

	// Test with BulkReplaceArgs
	registry.logExecution(spec, wiki.BulkReplaceArgs{Pages: []string{"A", "B"}, Preview: ptr(true)}, wiki.BulkReplaceResult{PagesModified: 2})

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
		"ListUsers":     true,
		"GetPagesBatch": true, "GetPagesInfoBatch": true,
		"SearchAndRead": true, "GetPageSummary": true,
		"MovePage": true, "ManageCategories": true,
		"GetStalePages": true,
		"EditPage":      true, "FindReplace": true, "ApplyFormatting": true, "BulkReplace": true, "UploadFile": true,
	}

	for _, spec := range AllTools {
		if !knownMethods[spec.Method] {
			t.Errorf("Tool %s has unknown method: %s", spec.Name, spec.Method)
		}
	}
}
