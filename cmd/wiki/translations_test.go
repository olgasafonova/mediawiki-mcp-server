package main

import "testing"

func TestNewTranslationsCmd(t *testing.T) {
	cmd := newTranslationsCmd()
	if cmd.Name() != "translations" {
		t.Errorf("Name = %q, want translations", cmd.Name())
	}
	cwFlagDefaultString(t, cmd, "base", "")
	cwFlagDefaultString(t, cmd, "category", "")
	cwFlagDefaultString(t, cmd, "languages", "")
	cwFlagDefaultString(t, cmd, "pattern", "subpage")
	cwFlagDefaultInt(t, cmd, "limit", "20")
	// NoArgs
	if err := cwArgsErr(cmd, []string{"x"}); err == nil {
		t.Error("translations: expected error for positional args")
	}
}

func TestRunTranslationsValidation(t *testing.T) {
	t.Run("languages required", func(t *testing.T) {
		cmd := newTranslationsCmd()
		cwGlobalFlags(cmd)
		// base set, languages missing.
		if err := cmd.Flags().Set("base", "Home"); err != nil {
			t.Fatal(err)
		}
		err := runTranslations(cmd, nil)
		if err == nil {
			t.Fatal("expected error when --languages missing")
		}
		if ExitCode(err) != exitUsage {
			t.Errorf("expected usage exit, got %d", ExitCode(err))
		}
	})

	t.Run("base or category required", func(t *testing.T) {
		cmd := newTranslationsCmd()
		cwGlobalFlags(cmd)
		// languages set, neither base nor category.
		if err := cmd.Flags().Set("languages", "en,no"); err != nil {
			t.Fatal(err)
		}
		err := runTranslations(cmd, nil)
		if err == nil {
			t.Fatal("expected error when neither --base nor --category set")
		}
		if ExitCode(err) != exitUsage {
			t.Errorf("expected usage exit, got %d", ExitCode(err))
		}
	})
}
