package session

import (
	"fmt"
	"os"
	"testing"
)

// TestMain points ARGUS_HOME at a throwaway directory for the whole package, so
// no test can resolve Argus's real tmux socket. Production tmux commands derive
// their socket path from ARGUS_HOME (see shared.TmuxCommand), and a prod Argus
// often runs on the same machine with live sessions — an unisolated test could
// probe or kill them. The directory is removed when the test binary exits.
//
// As a backstop against a tmux command that ever forgets the dedicated -S flag,
// TMUX_TMPDIR also points into the throwaway dir so even a bare `tmux` lands on
// a temp default socket rather than the user's real one, and TMUX is unset so we
// never inherit an outer server. Individual tmux integration tests still override
// ARGUS_HOME with their own t.TempDir() (and assert it via
// requireDedicatedSocketUnder); this baseline protects every other test, current
// and future, by default.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "argus-session-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "session tests: create temp ARGUS_HOME: %v\n", err)
		os.Exit(1)
	}
	os.Setenv("ARGUS_HOME", tmp)
	os.Setenv("TMUX_TMPDIR", tmp)
	os.Unsetenv("TMUX")
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}
