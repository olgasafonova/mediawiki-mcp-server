package main

import "testing"

func TestDiffSelectorsEmpty(t *testing.T) {
	tests := []struct {
		name string
		sel  diffSelectors
		want bool
	}{
		{"all zero is empty", diffSelectors{}, true},
		{"fromRev set", diffSelectors{fromRev: 5}, false},
		{"toRev set", diffSelectors{toRev: 5}, false},
		{"fromTitle set", diffSelectors{fromTitle: "A"}, false},
		{"toTitle set", diffSelectors{toTitle: "B"}, false},
		{"all set", diffSelectors{1, 2, "A", "B"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.sel.empty(); got != tc.want {
				t.Errorf("empty() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadDiffSelectors(t *testing.T) {
	cmd := newDiffCmd()
	if err := cmd.Flags().Set("from", "10"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("to", "20"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("from-title", "Old"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("to-title", "New"); err != nil {
		t.Fatal(err)
	}

	sel := readDiffSelectors(cmd)
	if sel.fromRev != 10 || sel.toRev != 20 {
		t.Errorf("rev IDs: got from=%d to=%d, want 10/20", sel.fromRev, sel.toRev)
	}
	if sel.fromTitle != "Old" || sel.toTitle != "New" {
		t.Errorf("titles: got from=%q to=%q, want Old/New", sel.fromTitle, sel.toTitle)
	}
	if sel.empty() {
		t.Error("selectors should not be empty after setting flags")
	}
}

func TestReadDiffSelectorsDefaults(t *testing.T) {
	cmd := newDiffCmd()
	sel := readDiffSelectors(cmd)
	if !sel.empty() {
		t.Errorf("default selectors should be empty, got %+v", sel)
	}
}

func TestNewDiffCmd(t *testing.T) {
	cmd := newDiffCmd()
	if cmd.Name() != "diff" {
		t.Errorf("Name = %q, want diff", cmd.Name())
	}
	cwFlagDefaultInt(t, cmd, "from", "0")
	cwFlagDefaultInt(t, cmd, "to", "0")
	cwFlagDefaultString(t, cmd, "from-title", "")
	cwFlagDefaultString(t, cmd, "to-title", "")

	// Args: MaximumNArgs(1) — two args must error, one is fine.
	if err := cwArgsErr(cmd, []string{"a", "b"}); err == nil {
		t.Error("expected error for 2 positional args")
	}
	if err := cwArgsErr(cmd, []string{"a"}); err != nil {
		t.Errorf("unexpected error for 1 arg: %v", err)
	}
}
