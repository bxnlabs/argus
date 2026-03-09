package session

import (
	"strings"
	"testing"
)

func TestGenerateInitScript_WithPattern(t *testing.T) {
	script := GenerateInitScript("sess_abc123", "claude --dangerously-skip-permissions", `claude --resume ([0-9a-f-]+)`)

	// Should NOT use exec to launch the agent (script must continue after exit)
	if strings.Contains(script, "\nexec ") {
		t.Error("script should not use exec when pattern is set")
	}

	// Should contain the agent command
	if !strings.Contains(script, "claude --dangerously-skip-permissions") {
		t.Error("script should contain agent command")
	}

	// Should contain capture logic
	if !strings.Contains(script, "tmux capture-pane") {
		t.Error("script should contain tmux capture-pane")
	}
	if !strings.Contains(script, "argus internal session set-provider-id") {
		t.Error("script should contain argus CLI call")
	}
	if !strings.Contains(script, "sess_abc123") {
		t.Error("script should contain session ID")
	}
}

func TestGenerateInitScript_WithoutPattern(t *testing.T) {
	script := GenerateInitScript("sess_abc123", "claude --dangerously-skip-permissions", "")

	// Should contain the agent command without exec
	if !strings.Contains(script, "claude --dangerously-skip-permissions") {
		t.Error("script should contain agent command")
	}

	// Should NOT contain capture logic
	if strings.Contains(script, "tmux capture-pane") {
		t.Error("script should not contain capture logic without pattern")
	}
}
