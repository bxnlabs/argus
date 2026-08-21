package shared

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

	// The append has to be preceded by a reset, and both have to stay ahead of
	// the source line. The managed config is sourced into a live server on every
	// start, and -a with no -u before it grows terminal-overrides by an entry per
	// restart; moving the pair after source-file would instead discard whatever
	// the user set. Asserting only reset < append would miss that second case.
	reset := strings.Index(conf, "set -gu terminal-overrides")
	appended := strings.Index(conf, "set -ga terminal-overrides")
	switch {
	case reset < 0:
		t.Errorf("managed config appends terminal-overrides without resetting it first; re-sourcing will grow it\ngot:\n%s", conf)
	case reset > appended:
		t.Errorf("terminal-overrides reset at %d is after the append at %d, so the append still accumulates", reset, appended)
	case appended > source:
		t.Errorf("terminal-overrides reset/append at %d/%d is after source-file at %d; the reset would wipe the user's own override", reset, appended, source)
	}

	// status off is set on both sides of the user config, and both matter: the
	// copy after it is what makes the bar non-overridable, and the copy before
	// it is what applies when a slow command in the user's config strands the
	// rest of the queue.
	if i := strings.Index(conf, "status off"); i > source {
		t.Errorf("status off first appears at %d, after source-file at %d; a user config that blocks would leave the bar up", i, source)
	}
	if i := strings.LastIndex(conf, "status off"); i < source {
		t.Errorf("status off last appears at %d, before source-file at %d; a user config could turn the bar back on", i, source)
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
// The glob cases are the same failure by a different route: source-file globs
// the path the parser hands it, so a wildcard that reaches glob unescaped makes
// the argument a pattern rather than a filename, and -q swallows the miss.
func TestQuoteTmuxPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "/home/u/.argus/tmux/tmux.conf", `"/home/u/.argus/tmux/tmux.conf"`},
		{"space", "/home/u/my dir/tmux.conf", `"/home/u/my dir/tmux.conf"`},
		{"dollar", "/home/u/$HOME/tmux.conf", `"/home/u/\$HOME/tmux.conf"`},
		{"quote", `/home/u/a"b/tmux.conf`, `"/home/u/a\"b/tmux.conf"`},
		{"hash", "/home/u/a#b/tmux.conf", `"/home/u/a#b/tmux.conf"`},
		// Doubled: tmux's parser consumes one backslash, glob needs the other.
		{"star", "/home/u/a*b/tmux.conf", `"/home/u/a\\*b/tmux.conf"`},
		{"question", "/home/u/a?b/tmux.conf", `"/home/u/a\\?b/tmux.conf"`},
		{"brackets", "/srv/[prod]/tmux.conf", `"/srv/\\[prod\\]/tmux.conf"`},
		// Quadrupled: two survive the parser, and glob spends one escaping the
		// other. Two would leave glob escaping the character that follows.
		{"backslash", `/home/u/a\b/tmux.conf`, `"/home/u/a\\\\b/tmux.conf"`},
		// A backslash beside a metacharacter is where hand-counted escaping goes
		// wrong: both layers have to unwrap to one literal of each.
		{"backslash then star", `/home/u/a\*b/tmux.conf`, `"/home/u/a\\\\\\*b/tmux.conf"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteTmuxPath(tt.in); got != tt.want {
				t.Errorf("quoteTmuxPath(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

// The unit cases above assert the string Argus writes; these assert what tmux
// does with it, which is the only thing that settles how many backslashes a
// layer actually consumes. Each directory name below is a real glob pattern or
// escape once it reaches glob, so without the escaping the user's config is
// skipped and -q says nothing — history-limit would silently stay at Argus's
// default instead of taking the user's value.
func TestManagedTmuxConfig_SourcesAwkwardPaths(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	for _, name := range []string{
		"[prod]",
		"a*b",
		"a?b",
		`a\b`,
		`a\*b`,
		"my dir",
		"a$b",
	} {
		t.Run(name, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "argus")
			if err != nil {
				t.Fatalf("MkdirTemp: %v", err)
			}
			t.Cleanup(func() { os.RemoveAll(dir) })

			holder := filepath.Join(dir, name)
			if err := os.MkdirAll(holder, 0o700); err != nil {
				t.Skipf("cannot create a directory named %q here: %v", name, err)
			}
			userPath := filepath.Join(holder, "tmux.conf")
			if err := os.WriteFile(userPath, []byte("set -g history-limit 4321\n"), 0o600); err != nil {
				t.Fatalf("write user config: %v", err)
			}
			managedPath := filepath.Join(dir, "managed.conf")
			if err := os.WriteFile(managedPath, []byte(managedTmuxConfig(userPath)), 0o600); err != nil {
				t.Fatalf("write managed config: %v", err)
			}

			// The socket sits beside the configs rather than under the awkward
			// directory: this is about the sourced path, and tmux never globs -S.
			sock := filepath.Join(dir, "server")
			run := func(args ...string) (string, error) {
				out, err := exec.Command("tmux", append([]string{"-S", sock}, args...)...).CombinedOutput()
				return strings.TrimSpace(string(out)), err
			}
			if out, err := run("-f", managedPath, "new-session", "-d", "-s", "awkward", "sleep 60"); err != nil {
				t.Fatalf("new-session: %v: %s", err, out)
			}
			t.Cleanup(func() { run("kill-server") })

			got, err := run("show-options", "-gv", "history-limit")
			if err != nil {
				t.Fatalf("show-options: %v: %s", err, got)
			}
			if got != "4321" {
				t.Errorf("history-limit = %q, want %q; tmux did not read the user config at %q", got, "4321", userPath)
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
	bootstrapThrowawayTmux(t)
	if err := SourceManagedTmuxConfig(); err != nil {
		t.Errorf("SourceManagedTmuxConfig with no server = %v, want nil", err)
	}
}

// bootstrapThrowawayTmux points ARGUS_HOME at a throwaway directory and writes
// the configs into it, returning the directory.
//
// A t.TempDir() name can push the derived socket path past sockaddr_un's limit,
// which TmuxCommand rejects outright; a fixed short name cannot.
func bootstrapThrowawayTmux(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	dir, err := os.MkdirTemp("", "argus")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("ARGUS_HOME", dir)
	if _, err := EnsureTmuxStateDir(); err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	if _, err := SeedTmuxConfig(); err != nil {
		t.Fatalf("SeedTmuxConfig: %v", err)
	}
	if _, err := WriteManagedTmuxConfig(); err != nil {
		t.Fatalf("WriteManagedTmuxConfig: %v", err)
	}
	return dir
}

// The integration tests below can only reach the states this box will produce,
// and one of the branches — a socket the node may not open — takes arranging
// permissions a test should not be doing. These are the messages real tmux 3.7c
// emits, checked against the classifier directly.
func TestClassifySourceFileError(t *testing.T) {
	exitErr := errors.New("exit status 1")
	tests := []struct {
		name    string
		ctxErr  error
		out     string
		wantErr bool
		wantIn  string
	}{
		{
			name: "socket absent",
			out:  "error connecting to /tmp/argus/tmux/server (No such file or directory)",
		},
		{
			name: "socket lingers after the server exits",
			out:  "no server running on /tmp/argus/tmux/server",
		},
		{
			name:    "socket exists but cannot be opened",
			out:     "error connecting to /tmp/argus/tmux/server (Permission denied)",
			wantErr: true,
			wantIn:  "Permission denied",
		},
		{
			name:    "config itself is bad",
			out:     "/tmp/argus/tmux/managed.conf:3: unknown command: nonsense",
			wantErr: true,
			wantIn:  "unknown command",
		},
		{
			// Output is whatever the killed client managed to write; the deadline
			// decides, not the text.
			name:    "deadline expired",
			ctxErr:  context.DeadlineExceeded,
			out:     "",
			wantErr: true,
			wantIn:  "blocking",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifySourceFileError(tt.ctxErr, exitErr, tt.out, 5*time.Second)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("classifySourceFileError(%q) = %v, want nil", tt.out, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("classifySourceFileError(%q) = nil, want an error", tt.out)
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("error %q does not mention %q", err, tt.wantIn)
			}
		})
	}

	// The timeout has to stay matchable, or a caller cannot tell it from a
	// config that genuinely failed.
	err := classifySourceFileError(context.DeadlineExceeded, exitErr, "", 5*time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("timeout error %v does not wrap context.DeadlineExceeded", err)
	}
}

// A tmux socket outlives the server that created it, so the ordinary state
// after the last session exits is a socket file with nothing behind it. tmux
// reports that as "no server running" rather than the "error connecting" a
// missing socket produces — treating only the latter as benign would log a
// failure on every node start on an idle box.
func TestSourceManagedTmuxConfig_StaleSocket(t *testing.T) {
	bootstrapThrowawayTmux(t)

	name := "argus-stale"
	cmd, err := TmuxCommand("new-session", "-d", "-s", name, "true")
	if err != nil {
		t.Fatalf("build new-session: %v", err)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v: %s", err, out)
	}

	// The session's command exits immediately; wait for the server to follow it
	// down, leaving the socket behind.
	sock, err := TmuxSocketPath()
	if err != nil {
		t.Fatalf("TmuxSocketPath: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		probe, err := TmuxCommand("has-session", "-t", name)
		if err != nil {
			t.Fatalf("build has-session: %v", err)
		}
		if probe.Run() != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tmux server still up after 10s; cannot reach the stale-socket state")
		}
		time.Sleep(25 * time.Millisecond)
	}
	if _, err := os.Stat(sock); err != nil {
		t.Skipf("socket did not linger after server exit (%v); nothing to test", err)
	}

	if err := SourceManagedTmuxConfig(); err != nil {
		t.Errorf("SourceManagedTmuxConfig against a stale socket = %v, want nil", err)
	}
}

// A tmux config is a command list, and tmux does not return from source-file
// until it has run. Without a bound, a blocking run-shell in the user's config
// hangs node startup outright, so the node's API never binds.
func TestSourceManagedTmuxConfig_BoundedByTimeout(t *testing.T) {
	dir := bootstrapThrowawayTmux(t)

	userPath, err := TmuxUserConfigPath()
	if err != nil {
		t.Fatalf("TmuxUserConfigPath: %v", err)
	}
	if err := os.WriteFile(userPath, []byte("run-shell \"sleep 30\"\n"), 0o600); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	name := "argus-timeout"
	cmd, err := TmuxCommand("new-session", "-d", "-s", name, "sleep 60")
	if err != nil {
		t.Fatalf("build new-session: %v", err)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v: %s", err, out)
	}
	// kill-session is not enough: tmux keeps the server alive while a
	// synchronous run-shell job is outstanding, so the config's sleep would
	// outlive the test.
	t.Cleanup(func() { killThrowawayTmuxServer(t, dir) })

	// start before the deadline is armed, not after: the context's clock would
	// otherwise be running for some microseconds that elapsed never counts, and
	// the lower bound below could fail on a fast machine for no real reason.
	const bound = 500 * time.Millisecond
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()

	err = sourceManagedTmuxConfig(ctx, bound)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("SourceManagedTmuxConfig with a blocking user config = %v, want a context.DeadlineExceeded error", err)
	}
	// Close to the bound, not merely "less than the config's 30s sleep": a
	// version that returned instantly on an unrelated error would otherwise
	// satisfy the assertion and the test would stop proving anything.
	if elapsed < bound {
		t.Errorf("returned in %s, before the %s bound; it cannot have waited on the blocking command", elapsed, bound)
	}
	if elapsed > 10*bound {
		t.Errorf("took %s against a %s bound; the timeout did not bound it", elapsed, bound)
	}
}

// The web UI's layout is built on tmux's status bar being absent, so Argus has
// to hold that even when the config carrying it never finishes. This is the
// ordering the two-sided config alone could not cover: the user turns the bar
// back on and only then blocks, stranding the managed config's closing
// status-off behind a command that may never return.
func TestSourceManagedTmuxConfig_HidesStatusBarDespiteBlockingUserConfig(t *testing.T) {
	dir := bootstrapThrowawayTmux(t)

	userPath, err := TmuxUserConfigPath()
	if err != nil {
		t.Fatalf("TmuxUserConfigPath: %v", err)
	}
	if err := os.WriteFile(userPath, []byte("set -g status on\nrun-shell \"sleep 30\"\n"), 0o600); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	name := "argus-statusbar"
	cmd, err := TmuxCommand("new-session", "-d", "-s", name, "sleep 60")
	if err != nil {
		t.Fatalf("build new-session: %v", err)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v: %s", err, out)
	}
	t.Cleanup(func() { killThrowawayTmuxServer(t, dir) })

	// tmux's own default, and what the user config below asks for. Starting here
	// means "off" at the end can only have come from Argus.
	if set, err := TmuxCommand("set-option", "-g", "status", "on"); err == nil {
		if out, err := set.CombinedOutput(); err != nil {
			t.Fatalf("set status on: %v: %s", err, out)
		}
	}

	const bound = 500 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()
	// It still reports the timeout — the rest of the config really did not
	// apply, and Setup logs that. Only the bar is rescued.
	if err := sourceManagedTmuxConfig(ctx, bound); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("sourceManagedTmuxConfig = %v, want a context.DeadlineExceeded error", err)
	}

	show, err := TmuxCommand("show-options", "-gv", "status")
	if err != nil {
		t.Fatalf("build show-options: %v", err)
	}
	out, err := show.CombinedOutput()
	if err != nil {
		t.Fatalf("show-options: %v: %s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "off" {
		t.Errorf("status = %q, want %q; the fallback did not reach the server past the blocked queue", got, "off")
	}
}

// killThrowawayTmuxServer tears down the whole server behind Argus's socket.
//
// kill-server is the one tmux command a test must never aim wrong: it takes out
// every session on the socket, and a real Argus box runs its work on one. So it
// refuses unless the socket resolves under dir — the specific directory this
// test created — rather than under a temp root, which $TMPDIR could set to
// anything up to and including a parent of the user's real state directory.
func killThrowawayTmuxServer(t *testing.T, dir string) {
	t.Helper()
	sock, err := TmuxSocketPath()
	if err != nil {
		return
	}
	if !strings.HasPrefix(sock, filepath.Clean(dir)+string(filepath.Separator)) {
		t.Fatalf("refusing to kill-server: socket %q is not under the test's dir %q", sock, dir)
	}
	if cmd, err := TmuxCommand("kill-server"); err == nil {
		cmd.Run()
	}
}
