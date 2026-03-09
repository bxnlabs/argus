package session

import (
	"fmt"
	"os"
	"path/filepath"
)

// GenerateInitScript returns a bash init script that shows the Argus banner,
// runs the agent command, and captures the provider session ID on exit.
func GenerateInitScript(sessionID, agentCommand, sessionIDPattern string) string {
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
		"\n" +
		"# Start the agent (no exec — script continues after exit)\n" +
		agentCommand + "\n"

	// If the provider supports session ID capture, add post-exit logic
	if sessionIDPattern != "" {
		script += "\n" +
			"# Capture provider session ID from terminal output\n" +
			"PANE_CONTENT=$(tmux capture-pane -p -S -100 2>/dev/null)\n" +
			"PROVIDER_ID=$(echo \"$PANE_CONTENT\" | sed -nE 's/.*" + sessionIDPattern + ".*/\\1/p' | tail -1)\n" +
			"\n" +
			"if [ -n \"$PROVIDER_ID\" ]; then\n" +
			"  argus internal session set-provider-id '" + sessionID + "' \"$PROVIDER_ID\" 2>/dev/null\n" +
			"fi\n"
	}

	return script
}

// WriteInitScript writes the init script to a temp file and returns the path.
// The sessionID is used to make the filename unique across concurrent calls.
func WriteInitScript(sessionID, agentCommand, sessionIDPattern string) (string, error) {
	content := GenerateInitScript(sessionID, agentCommand, sessionIDPattern)
	path := filepath.Join(os.TempDir(), fmt.Sprintf("argus-init-%s.sh", sessionID))
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return "", fmt.Errorf("write init script: %w", err)
	}
	return path, nil
}
