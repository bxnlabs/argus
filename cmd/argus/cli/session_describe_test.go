package cli

import (
	"strings"
	"testing"
)

func strptr(s string) *string { return &s }

func TestFormatSessionDescribe_Full(t *testing.T) {
	s := sessionInfo{
		ID:               "sess_abc123",
		Name:             "my-session",
		CreatedAt:        "2026-05-20 14:32:05",
		UpdatedAt:        "2026-05-28 09:15:00",
		WorkingDirectory: "/home/u/work",
		ProviderType:     "claude",
		AutoApprove:      true,
		Pinned:           true,
		Model:            strptr("claude-opus-4-7"),
		WorktreeBranch:   strptr("jeev/bxn-97"),
		Profile:          strptr("default"),
		GitRemoteURL:     strptr("git@github.com:bxnlabs/argus.git"),
	}
	out := formatSessionDescribe(s, "idle", "/home/u")

	for _, want := range []string{
		"Session: my-session",
		"sess_abc123",
		"Status:", "idle",
		"Pinned:", "yes",
		"Profile:", "default",
		"Type:", "claude",
		"Model:", "claude-opus-4-7",
		"Auto-approve:", "on",
		"Repo:", "bxnlabs/argus",
		"Branch:", "jeev/bxn-97",
		"Created:", "2026-05-20 14:32:05",
		"Updated:", "2026-05-28 09:15:00",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}
}

func TestFormatSessionDescribe_Minimal(t *testing.T) {
	s := sessionInfo{
		ID:               "sess_x",
		Name:             "bare",
		CreatedAt:        "2026-05-20 14:32:05",
		UpdatedAt:        "2026-05-20 14:32:05",
		WorkingDirectory: "/home/u/work",
		ProviderType:     "shell",
	}
	out := formatSessionDescribe(s, "", "/home/u")

	if !strings.Contains(out, "Profile:") || !strings.Contains(out, "none") {
		t.Errorf("expected profile 'none', got:\n%s", out)
	}
	if !strings.Contains(out, "Auto-approve:") || !strings.Contains(out, "off") {
		t.Errorf("expected auto-approve 'off', got:\n%s", out)
	}
	if strings.Contains(out, "Model:") {
		t.Errorf("did not expect a Model line for a session with no model:\n%s", out)
	}
	if strings.Contains(out, "Branch:") {
		t.Errorf("did not expect a Branch line for a session with no worktree:\n%s", out)
	}
	if strings.Contains(out, "Repo:") {
		t.Errorf("did not expect a Repo line for a session with no remote:\n%s", out)
	}
	if !strings.Contains(out, "Status:") || !strings.Contains(out, "-") {
		t.Errorf("expected status '-' when unknown, got:\n%s", out)
	}
}
