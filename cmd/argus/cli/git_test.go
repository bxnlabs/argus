package cli

import "testing"

func TestGitCmd_HasComments(t *testing.T) {
	git := NewGitCmd()
	found := false
	for _, c := range git.Commands() {
		if c.Name() == "comments" {
			found = true
		}
	}
	if !found {
		t.Fatal("git command missing `comments` subcommand")
	}
}

func TestCommentsCmd_HasView(t *testing.T) {
	comments := newCommentsCmd()
	found := false
	for _, c := range comments.Commands() {
		if c.Name() == "view" {
			found = true
		}
	}
	if !found {
		t.Fatal("comments command missing `view` subcommand")
	}
}

func TestCommentsCmd_HasLs(t *testing.T) {
	comments := newCommentsCmd()
	found := false
	for _, c := range comments.Commands() {
		if c.Name() == "ls" {
			found = true
		}
	}
	if !found {
		t.Fatal("comments command missing `ls` subcommand")
	}
}

func TestCommentsViewCmd_BaseFlagRegistered(t *testing.T) {
	cmd := newCommentsViewCmd()
	f := cmd.Flags().Lookup("base")
	if f == nil {
		t.Fatal("--base flag not registered on `comments view`")
	}
	if f.DefValue != "" {
		t.Errorf("--base default = %q, want empty (preserves auto-detect fallback)", f.DefValue)
	}
}

func TestCommentsLsCmd_BaseFlagRegistered(t *testing.T) {
	cmd := newCommentsLsCmd()
	f := cmd.Flags().Lookup("base")
	if f == nil {
		t.Fatal("--base flag not registered on `comments ls`")
	}
	if f.DefValue != "" {
		t.Errorf("--base default = %q, want empty (preserves auto-detect fallback)", f.DefValue)
	}
}
