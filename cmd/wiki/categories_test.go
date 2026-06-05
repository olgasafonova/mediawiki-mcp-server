package main

import (
	"reflect"
	"testing"
)

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty string is nil", "", nil},
		{"single", "Docs", []string{"Docs"}},
		{"two", "Docs,API", []string{"Docs", "API"}},
		{"trims whitespace", " Docs , API ", []string{"Docs", "API"}},
		{"drops empties", "Docs,,API,", []string{"Docs", "API"}},
		{"only commas is nil", ",,,", nil},
		{"only whitespace is nil", "  ,  ", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitCSV(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitCSV(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestNewCategoriesCmd(t *testing.T) {
	cmd := newCategoriesCmd()
	if cmd.Name() != "categories" {
		t.Errorf("Name = %q, want categories", cmd.Name())
	}
	cwFlagDefaultString(t, cmd, "add", "")
	cwFlagDefaultString(t, cmd, "remove", "")
	cwFlagDefaultString(t, cmd, "summary", "")
	cwFlagDefaultString(t, cmd, "preview", "false")
	// ExactArgs(1)
	if err := cwArgsErr(cmd, nil); err == nil {
		t.Error("expected error for 0 args")
	}
}

func TestRunCategoriesRequiresAddOrRemove(t *testing.T) {
	// Neither --add nor --remove: usage error before any network call.
	cmd := newCategoriesCmd()
	cwGlobalFlags(cmd)
	err := runCategories(cmd, []string{"SomePage"})
	if err == nil {
		t.Fatal("expected error when neither --add nor --remove provided")
	}
	if ExitCode(err) != exitUsage {
		t.Errorf("expected usage exit code %d, got %d", exitUsage, ExitCode(err))
	}
}
