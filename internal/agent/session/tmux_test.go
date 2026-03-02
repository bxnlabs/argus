package session

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func hasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func TestCapturePaneContext_JoinsWrappedLines(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}

	sess := fmt.Sprintf("argus-test-%d", time.Now().UnixNano())

	// Create a narrow (40-column) tmux session.
	cmd := exec.Command("tmux", "new-session", "-d", "-s", sess, "-x", "40", "-y", "10")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v: %s", err, out)
	}
	t.Cleanup(func() {
		exec.Command("tmux", "kill-session", "-t", sess).Run()
	})

	// Send a line longer than the pane width so tmux wraps it.
	longLine := "background tasks still running indicator test line"
	cmd = exec.Command("tmux", "send-keys", "-t", sess, fmt.Sprintf("echo '%s'", longLine), "Enter")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("send-keys: %v: %s", err, out)
	}

	// Give tmux a moment to render.
	time.Sleep(500 * time.Millisecond)

	content, err := CapturePaneContext(context.Background(), sess)
	if err != nil {
		t.Fatalf("CapturePaneContext: %v", err)
	}

	// With -J, the long line should appear as a single logical line containing
	// the full string, not split across multiple physical rows.
	if !strings.Contains(content, longLine) {
		t.Errorf("expected captured content to contain %q as a single logical line\ngot:\n%s", longLine, content)
	}
}
