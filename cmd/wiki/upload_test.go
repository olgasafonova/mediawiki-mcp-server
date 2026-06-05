package main

import "testing"

func TestNewUploadCmd(t *testing.T) {
	cmd := newUploadCmd()
	if cmd.Name() != "upload" {
		t.Errorf("Name = %q, want upload", cmd.Name())
	}
	cwFlagDefaultString(t, cmd, "file", "")
	cwFlagDefaultString(t, cmd, "url", "")
	cwFlagDefaultString(t, cmd, "text", "")
	cwFlagDefaultString(t, cmd, "comment", "")
	cwFlagDefaultString(t, cmd, "force", "false")
	// ExactArgs(1)
	if err := cwArgsErr(cmd, nil); err == nil {
		t.Error("expected error for 0 args")
	}
}

func TestRunUploadFlagValidation(t *testing.T) {
	t.Run("neither file nor url errors", func(t *testing.T) {
		cmd := newUploadCmd()
		cwGlobalFlags(cmd)
		err := runUpload(cmd, []string{"Logo.png"})
		if err == nil {
			t.Fatal("expected error when neither --file nor --url set")
		}
		if ExitCode(err) != exitUsage {
			t.Errorf("expected usage exit, got %d", ExitCode(err))
		}
	})

	t.Run("file and url are mutually exclusive", func(t *testing.T) {
		cmd := newUploadCmd()
		cwGlobalFlags(cmd)
		if err := cmd.Flags().Set("file", "./x.png"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Flags().Set("url", "https://example.com/x.png"); err != nil {
			t.Fatal(err)
		}
		err := runUpload(cmd, []string{"Logo.png"})
		if err == nil {
			t.Fatal("expected error when both --file and --url set")
		}
		if ExitCode(err) != exitUsage {
			t.Errorf("expected usage exit, got %d", ExitCode(err))
		}
	})

	t.Run("missing local file errors before network", func(t *testing.T) {
		cmd := newUploadCmd()
		cwGlobalFlags(cmd)
		if err := cmd.Flags().Set("file", "/nonexistent/path/to/file.png"); err != nil {
			t.Fatal(err)
		}
		if err := runUpload(cmd, []string{"Logo.png"}); err == nil {
			t.Error("expected error reading nonexistent local file")
		}
	})
}
