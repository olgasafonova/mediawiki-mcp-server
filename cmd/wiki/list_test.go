package main

import "testing"

func TestNewListCmdSubcommands(t *testing.T) {
	cmd := newListCmd()
	if cmd.Name() != "list" {
		t.Errorf("Name = %q, want list", cmd.Name())
	}
	for _, sub := range []string{"pages", "categories", "members", "users"} {
		if !cwHasSubcommand(cmd, sub) {
			t.Errorf("list missing subcommand %q", sub)
		}
	}
}

func TestNewListPagesCmdFlags(t *testing.T) {
	cmd := newListPagesCmd()
	cwFlagDefaultInt(t, cmd, "namespace", "0")
	cwFlagDefaultString(t, cmd, "prefix", "")
	cwFlagDefaultInt(t, cmd, "limit", "50")
	cwFlagDefaultString(t, cmd, "continue", "")
}

func TestNewListCategoriesCmdFlags(t *testing.T) {
	cmd := newListCategoriesCmd()
	cwFlagDefaultString(t, cmd, "prefix", "")
	cwFlagDefaultInt(t, cmd, "limit", "50")
	cwFlagDefaultString(t, cmd, "continue", "")
}

func TestNewListMembersCmdArgs(t *testing.T) {
	cmd := newListMembersCmd()
	cwFlagDefaultInt(t, cmd, "limit", "50")
	// ExactArgs(1)
	if err := cwArgsErr(cmd, nil); err == nil {
		t.Error("members: expected error for 0 args")
	}
	if err := cwArgsErr(cmd, []string{"Cat"}); err != nil {
		t.Errorf("members: unexpected error for 1 arg: %v", err)
	}
	if err := cwArgsErr(cmd, []string{"a", "b"}); err == nil {
		t.Error("members: expected error for 2 args")
	}
}

func TestNewListUsersCmdFlags(t *testing.T) {
	cmd := newListUsersCmd()
	cwFlagDefaultString(t, cmd, "group", "")
	cwFlagDefaultInt(t, cmd, "limit", "50")
	cwFlagDefaultString(t, cmd, "active", "false")
	cwFlagDefaultString(t, cmd, "continue", "")
}
