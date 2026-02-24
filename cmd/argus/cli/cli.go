package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// Run is the entry point for `argus session <subcommand>`.
func Run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}

	switch args[0] {
	case "list":
		return runList(args[1:])
	case "create":
		return runCreate(args[1:])
	case "attach":
		return runAttach(args[1:])
	case "delete":
		return runDelete(args[1:])
	case "rename":
		return runRename(args[1:])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage())
	}
}

func usage() string {
	return `Usage: argus session <command>

Commands:
  list      List all sessions
  create    Create a new session
  attach    Attach to a session's tmux
  delete    Delete a session
  rename    Rename a session`
}

func usageError() error {
	return fmt.Errorf("%s", usage())
}

// die prints an error to stderr and exits.
func die(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

// discoveryFilePath returns the path to the agent discovery file.
// Exits with an error if the home directory cannot be determined.
func discoveryFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		die(fmt.Errorf("cannot determine home directory: %w", err))
	}
	return filepath.Join(home, ".argus", "agent.json")
}
