package shared

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestTmuxSocketPathHonorsArgusHome(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	got, err := TmuxSocketPath()
	if err != nil {
		t.Fatalf("TmuxSocketPath: %v", err)
	}
	want := filepath.Join("/custom/home", "tmux", "server")
	if got != want {
		t.Errorf("TmuxSocketPath() = %q, want %q", got, want)
	}
}

// homeForSocketLen returns an ARGUS_HOME whose tmux socket path is exactly n
// bytes long, so the boundary tests below don't hand-count separators.
func homeForSocketLen(t *testing.T, n int) string {
	t.Helper()
	suffix := len(filepath.Join("/", "tmux", "server")) // "/tmux/server"
	return "/" + strings.Repeat("a", n-suffix-1)
}

func TestTmuxSocketPathAcceptsPathAtLimit(t *testing.T) {
	home := homeForSocketLen(t, maxTmuxSocketPath)
	t.Setenv("ARGUS_HOME", home)
	got, err := TmuxSocketPath()
	if err != nil {
		t.Fatalf("TmuxSocketPath at the %d-byte limit: %v", maxTmuxSocketPath, err)
	}
	if len(got) != maxTmuxSocketPath {
		t.Fatalf("test bug: built a %d-byte socket path, want %d", len(got), maxTmuxSocketPath)
	}
}

func TestTmuxSocketPathRejectsPathOverLimit(t *testing.T) {
	// One byte over the platform's sockaddr_un capacity. tmux reports this as
	// an opaque "File name too long" that points nowhere near the cause, so
	// TmuxSocketPath rejects it up front with a message naming the limit.
	home := homeForSocketLen(t, maxTmuxSocketPath+1)
	t.Setenv("ARGUS_HOME", home)
	_, err := TmuxSocketPath()
	if err == nil {
		t.Fatalf("TmuxSocketPath with a %d-byte socket path: got nil error, want over-limit error", maxTmuxSocketPath+1)
	}
	for _, want := range []string{
		strconv.Itoa(maxTmuxSocketPath),
		strconv.Itoa(maxTmuxSocketPath + 1),
		"ARGUS_HOME",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestTmuxCommandRejectsPathOverLimit(t *testing.T) {
	// The limit must reach every tmux caller, not just direct TmuxSocketPath users.
	t.Setenv("ARGUS_HOME", homeForSocketLen(t, maxTmuxSocketPath+1))
	if _, err := TmuxCommand("has-session", "-t", "x"); err == nil {
		t.Error("TmuxCommand: got nil error, want over-limit error")
	}
	if _, err := TmuxCommandContext(context.Background(), "list-sessions"); err == nil {
		t.Error("TmuxCommandContext: got nil error, want over-limit error")
	}
}

func TestTmuxConfigPathsHonorArgusHome(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")

	got, err := TmuxUserConfigPath()
	if err != nil {
		t.Fatalf("TmuxUserConfigPath: %v", err)
	}
	if want := filepath.Join("/custom/home", "tmux", "tmux.conf"); got != want {
		t.Errorf("TmuxUserConfigPath() = %q, want %q", got, want)
	}

	got, err = TmuxManagedConfigPath()
	if err != nil {
		t.Fatalf("TmuxManagedConfigPath: %v", err)
	}
	if want := filepath.Join("/custom/home", "tmux", "managed.conf"); got != want {
		t.Errorf("TmuxManagedConfigPath() = %q, want %q", got, want)
	}
}

func TestTmuxCommandThreadsSocket(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	cmd, err := TmuxCommand("has-session", "-t", "x")
	if err != nil {
		t.Fatalf("TmuxCommand: %v", err)
	}
	sock := filepath.Join("/custom/home", "tmux", "server")
	want := []string{"tmux", "-S", sock, "has-session", "-t", "x"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("cmd.Args = %v, want %v", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Errorf("cmd.Args[%d] = %q, want %q", i, cmd.Args[i], want[i])
		}
	}
}

func TestTmuxCommandContextThreadsSocket(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	cmd, err := TmuxCommandContext(context.Background(), "list-sessions")
	if err != nil {
		t.Fatalf("TmuxCommandContext: %v", err)
	}
	sock := filepath.Join("/custom/home", "tmux", "server")
	want := []string{"tmux", "-S", sock, "list-sessions"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("cmd.Args = %v, want %v", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Errorf("cmd.Args[%d] = %q, want %q", i, cmd.Args[i], want[i])
		}
	}
}

func TestEnsureTmuxStateDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	got, err := EnsureTmuxStateDir()
	if err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	want := filepath.Join(dir, "tmux")
	if got != want {
		t.Fatalf("EnsureTmuxStateDir() = %q, want %q", got, want)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat tmux dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("tmux dir perm = %o, want 700", perm)
	}
}

func TestSeedTmuxConfig_WritesWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	if _, err := EnsureTmuxStateDir(); err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	got, err := SeedTmuxConfig()
	if err != nil {
		t.Fatalf("SeedTmuxConfig: %v", err)
	}
	want := filepath.Join(dir, "tmux", "tmux.conf")
	if got != want {
		t.Fatalf("SeedTmuxConfig() = %q, want %q", got, want)
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	// The seed is the user's override file and nothing else. Every setting Argus
	// wants lives in the managed config; shipping a copy here would freeze it at
	// install time, which is the drift this file's contract exists to avoid.
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			t.Errorf("seeded user config carries a directive %q; settings belong in managed.conf\ngot:\n%s", line, data)
		}
	}
}

func TestSeedTmuxConfig_LeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	tmuxDir, err := EnsureTmuxStateDir()
	if err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	// First call creates the config; second hits the EEXIST link path. Neither
	// must leave behind the temp file used for the atomic publish.
	for i := 0; i < 2; i++ {
		if _, err := SeedTmuxConfig(); err != nil {
			t.Fatalf("SeedTmuxConfig (call %d): %v", i+1, err)
		}
	}
	entries, err := os.ReadDir(tmuxDir)
	if err != nil {
		t.Fatalf("read tmux dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "tmux.conf" {
		t.Errorf("tmux dir = %v, want only [tmux.conf] (stray temp file?)", names)
	}
}

func TestSeedTmuxConfig_DoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	if _, err := EnsureTmuxStateDir(); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	if _, err := SeedTmuxConfig(); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	confPath := filepath.Join(dir, "tmux", "tmux.conf")
	custom := "# my custom config\nset -g mouse off\n"
	if err := os.WriteFile(confPath, []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedTmuxConfig(); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	data, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Errorf("SeedTmuxConfig overwrote user config\ngot:\n%s\nwant:\n%s", data, custom)
	}
}

func TestWriteManagedTmuxConfig_CarriesDefaultsInOrder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	if _, err := EnsureTmuxStateDir(); err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	got, err := WriteManagedTmuxConfig()
	if err != nil {
		t.Fatalf("WriteManagedTmuxConfig: %v", err)
	}
	want := filepath.Join(dir, "tmux", "managed.conf")
	if got != want {
		t.Fatalf("WriteManagedTmuxConfig() = %q, want %q", got, want)
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read managed config: %v", err)
	}
	conf := string(data)

	for _, directive := range []string{
		`default-terminal "tmux-256color"`,
		"terminal-overrides",
		"mouse on",
		"history-limit 20000",
		"focus-events on",
		"status off",
	} {
		if !strings.Contains(conf, directive) {
			t.Errorf("managed config missing %q\ngot:\n%s", directive, conf)
		}
	}

	// Precedence is expressed entirely by position: anything before the
	// source-file line is a default the user can override, anything after it is
	// one they cannot. Reordering silently flips which is which.
	source := strings.Index(conf, "source-file")
	if source < 0 {
		t.Fatalf("managed config never sources the user config\ngot:\n%s", conf)
	}
	if i := strings.Index(conf, "history-limit"); i > source {
		t.Errorf("history-limit at %d is after source-file at %d; the user could no longer override it", i, source)
	}
	if i := strings.Index(conf, "status off"); i < source {
		t.Errorf("status off at %d is before source-file at %d; a user config could turn the bar back on", i, source)
	}

	// The path it sources must be the file SeedTmuxConfig writes, or the user's
	// settings are silently ignored.
	userPath, err := TmuxUserConfigPath()
	if err != nil {
		t.Fatalf("TmuxUserConfigPath: %v", err)
	}
	if !strings.Contains(conf, userPath) {
		t.Errorf("managed config does not source %q\ngot:\n%s", userPath, conf)
	}
}

func TestWriteManagedTmuxConfig_OverwritesAndLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	tmuxDir, err := EnsureTmuxStateDir()
	if err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	managedPath, err := WriteManagedTmuxConfig()
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	// Stand in for a config left by an older version: rewriting over it is the
	// whole point of the managed file, so unlike the user's it must not survive.
	stale := "# stale\nset -g history-limit 7\n"
	if err := os.WriteFile(managedPath, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteManagedTmuxConfig(); err != nil {
		t.Fatalf("second write: %v", err)
	}
	data, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "history-limit 7") {
		t.Errorf("stale managed config survived regeneration\ngot:\n%s", data)
	}

	entries, err := os.ReadDir(tmuxDir)
	if err != nil {
		t.Fatalf("read tmux dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "managed.conf" {
		t.Errorf("tmux dir = %v, want only [managed.conf] (stray temp file?)", names)
	}
}

// tmux expands $VAR inside a double-quoted string, so an unescaped one turns
// the sourced path into a different (usually empty) one and the user's config
// is silently skipped. The rest are quoted so a path with spaces or a # does
// not end the argument or start a comment.
func TestQuoteTmuxString(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "/home/u/.argus/tmux/tmux.conf", `"/home/u/.argus/tmux/tmux.conf"`},
		{"space", "/home/u/my dir/tmux.conf", `"/home/u/my dir/tmux.conf"`},
		{"dollar", "/home/u/$HOME/tmux.conf", `"/home/u/\$HOME/tmux.conf"`},
		{"quote", `/home/u/a"b/tmux.conf`, `"/home/u/a\"b/tmux.conf"`},
		{"backslash", `/home/u/a\b/tmux.conf`, `"/home/u/a\\b/tmux.conf"`},
		{"hash", "/home/u/a#b/tmux.conf", `"/home/u/a#b/tmux.conf"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteTmuxString(tt.in); got != tt.want {
				t.Errorf("quoteTmuxString(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

// A node start with no server running is the common case (fresh install, or a
// box whose last session exited). source-file cannot start a server, so this
// must report success rather than failing the caller over an absent one.
//
// It runs real tmux, but only ever against a socket under a throwaway
// ARGUS_HOME, and source-file starts no server — so there is nothing here that
// can reach the user's own tmux sessions.
func TestSourceManagedTmuxConfig_NoServerRunning(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	// A t.TempDir() name can push the derived socket path past sockaddr_un's
	// limit, which TmuxCommand rejects outright; a fixed short name cannot.
	dir, err := os.MkdirTemp("", "argus")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("ARGUS_HOME", dir)
	if _, err := EnsureTmuxStateDir(); err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	if _, err := WriteManagedTmuxConfig(); err != nil {
		t.Fatalf("WriteManagedTmuxConfig: %v", err)
	}
	if err := SourceManagedTmuxConfig(); err != nil {
		t.Errorf("SourceManagedTmuxConfig with no server = %v, want nil", err)
	}
}
