package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// shellQuote returns a single-quoted shell string. Internal single quotes are
// escaped as (end quote, escaped literal quote, reopen quote):
//
//	'\''
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
// sources any post_create hooks, runs the agent command, and captures the
// provider session ID on exit. argusBin is the absolute path to the argus
// binary used for the post-exit set-provider-id call; if empty, falls back
// to "argus" on PATH.
func GenerateInitScript(sessionID, agentCommand, sessionIDPattern, argusBin string, hookPaths []string) string {
	hookBlock := generateHookSourceBlock(hookPaths)
	hookSection := ""
	if hookBlock != "" {
		hookSection = "\n" + hookBlock
	}

	script := "#!/bin/bash\n" +
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
		"# Start the agent (no exec — script continues after exit)\n" +
		agentCommand + "\n"

	// If the provider supports session ID capture, add post-exit logic
	if sessionIDPattern != "" {
		bin := argusBin
		if bin == "" {
			bin = "argus"
		}
		script += "\n" +
			"# Capture provider session ID from terminal output\n" +
			"# -J joins wrapped lines so narrow terminals (e.g. phone) don't truncate the ID\n" +
			"PANE_CONTENT=$(tmux capture-pane -pJ -S -100 2>/dev/null)\n" +
			"PROVIDER_ID=$(echo \"$PANE_CONTENT\" | sed -nE 's/.*" + sessionIDPattern + ".*/\\1/p' | tail -1)\n" +
			"\n" +
			"if [ -n \"$PROVIDER_ID\" ]; then\n" +
			"  '" + bin + "' internal session set-provider-id '" + sessionID + "' \"$PROVIDER_ID\"\n" +
			"fi\n"
	}

	return script
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
		"exec \"${SHELL:-/bin/bash}\" -l\n"
}

// WriteInitScript writes the init script to a temp file and returns the path.
// The sessionID is used to make the filename unique across concurrent calls.
func WriteInitScript(sessionID, agentCommand, sessionIDPattern string, hookPaths []string) (string, error) {
	argusBin, _ := os.Executable()
	content := GenerateInitScript(sessionID, agentCommand, sessionIDPattern, argusBin, hookPaths)
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

// GenerateContainerInitScript returns the script run INSIDE the container to
// launch an agent session: export PATH, source post_create hooks, run the
// agent command. The banner and provider-session-ID capture stay in the host
// wrapper (GenerateInitScript). The script self-deletes on start.
func GenerateContainerInitScript(agentCommand string, hookPaths []string) string {
	hookBlock := generateHookSourceBlock(hookPaths)

	// Build the section between the PATH export and the agent command.
	// When hooks are present: blank line + hook block (which ends with "\n").
	// When hooks are absent: nothing — the trailing "\n" below provides the
	// single blank line separator before the agent command.
	middle := ""
	if hookBlock != "" {
		middle = "\n" + hookBlock
	}

	return "#!/bin/bash\n" +
		"# Argus Container Init Script\n" +
		"# Auto-generated - do not edit manually\n" +
		"rm -f -- \"$0\"\n" +
		"export PATH=\"$HOME/.local/bin:$PATH\"\n" +
		middle +
		"\n" +
		agentCommand + "\n"
}

// GenerateContainerShellInitScript returns the script run INSIDE the container
// for a shell session: source post_create hooks (if any) and exec the login
// shell. Unlike GenerateShellInitScript it always returns a script, because a
// containerized shell session must run through `docker compose exec`.
func GenerateContainerShellInitScript(hookPaths []string) string {
	hookBlock := generateHookSourceBlock(hookPaths)
	hookSection := ""
	if hookBlock != "" {
		hookSection = hookBlock + "\n"
	}
	return "#!/bin/bash\n" +
		"# Argus Container Shell Init Script\n" +
		"# Auto-generated - do not edit manually\n" +
		"rm -f -- \"$0\"\n" +
		hookSection +
		"exec \"${SHELL:-/bin/bash}\" -l\n"
}

// containerScriptDir returns <stateDir>/tmp, creating it 0700. This directory
// is mounted into the container at the same path, so init scripts written here
// are runnable in-container by path.
func containerScriptDir(stateDir string) (string, error) {
	dir := filepath.Join(stateDir, "tmp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create container script dir: %w", err)
	}
	return dir, nil
}

// writeContainerScript writes a generated inner-init script under the mounted
// state tmp dir and returns its path (identical on host and in-container). The
// caller picks content via GenerateContainerInitScript (agent) or
// GenerateContainerShellInitScript (shell).
func writeContainerScript(sessionID, stateDir, content string) (string, error) {
	dir, err := containerScriptDir(stateDir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("argus-inner-%s.sh", sessionID))
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return "", fmt.Errorf("write container init script: %w", err)
	}
	return path, nil
}
