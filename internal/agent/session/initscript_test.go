package session

import (
	"strings"
	"testing"
)

func TestGenerateInitScriptWithHooks(t *testing.T) {
	hooks := []string{"/tmp/profiles/work/hooks/post_create.sh", "/tmp/projects/repo/hooks/post_create.sh"}
	script := GenerateInitScript("sess_123", "claude --resume abc", "", "", hooks)

	// Should contain guarded source commands with single-quoted paths
	if !strings.Contains(script, "source '/tmp/profiles/work/hooks/post_create.sh'") {
		t.Error("expected profile hook source command")
	}
	if !strings.Contains(script, "source '/tmp/projects/repo/hooks/post_create.sh'") {
		t.Error("expected project hook source command")
	}
	if !strings.Contains(script, "set +e") {
		t.Error("expected set +e guard")
	}
	if !strings.Contains(script, "|| true") {
		t.Error("expected || true guard")
	}
	// Agent command should be present (no exec — script continues)
	if !strings.Contains(script, "claude --resume abc") {
		t.Error("expected agent command")
	}
}

func TestGenerateInitScriptWithoutHooks(t *testing.T) {
	script := GenerateInitScript("sess_123", "claude", "", "", nil)
	// Should not contain hook sourcing section
	if strings.Contains(script, "set +e") {
		t.Error("no hooks means no set +e block")
	}
	if !strings.Contains(script, "claude") {
		t.Error("expected agent command")
	}
}

func TestGenerateInitScript_WithPattern(t *testing.T) {
	script := GenerateInitScript("sess_abc123", "claude --dangerously-skip-permissions", `claude --resume ([0-9a-f-]+)`, "/usr/local/bin/argus", nil)

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
	if !strings.Contains(script, "internal session set-provider-id") {
		t.Error("script should contain argus CLI call")
	}
	if !strings.Contains(script, "sess_abc123") {
		t.Error("script should contain session ID")
	}
	// Should use the absolute binary path
	if !strings.Contains(script, "/usr/local/bin/argus") {
		t.Error("script should contain absolute argus binary path")
	}
}

func TestGenerateInitScript_WithPatternFallbackBin(t *testing.T) {
	script := GenerateInitScript("sess_abc123", "claude --dangerously-skip-permissions", `claude --resume ([0-9a-f-]+)`, "", nil)

	// Should fall back to "argus" when no binary path provided
	if !strings.Contains(script, "'argus' internal session set-provider-id") {
		t.Error("script should fall back to 'argus' when argusBin is empty")
	}
}

func TestGenerateInitScript_WithoutPattern(t *testing.T) {
	script := GenerateInitScript("sess_abc123", "claude --dangerously-skip-permissions", "", "", nil)

	// Should contain the agent command without exec
	if !strings.Contains(script, "claude --dangerously-skip-permissions") {
		t.Error("script should contain agent command")
	}

	// Should NOT contain capture logic
	if strings.Contains(script, "tmux capture-pane") {
		t.Error("script should not contain capture logic without pattern")
	}
}

func TestGenerateInitScript_WithHooksAndPattern(t *testing.T) {
	hooks := []string{"/tmp/profiles/work/hooks/post_create.sh"}
	script := GenerateInitScript("sess_123", "claude", `claude --resume ([0-9a-f-]+)`, "/usr/bin/argus", hooks)

	// Should have both hooks and capture logic
	if !strings.Contains(script, "source '/tmp/profiles/work/hooks/post_create.sh'") {
		t.Error("expected hook source command")
	}
	if !strings.Contains(script, "tmux capture-pane") {
		t.Error("expected capture logic")
	}
}

func TestGenerateShellInitScript(t *testing.T) {
	hooks := []string{"/tmp/profiles/default/hooks/post_create.sh"}
	script := GenerateShellInitScript(hooks)

	if !strings.Contains(script, "source '/tmp/profiles/default/hooks/post_create.sh'") {
		t.Error("expected hook source command")
	}
	if !strings.Contains(script, "exec $SHELL -l") {
		t.Error("expected exec $SHELL -l")
	}
	// Should NOT contain the agent banner
	if strings.Contains(script, "Argus") {
		t.Error("shell init script should not have agent banner")
	}
}

func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	got := shellQuote("/path/with'quote/hook.sh")
	want := `'/path/with'\''quote/hook.sh'`
	if got != want {
		t.Errorf("shellQuote = %q, want %q", got, want)
	}
}

func TestGenerateShellInitScriptNoHooks(t *testing.T) {
	script := GenerateShellInitScript(nil)
	if script != "" {
		t.Errorf("expected empty string when no hooks, got %q", script)
	}
}
