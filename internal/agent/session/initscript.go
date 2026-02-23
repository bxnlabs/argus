package session

import (
	"fmt"
	"os"
	"path/filepath"
)

// GenerateInitScript returns a bash init script that shows the Argus banner,
// configures tmux, and runs the agent command.
func GenerateInitScript(agentCommand string) string {
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
		"\n" +
		"# Start the agent\n" +
		"exec " + agentCommand + "\n"
}

// WriteInitScript writes the init script to a temp file and returns the path.
// The sessionID is used to make the filename unique across concurrent calls.
func WriteInitScript(sessionID, agentCommand string) (string, error) {
	content := GenerateInitScript(agentCommand)
	path := filepath.Join(os.TempDir(), fmt.Sprintf("argus-init-%s.sh", sessionID))
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return "", fmt.Errorf("write init script: %w", err)
	}
	return path, nil
}
