package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// shellQuote returns a single-quoted shell string. Internal single quotes
// are escaped as '\'' (end quote, escaped literal quote, reopen quote).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// generateHookSourceBlock builds a bash snippet that sources each hook path
// with error guards (set +e, || true) so that hook failures are non-fatal.
// Returns an empty string when hookPaths is empty.
func generateHookSourceBlock(hookPaths []string) string {
	if len(hookPaths) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Source post_create hooks (errors are non-fatal)\n")
	b.WriteString("set +e\n")
	for _, p := range hookPaths {
		fmt.Fprintf(&b, "source %s 2>&1 || true\n", shellQuote(p))
	}
	b.WriteString("set -e\n")
	return b.String()
}

// GenerateInitScript returns a bash init script that shows the Argus banner,
// configures tmux, sources any post_create hooks, and runs the agent command.
func GenerateInitScript(agentCommand string, hookPaths []string) string {
	hookBlock := generateHookSourceBlock(hookPaths)
	hookSection := ""
	if hookBlock != "" {
		hookSection = "\n" + hookBlock
	}

	// Use fmt.Sprintf with double-quoted Go string since the script contains backticks
	// that would conflict with Go raw string literals.
	return "#!/bin/bash\n" +
		"# Argus Session Init Script\n" +
		"# Auto-generated - do not edit manually\n" +
		"\n" +
		"# Self-cleanup: remove this temp script\n" +
		"rm -f -- \"$0\"\n" +
		"\n" +
		"# ANSI Colors (purple theme)\n" +
		"C_RESET=$'\\033[0m'\n" +
		"C_PURPLE=$'\\033[38;5;141m'\n" +
		"C_PURPLE2=$'\\033[38;5;177m'\n" +
		"C_PINK=$'\\033[38;5;213m'\n" +
		"\n" +
		"# Clear and show banner\n" +
		"clear\n" +
		"\n" +
		"printf \"\\n\"\n" +
		"printf \"${C_PURPLE}       _${C_RESET}\\n\"\n" +
		"printf \"${C_PURPLE}      / \\\\   _ __ __ _ _   _ ___${C_RESET}\\n\"\n" +
		"printf \"${C_PURPLE2}     / _ \\\\ | '__/ _\\` | | | / __|${C_RESET}\\n\"\n" +
		"printf \"${C_PURPLE2}    / ___ \\\\| | | (_| | |_| \\\\__ \\\\${C_RESET}\\n\"\n" +
		"printf \"${C_PINK}   /_/   \\\\_\\\\_|  \\\\__, |\\\\__,_|___/${C_RESET}\\n\"\n" +
		"printf \"${C_PINK}                |___/${C_RESET}\\n\"\n" +
		"printf \"\\n\"\n" +
		"\n" +
		"# Brief pause to show banner\n" +
		"sleep 0.8\n" +
		"\n" +
		"# Ensure ~/.local/bin is in PATH (where claude is installed)\n" +
		"export PATH=\"$HOME/.local/bin:$PATH\"\n" +
		hookSection +
		"\n" +
		"# Start the agent\n" +
		"exec " + agentCommand + "\n"
}

// GenerateShellInitScript returns a bash init script that sources post_create
// hooks and then exec's the user's login shell. Returns an empty string when
// there are no hooks to source.
func GenerateShellInitScript(hookPaths []string) string {
	hookBlock := generateHookSourceBlock(hookPaths)
	if hookBlock == "" {
		return ""
	}

	return "#!/bin/bash\n" +
		"# Shell Session Init Script\n" +
		"# Auto-generated - do not edit manually\n" +
		"\n" +
		"# Self-cleanup: remove this temp script\n" +
		"rm -f -- \"$0\"\n" +
		"\n" +
		hookBlock +
		"\n" +
		"exec $SHELL -l\n"
}

// WriteInitScript writes the init script to a temp file and returns the path.
// The sessionID is used to make the filename unique across concurrent calls.
func WriteInitScript(sessionID, agentCommand string, hookPaths []string) (string, error) {
	content := GenerateInitScript(agentCommand, hookPaths)
	path := filepath.Join(os.TempDir(), fmt.Sprintf("argus-init-%s.sh", sessionID))
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return "", fmt.Errorf("write init script: %w", err)
	}
	return path, nil
}

// WriteShellInitScript writes a shell init script that sources hooks and
// exec's the user's login shell. Returns an empty path when there are no hooks.
func WriteShellInitScript(sessionID string, hookPaths []string) (string, error) {
	content := GenerateShellInitScript(hookPaths)
	if content == "" {
		return "", nil
	}
	path := filepath.Join(os.TempDir(), fmt.Sprintf("argus-shell-init-%s.sh", sessionID))
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return "", fmt.Errorf("write shell init script: %w", err)
	}
	return path, nil
}
