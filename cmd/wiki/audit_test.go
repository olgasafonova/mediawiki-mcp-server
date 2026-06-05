package main

import (
	"strings"
	"testing"

	"github.com/olgasafonova/mediawiki-mcp-server/wiki"
)

func TestHealthBar(t *testing.T) {
	tests := []struct {
		score  int
		filled int
	}{
		{0, 0},
		{50, 5},
		{100, 10},
		{73, 7},
	}
	for _, tc := range tests {
		bar := healthBar(tc.score)
		if !strings.HasPrefix(bar, "[") || !strings.HasSuffix(bar, "]") {
			t.Errorf("healthBar(%d) = %q, want bracketed", tc.score, bar)
		}
		gotFilled := strings.Count(bar, "█")
		if gotFilled != tc.filled {
			t.Errorf("healthBar(%d) filled = %d, want %d", tc.score, gotFilled, tc.filled)
		}
		// filled + empty must equal 10.
		if total := strings.Count(bar, "█") + strings.Count(bar, "░"); total != 10 {
			t.Errorf("healthBar(%d) segments = %d, want 10", tc.score, total)
		}
	}
}

func TestBuildAuditArgs(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cmd := newAuditCmd()
		args := buildAuditArgs(cmd)
		if args.Limit != 20 {
			t.Errorf("Limit = %d, want 20", args.Limit)
		}
		if len(args.Checks) != 0 {
			t.Errorf("Checks = %v, want empty", args.Checks)
		}
		if args.Category != "" {
			t.Errorf("Category = %q, want empty", args.Category)
		}
	})

	t.Run("checks split and trimmed", func(t *testing.T) {
		cmd := newAuditCmd()
		if err := cmd.Flags().Set("checks", "links, terminology ,orphans"); err != nil {
			t.Fatal(err)
		}
		args := buildAuditArgs(cmd)
		want := []string{"links", "terminology", "orphans"}
		if len(args.Checks) != len(want) {
			t.Fatalf("Checks = %v, want %v", args.Checks, want)
		}
		for i := range want {
			if args.Checks[i] != want[i] {
				t.Errorf("Checks[%d] = %q, want %q", i, args.Checks[i], want[i])
			}
		}
	})

	t.Run("category and limit", func(t *testing.T) {
		cmd := newAuditCmd()
		if err := cmd.Flags().Set("category", "Docs"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Flags().Set("limit", "5"); err != nil {
			t.Fatal(err)
		}
		args := buildAuditArgs(cmd)
		if args.Category != "Docs" || args.Limit != 5 {
			t.Errorf("got category=%q limit=%d, want Docs/5", args.Category, args.Limit)
		}
	})
}

func TestNewAuditCmd(t *testing.T) {
	cmd := newAuditCmd()
	if cmd.Name() != "audit" {
		t.Errorf("Name = %q, want audit", cmd.Name())
	}
	cwFlagExists(t, cmd, "pages")
	cwFlagDefaultString(t, cmd, "category", "")
	cwFlagDefaultInt(t, cmd, "limit", "20")
	cwFlagDefaultString(t, cmd, "checks", "")
}

// printAuditHuman should not panic on a zero-value result (all sub-printers
// guard nil pointers / zero counts).
func TestPrintAuditHumanZeroValue(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("printAuditHuman panicked on zero result: %v", r)
		}
	}()
	printAuditHuman(&wiki.WikiHealthAuditResult{})
}
