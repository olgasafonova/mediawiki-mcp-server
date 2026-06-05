package main

import (
	"reflect"
	"testing"
)

func TestPlural(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "s"},
		{1, ""},
		{2, "s"},
		{100, "s"},
	}
	for _, tc := range tests {
		if got := plural(tc.n); got != tc.want {
			t.Errorf("plural(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestParseChecks(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string]bool
	}{
		{"single", "terminology", map[string]bool{"terminology": true}},
		{"two", "terminology,links", map[string]bool{"terminology": true, "links": true}},
		{"trims whitespace", " terminology , links ", map[string]bool{"terminology": true, "links": true}},
		{"empty string yields empty key", "", map[string]bool{"": true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseChecks(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseChecks(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseLintOpts(t *testing.T) {
	t.Run("error when no pages and no category", func(t *testing.T) {
		cmd := newLintCmd()
		_, err := parseLintOpts(cmd, nil)
		if err == nil {
			t.Error("expected error when neither pages nor --category given")
		}
	})

	t.Run("pages from args", func(t *testing.T) {
		cmd := newLintCmd()
		opts, err := parseLintOpts(cmd, []string{"PageA", "PageB"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(opts.pages) != 2 || opts.pages[0] != "PageA" {
			t.Errorf("pages = %v, want [PageA PageB]", opts.pages)
		}
		if opts.limit != 20 {
			t.Errorf("default limit = %d, want 20", opts.limit)
		}
		if !opts.checks["terminology"] || !opts.checks["links"] {
			t.Errorf("default checks = %v, want terminology+links", opts.checks)
		}
	})

	t.Run("category alone is valid", func(t *testing.T) {
		cmd := newLintCmd()
		if err := cmd.Flags().Set("category", "API"); err != nil {
			t.Fatal(err)
		}
		opts, err := parseLintOpts(cmd, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.category != "API" {
			t.Errorf("category = %q, want API", opts.category)
		}
	})
}

func TestNewLintCmdFlags(t *testing.T) {
	cmd := newLintCmd()
	if cmd.Name() != "lint" {
		t.Errorf("Name = %q, want lint", cmd.Name())
	}
	cwFlagDefaultString(t, cmd, "category", "")
	cwFlagDefaultString(t, cmd, "glossary-page", "")
	cwFlagDefaultInt(t, cmd, "limit", "20")
	cwFlagDefaultString(t, cmd, "check", "terminology,links")
}
