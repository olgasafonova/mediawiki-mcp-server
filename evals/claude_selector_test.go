package evals

import (
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/olgasafonova/mediawiki-mcp-server/tools"
)

// NewClaudeSelector only constructs an API client; it performs no network call.
// These tests exercise the construction logic hermetically by controlling the
// ANTHROPIC_API_KEY environment variable with t.Setenv. SelectTool is not
// exercised here because it requires a live Anthropic API round trip.

// TestNewClaudeSelectorMissingKey verifies construction fails with a clear
// error when no API key is configured.
func TestNewClaudeSelectorMissingKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	selector, err := NewClaudeSelector("")
	if err == nil {
		t.Fatal("expected error when ANTHROPIC_API_KEY is unset, got nil")
	}
	if selector != nil {
		t.Errorf("expected nil selector on error, got %+v", selector)
	}
}

// TestNewClaudeSelectorDefaultModel verifies an empty model string defaults to
// the configured Sonnet model and that the default timeout is applied.
func TestNewClaudeSelectorDefaultModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key-not-used")

	selector, err := NewClaudeSelector("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selector.model != anthropic.ModelClaudeSonnet4_6 {
		t.Errorf("default model = %q, want %q", selector.model, anthropic.ModelClaudeSonnet4_6)
	}
	if selector.timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want %v", selector.timeout, 30*time.Second)
	}
}

// TestNewClaudeSelectorExplicitModel verifies an explicit model string is
// passed through unchanged rather than overridden by the default.
func TestNewClaudeSelectorExplicitModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key-not-used")

	const customModel = "claude-opus-4-1"
	selector, err := NewClaudeSelector(customModel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(selector.model) != customModel {
		t.Errorf("model = %q, want %q", selector.model, customModel)
	}
}

// TestNewClaudeSelectorToolDefs verifies the selector builds one tool
// definition per registered tool, preserving names and descriptions, and
// applies an ephemeral cache breakpoint to the final tool def.
func TestNewClaudeSelectorToolDefs(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key-not-used")

	selector, err := NewClaudeSelector("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(selector.toolDefs) != len(tools.AllTools) {
		t.Fatalf("toolDefs count = %d, want %d", len(selector.toolDefs), len(tools.AllTools))
	}
	if len(selector.toolDefs) == 0 {
		t.Fatal("expected at least one tool def")
	}

	// Names and descriptions should mirror the source specs in order.
	for i, spec := range tools.AllTools {
		def := selector.toolDefs[i].OfTool
		if def == nil {
			t.Fatalf("toolDefs[%d].OfTool is nil", i)
		}
		if def.Name != spec.Name {
			t.Errorf("toolDefs[%d].Name = %q, want %q", i, def.Name, spec.Name)
		}
	}

	// The last tool def carries the ephemeral cache-control breakpoint so all
	// preceding tool defs are cached.
	last := selector.toolDefs[len(selector.toolDefs)-1].OfTool
	if last.CacheControl.Type == "" {
		t.Error("expected cache control on the last tool def, got none")
	}
}

// TestClaudeSelectorImplementsToolSelector is a compile-time assertion that
// ClaudeSelector satisfies the ToolSelector interface used by the evaluators.
func TestClaudeSelectorImplementsToolSelector(t *testing.T) {
	var _ ToolSelector = (*ClaudeSelector)(nil)
}
