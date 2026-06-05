package wiki

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMovePage_Success(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("action") == "move" {
			response := map[string]interface{}{
				"move": map[string]interface{}{
					"from":     "Old Title",
					"to":       "New Title",
					"reason":   "Renaming",
					"talkfrom": "Talk:Old Title",
					"talkto":   "Talk:New Title",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.MovePage(context.Background(), MovePageArgs{
		From:   "Old Title",
		To:     "New Title",
		Reason: "Renaming",
	})
	if err != nil {
		t.Fatalf("MovePage failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if result.From != "Old Title" || result.To != "New Title" {
		t.Errorf("From/To = %q/%q, want 'Old Title'/'New Title'", result.From, result.To)
	}
	if !result.TalkMoved {
		t.Error("expected TalkMoved=true when talkfrom is present")
	}
	if !strings.Contains(result.RedirectURL, "Old_Title") {
		t.Errorf("RedirectURL should reference the source title, got: %q", result.RedirectURL)
	}
}

func TestMovePage_APIError(t *testing.T) {
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("action") == "move" {
			response := map[string]interface{}{
				"error": map[string]interface{}{
					"code": "articleexists",
					"info": "A page of that name already exists",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.MovePage(context.Background(), MovePageArgs{
		From: "Old Title",
		To:   "Existing Title",
	})
	// The request layer surfaces MediaWiki API errors as a returned Go error.
	if err == nil {
		t.Fatal("expected an error for an API-level failure")
	}
	if result.Success {
		t.Error("expected success=false on API error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %v, want it to surface the API info", err)
	}
}

func TestMovePage_BadTokenRetry(t *testing.T) {
	attempts := 0
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("action") == "move" {
			attempts++
			if attempts == 1 {
				response := map[string]interface{}{
					"error": map[string]interface{}{
						"code": "badtoken",
						"info": "Invalid CSRF token",
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
				return
			}
			response := map[string]interface{}{
				"move": map[string]interface{}{
					"from": "Old Title",
					"to":   "New Title",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.MovePage(context.Background(), MovePageArgs{
		From: "Old Title",
		To:   "New Title",
	})
	if err != nil {
		t.Fatalf("MovePage failed after retry: %v", err)
	}
	if !result.Success {
		t.Error("expected success after badtoken retry")
	}
	if attempts != 2 {
		t.Errorf("expected 2 move attempts, got %d", attempts)
	}
}

func TestMovePage_UnexpectedResponse(t *testing.T) {
	// Neither "error" nor "move" key in the response.
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("action") == "move" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"something":"else"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.MovePage(context.Background(), MovePageArgs{
		From: "Old Title",
		To:   "New Title",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected success=false for unexpected response format")
	}
	if !strings.Contains(result.Message, "Unexpected response") {
		t.Errorf("Message = %q, want unexpected-response message", result.Message)
	}
}

func TestMovePage_EmptyFrom(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.MovePage(context.Background(), MovePageArgs{To: "New Title"})
	if err == nil {
		t.Fatal("expected error for empty 'from'")
	}
	if _, ok := err.(*ValidationError); !ok {
		t.Errorf("expected ValidationError, got %T", err)
	}
}

func TestMovePage_EmptyTo(t *testing.T) {
	client := createTestClient(t)
	defer client.Close()

	_, err := client.MovePage(context.Background(), MovePageArgs{From: "Old Title"})
	if err == nil {
		t.Fatal("expected error for empty 'to'")
	}
	if _, ok := err.(*ValidationError); !ok {
		t.Errorf("expected ValidationError, got %T", err)
	}
}

func TestMovePage_WithOptionalFlags(t *testing.T) {
	// Exercise the MoveTalk / MoveSubpages / NoRedirect parameter branches in
	// performMove.
	var sawNoRedirect, sawMoveTalk, sawMoveSubpages string
	server := mockMediaWikiServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("action") == "move" {
			sawNoRedirect = r.FormValue("noredirect")
			sawMoveTalk = r.FormValue("movetalk")
			sawMoveSubpages = r.FormValue("movesubpages")
			response := map[string]interface{}{
				"move": map[string]interface{}{
					"from": "Old Title",
					"to":   "New Title",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	defer server.Close()

	client := createMockClient(t, server)
	defer client.Close()

	result, err := client.MovePage(context.Background(), MovePageArgs{
		From:         "Old Title",
		To:           "New Title",
		NoRedirect:   true,
		MoveTalk:     true,
		MoveSubpages: true,
	})
	if err != nil {
		t.Fatalf("MovePage failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if sawNoRedirect != "1" {
		t.Errorf("noredirect = %q, want 1", sawNoRedirect)
	}
	if sawMoveTalk != "1" {
		t.Errorf("movetalk = %q, want 1", sawMoveTalk)
	}
	if sawMoveSubpages != "1" {
		t.Errorf("movesubpages = %q, want 1", sawMoveSubpages)
	}
}
