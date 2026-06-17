package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/git/worktree"
	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/docker"
)

// fakeCompose records calls and tracks per-project up state.
type fakeCompose struct {
	up        map[string]bool
	upCalls   []string
	upOpts    []docker.ComposeUpOpts // opts for each Up call, parallel to upCalls
	downCalls []string
}

func newFakeCompose() *fakeCompose { return &fakeCompose{up: map[string]bool{}} }

func (f *fakeCompose) Up(_ context.Context, project, _ string, _ []string, opts docker.ComposeUpOpts) error {
	f.upCalls = append(f.upCalls, project)
	f.upOpts = append(f.upOpts, opts)
	f.up[project] = true
	return nil
}
func (f *fakeCompose) Down(_ context.Context, project, _ string, _ []string) error {
	f.downCalls = append(f.downCalls, project)
	f.up[project] = false
	return nil
}

func dockerTestManager(t *testing.T) (*Manager, *fakeCompose, string) {
	t.Helper()
	state := t.TempDir()
	database, err := db.Open(filepath.Join(state, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	wt := worktree.NewManager(state, &config.Config{})
	mgr := NewManager(database, wt, state)
	fake := newFakeCompose()
	mgr.compose = fake
	return mgr, fake, state
}

// makeProfile creates a profile dir with a hooks/ subdir, plus a compose file
// when dockerized.
func makeProfile(t *testing.T, state, name string, dockerized bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(state, "profiles", name, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if dockerized {
		if err := os.WriteFile(filepath.Join(state, "profiles", name, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProfileUpDown(t *testing.T) {
	mgr, fake, state := dockerTestManager(t)
	makeProfile(t, state, "work", true)

	if err := mgr.ProfileUp("work"); err != nil {
		t.Fatalf("ProfileUp: %v", err)
	}
	if !fake.up["argus-work"] {
		t.Error("expected stack up after ProfileUp")
	}
	// `up -d` is idempotent and always runs (no status probe), so a second
	// ProfileUp issues a second up call — both are no-op reconciles.
	if err := mgr.ProfileUp("work"); err != nil {
		t.Fatal(err)
	}
	if len(fake.upCalls) != 2 {
		t.Errorf("expected 2 up calls, got %d", len(fake.upCalls))
	}
	// ProfileUp is the refresh path: it forces image rebuild + re-pull.
	for i, opts := range fake.upOpts {
		if !opts.Build || !opts.Pull {
			t.Errorf("ProfileUp call %d: expected Build+Pull, got %+v", i, opts)
		}
	}

	if err := mgr.ProfileDown("work"); err != nil {
		t.Fatalf("ProfileDown: %v", err)
	}
	if fake.up["argus-work"] {
		t.Error("expected stack down after ProfileDown")
	}
	if len(fake.downCalls) != 1 {
		t.Errorf("expected 1 down call, got %d", len(fake.downCalls))
	}
}

func TestProfileUpRejectsNonDocker(t *testing.T) {
	mgr, _, state := dockerTestManager(t)
	makeProfile(t, state, "plain", false)
	if err := mgr.ProfileUp("plain"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for non-docker profile, got %v", err)
	}
}

func TestProfileUpRejectsMissingProfile(t *testing.T) {
	mgr, _, _ := dockerTestManager(t)
	// No profiles/<name>/hooks dir exists → resolveProfile fails the guard.
	if err := mgr.ProfileUp("ghost"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for missing profile, got %v", err)
	}
}

func TestProfileUpRejectsUppercaseName(t *testing.T) {
	mgr, _, _ := dockerTestManager(t)
	// Uppercase names are now invalid (docker project names must be lowercase).
	if err := mgr.ProfileUp("ClientA"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for uppercase name, got %v", err)
	}
}

func TestListProfilesDetailed(t *testing.T) {
	mgr, _, state := dockerTestManager(t)
	makeProfile(t, state, "plain", false)
	makeProfile(t, state, "work", true)

	infos, err := mgr.ListProfilesDetailed()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ProfileInfo{}
	for _, i := range infos {
		got[i.Name] = i
	}
	if got["plain"].Dockerized {
		t.Errorf("plain should not be dockerized: %+v", got["plain"])
	}
	if !got["work"].Dockerized {
		t.Errorf("work should be dockerized: %+v", got["work"])
	}
}

func TestBuildTmuxCmd_HostAgent(t *testing.T) {
	mgr, _, _ := dockerTestManager(t)
	cmd, err := mgr.buildTmuxCmd("sess_1", "claude", "", t.TempDir(), "claude --resume x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cmd, "bash ") {
		t.Errorf("expected 'bash <script>', got %q", cmd)
	}
}

func TestBuildTmuxCmd_HostShellNoHooks(t *testing.T) {
	mgr, _, _ := dockerTestManager(t)
	// Shell provider (empty agent command) with no hooks → no init script.
	cmd, err := mgr.buildTmuxCmd("sess_2", "shell", "", t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "" {
		t.Errorf("expected empty command for hookless shell session, got %q", cmd)
	}
}

func TestBuildDockerTmuxCmd_AgentSession(t *testing.T) {
	mgr, fake, state := dockerTestManager(t)
	makeProfile(t, state, "work", true)
	cwd := filepath.Join(state, "wt") // under stateDir → visible in container
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd, err := mgr.buildTmuxCmd("sess_d1", "claude", "work", cwd, "claude --resume z", nil)
	if err != nil {
		t.Fatalf("buildTmuxCmd: %v", err)
	}
	if !strings.HasPrefix(cmd, "bash ") {
		t.Fatalf("expected 'bash <host-wrapper>', got %q", cmd)
	}
	// Lazy-up happened.
	if !fake.up["argus-work"] {
		t.Error("expected stack brought up")
	}
	// The inner script was written under the mounted state tmp dir.
	innerPath := filepath.Join(state, "tmp", "argus-inner-sess_d1.sh")
	if _, err := os.Stat(innerPath); err != nil {
		t.Errorf("inner script not written: %v", err)
	}
	// The host wrapper invokes docker compose exec into the agent service.
	hostScript := strings.TrimPrefix(cmd, "bash ")
	data, err := os.ReadFile(hostScript)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"docker compose", "exec", "-p 'argus-work'", "agent", innerPath} {
		if !strings.Contains(string(data), want) {
			t.Errorf("host wrapper missing %q", want)
		}
	}
	// Capture logic present (claude supports resume).
	if !strings.Contains(string(data), "tmux capture-pane") {
		t.Error("expected provider-id capture in host wrapper")
	}
}

func TestBuildDockerTmuxCmd_RejectsInvisibleCwd(t *testing.T) {
	mgr, _, state := dockerTestManager(t)
	makeProfile(t, state, "work", true)
	// A cwd outside both home and the state dir.
	_, err := mgr.buildTmuxCmd("sess_d2", "claude", "work", "/nonexistent-root-xyz/wt", "claude", nil)
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for invisible cwd, got %v", err)
	}
}

func TestBuildDockerTmuxCmd_ShellSession(t *testing.T) {
	mgr, _, state := dockerTestManager(t)
	makeProfile(t, state, "work", true)
	cwd := filepath.Join(state, "sh")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd, err := mgr.buildTmuxCmd("sess_d3", "shell", "work", cwd, "", nil)
	if err != nil {
		t.Fatalf("buildTmuxCmd shell: %v", err)
	}
	hostScript := strings.TrimPrefix(cmd, "bash ")
	data, err := os.ReadFile(hostScript)
	if err != nil {
		t.Fatal(err)
	}
	// Shell session: docker exec present, but no provider-id capture.
	if !strings.Contains(string(data), "docker compose") {
		t.Error("expected docker compose exec for shell session")
	}
	if strings.Contains(string(data), "tmux capture-pane") {
		t.Error("shell session must not capture a provider id")
	}
	innerData, err := os.ReadFile(filepath.Join(state, "tmp", "argus-inner-sess_d3.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(innerData), `exec "${SHELL:-/bin/bash}" -l`) {
		t.Error("expected container shell inner script")
	}
}

// A no-profile session with a profiles/default/ compose file must take the
// dockerized path: "" normalizes to "default", consistent with the hooks layer.
func TestBuildTmuxCmd_NoProfileDockerizedDefault(t *testing.T) {
	mgr, fake, state := dockerTestManager(t)
	makeProfile(t, state, "default", true)
	cwd := filepath.Join(state, "wt")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd, err := mgr.buildTmuxCmd("sess_def", "claude", "", cwd, "claude --resume z", nil)
	if err != nil {
		t.Fatalf("buildTmuxCmd: %v", err)
	}
	// Resolves to the argus-default stack and brings it up.
	if !fake.up["argus-default"] {
		t.Error("expected argus-default stack brought up for no-profile session")
	}
	hostScript := strings.TrimPrefix(cmd, "bash ")
	data, err := os.ReadFile(hostScript)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "-p 'argus-default'") {
		t.Errorf("expected exec into argus-default stack, got:\n%s", string(data))
	}
}
