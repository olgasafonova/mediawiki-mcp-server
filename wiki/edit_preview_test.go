package wiki

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// TestDiffLines covers the pure line-diff that backs edit_page previews.
func TestDiffLines(t *testing.T) {
	tests := []struct {
		name    string
		oldText string
		newText string
		want    []TextChange
	}{
		{
			name:    "identical content yields no changes",
			oldText: "a\nb\nc",
			newText: "a\nb\nc",
			want:    nil,
		},
		{
			name:    "trailing newline is not a change",
			oldText: "a\nb",
			newText: "a\nb\n",
			want:    nil,
		},
		{
			name:    "creation diffs against empty content",
			oldText: "",
			newText: "a\nb",
			want:    []TextChange{{Line: 1, After: "a"}, {Line: 2, After: "b"}},
		},
		{
			name:    "modified line reports before and after",
			oldText: "a\nb\nc",
			newText: "a\nB\nc",
			want:    []TextChange{{Line: 2, Before: "b"}, {Line: 2, After: "B"}},
		},
		{
			name:    "insertion shifts nothing around it",
			oldText: "a\nc",
			newText: "a\nb\nc",
			want:    []TextChange{{Line: 2, After: "b"}},
		},
		{
			name:    "deletion keeps the old line number",
			oldText: "a\nb\nc",
			newText: "a\nc",
			want:    []TextChange{{Line: 2, Before: "b"}},
		},
		{
			name:    "changed core inside a large page",
			oldText: "h1\nh2\nbody old\nh3\nh4",
			newText: "h1\nh2\nbody new\nh3\nh4",
			want:    []TextChange{{Line: 3, Before: "body old"}, {Line: 3, After: "body new"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffLines(tt.oldText, tt.newText)
			if len(got) != len(tt.want) {
				t.Fatalf("diffLines() = %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("diffLines()[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestDiffLines_LargeRewriteFallsBackPositionally covers the memory guard:
// when the LCS table would exceed maxDiffCells, the changed core is paired
// positionally instead of allocating a quadratic table.
func TestDiffLines_LargeRewriteFallsBackPositionally(t *testing.T) {
	// 2100*2100 = 4.41M cells, just over the 4.19M-cell budget, with every
	// line differing so prefix/suffix trimming cannot shrink the core.
	const lines = 2100
	oldLines := make([]string, lines)
	newLines := make([]string, lines)
	for i := 0; i < lines; i++ {
		oldLines[i] = fmt.Sprintf("old %d", i)
		newLines[i] = fmt.Sprintf("new %d", i)
	}

	changes := diffLines(strings.Join(oldLines, "\n"), strings.Join(newLines, "\n"))
	if len(changes) != lines {
		t.Fatalf("changes = %d, want %d", len(changes), lines)
	}
	for i, change := range changes {
		if change.Line != i+1 || change.Before != oldLines[i] || change.After != newLines[i] {
			t.Fatalf("changes[%d] = %+v, want positional pairing of line %d", i, change, i+1)
		}
	}
}

// TestEditPage_PreviewAfterApplyIsFresh guards the cache interaction between
// the preview and apply paths: the first preview warms the page-content cache,
// so a successful apply must invalidate it. Without invalidation a second
// preview of the just-saved content diffs against the pre-edit snapshot and
// reports phantom changes for a page that would not change at all.
//
// The mock is stateful — the edit action updates the content the query action
// returns — to model a real wiki across the three calls.
func TestEditPage_PreviewAfterApplyIsFresh(t *testing.T) {
	var mu sync.Mutex
	current := "old line one\nold line two"
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.FormValue("action") {
		case "query":
			mu.Lock()
			content := current
			mu.Unlock()
			encodeJSONResponse(t, w, previewPageResponse(content))
		case "edit":
			mu.Lock()
			current = r.FormValue("text")
			mu.Unlock()
			encodeJSONResponse(t, w, map[string]interface{}{
				"edit": map[string]interface{}{
					"result":   "Success",
					"pageid":   float64(123),
					"title":    "Test Page",
					"newrevid": float64(101),
				},
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	planned := "old line one\nnew line two"

	// 1. Preview warms the read cache with the pre-edit revision.
	first, err := client.EditPage(context.Background(), EditPageArgs{
		Title: "Test Page", Content: planned,
	})
	if err != nil {
		t.Fatalf("first preview failed: %v", err)
	}
	if len(first.Changes) != 2 {
		t.Fatalf("first preview changes = %+v, want a removal followed by an addition for the modified line", first.Changes)
	}

	// 2. Apply the planned content.
	applied, err := client.EditPage(context.Background(), EditPageArgs{
		Title: "Test Page", Content: planned, Preview: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if applied.Preview || !applied.Success {
		t.Fatalf("apply = preview:%v success:%v, want an applied successful edit", applied.Preview, applied.Success)
	}

	// 3. Previewing the same content again must see the new revision: the
	// page would not change, so any reported change is a stale-cache phantom.
	second, err := client.EditPage(context.Background(), EditPageArgs{
		Title: "Test Page", Content: planned,
	})
	if err != nil {
		t.Fatalf("second preview failed: %v", err)
	}
	if len(second.Changes) != 0 {
		t.Errorf("second preview changes = %+v, want none: content now matches the saved revision", second.Changes)
	}
	if !strings.Contains(second.Message, "identical") {
		t.Errorf("second preview message = %q, want it to say the content is identical", second.Message)
	}
}
