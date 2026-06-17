package session

import (
	"os"
	"path/filepath"
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
	if !strings.Contains(script, `exec "${SHELL:-/bin/bash}" -l`) {
		t.Error("expected exec with SHELL fallback")
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

func TestGenerateContainerInitScript(t *testing.T) {
	hooks := []string{"/home/jeev/.argus/profiles/work/hooks/post_create.sh"}
	script := GenerateContainerInitScript("claude --resume abc", hooks)

	if !strings.HasPrefix(script, "#!/bin/bash") {
		t.Error("expected shebang")
	}
	if !strings.Contains(script, `rm -f -- "$0"`) {
		t.Error("expected self-delete line")
	}
	if !strings.Contains(script, `source '/home/jeev/.argus/profiles/work/hooks/post_create.sh'`) {
		t.Error("expected sourced post_create hook")
	}
	if !strings.Contains(script, "claude --resume abc") {
		t.Error("expected agent command")
	}
	// No banner and no capture — those live in the host wrapper.
	if strings.Contains(script, "Argus Session Init") || strings.Contains(script, "tmux capture-pane") {
		t.Error("container script must not contain banner or capture")
	}
}

func TestGenerateContainerShellInitScript(t *testing.T) {
	// Always returns a script, even with no hooks (a containerized shell must
	// run through docker compose exec).
	script := GenerateContainerShellInitScript(nil)
	if !strings.Contains(script, `exec "${SHELL:-/bin/bash}" -l`) {
		t.Error("expected exec with SHELL fallback")
	}
	withHooks := GenerateContainerShellInitScript([]string{"/h/post_create.sh"})
	if !strings.Contains(withHooks, "source '/h/post_create.sh'") {
		t.Error("expected sourced hook")
	}
}

func TestWriteContainerScript_Agent(t *testing.T) {
	state := t.TempDir()
	content := GenerateContainerInitScript("claude", nil)
	path, err := writeContainerScript("sess_xyz", state, content)
	if err != nil {
		t.Fatal(err)
	}
	// Must live under <stateDir>/tmp so it is visible in the container.
	if !strings.HasPrefix(path, filepath.Join(state, "tmp")) {
		t.Errorf("inner script not under state tmp dir: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "claude") {
		t.Error("expected agent command in written file")
	}
}

func TestWriteContainerScript_Shell(t *testing.T) {
	state := t.TempDir()
	content := GenerateContainerShellInitScript(nil)
	path, err := writeContainerScript("sess_sh", state, content)
	if err != nil {
		t.Fatal(err)
	}
	// Must live under <stateDir>/tmp so it is visible in the container.
	if !strings.HasPrefix(path, filepath.Join(state, "tmp")) {
		t.Errorf("inner shell script not under state tmp dir: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `exec "${SHELL:-/bin/bash}" -l`) {
		t.Error("expected exec with SHELL fallback in written file")
	}
}
