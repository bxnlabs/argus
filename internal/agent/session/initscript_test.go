package session

import (
	"strings"
	"testing"
)

func TestGenerateInitScriptWithHooks(t *testing.T) {
	hooks := []string{"/tmp/profiles/work/hooks/post_create.sh", "/tmp/projects/repo/hooks/post_create.sh"}
	script := GenerateInitScript("claude --resume abc", hooks)

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
	// Agent command should still be exec'd
	if !strings.Contains(script, "exec claude --resume abc") {
		t.Error("expected exec agent command")
	}
}

func TestGenerateInitScriptWithoutHooks(t *testing.T) {
	script := GenerateInitScript("claude", nil)
	// Should not contain hook sourcing section
	if strings.Contains(script, "set +e") {
		t.Error("no hooks means no set +e block")
	}
	if !strings.Contains(script, "exec claude") {
		t.Error("expected exec agent command")
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
