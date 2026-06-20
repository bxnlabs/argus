package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/git/worktree"
	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/docker"
	"github.com/bxnlabs/argus/internal/shared"
)

// fakeCompose records calls and tracks per-project up state.
type fakeCompose struct {
	up        map[string]bool
	upCalls   []string
	upOpts    []docker.ComposeUpOpts // opts for each Up call, parallel to upCalls
	downCalls []string
	upErr     error // when set, Up records the call then returns this error
}

func newFakeCompose() *fakeCompose { return &fakeCompose{up: map[string]bool{}} }

func (f *fakeCompose) Up(_ context.Context, project, _ string, _ []string, opts docker.ComposeUpOpts) error {
	f.upCalls = append(f.upCalls, project)
	f.upOpts = append(f.upOpts, opts)
	if f.upErr != nil {
		return f.upErr
	}
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

// TestProfileDownRejectsWhenInUse verifies the live-session guard: a profile
// with a running session can't be torn down (would kill its containers), but
// once the session is gone the teardown proceeds. A dead session row left in
// the DB must not block teardown.
func TestProfileDownRejectsWhenInUse(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	mgr, fake, state := dockerTestManager(t)
	t.Setenv("ARGUS_HOME", state)
	requireDedicatedSocketUnder(t, state)
	if _, err := shared.EnsureTmuxStateDir(); err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	if _, err := shared.SeedTmuxConfig(); err != nil {
		t.Fatalf("SeedTmuxConfig: %v", err)
	}
	makeProfile(t, state, "work", true)

	tmuxName := fmt.Sprintf("docker-inuse-%d", time.Now().UnixNano())
	if err := NewSession(tmuxName, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { KillSession(tmuxName) })

	work := "work"
	if err := mgr.db.CreateSession(&db.Session{
		ID: "sess-inuse", Name: "x", TmuxName: tmuxName,
		WorkingDirectory: state, ProviderType: "shell", Profile: &work,
	}); err != nil {
		t.Fatal(err)
	}

	if err := mgr.ProfileDown("work"); !errors.Is(err, ErrProfileInUse) {
		t.Fatalf("ProfileDown while in use: got %v, want ErrProfileInUse", err)
	}
	if len(fake.downCalls) != 0 {
		t.Errorf("expected no compose down while in use, got %v", fake.downCalls)
	}

	// The session row stays in the DB, but once tmux is gone it no longer
	// counts as live, so teardown proceeds.
	if err := KillSession(tmuxName); err != nil {
		t.Fatalf("KillSession: %v", err)
	}
	if err := mgr.ProfileDown("work"); err != nil {
		t.Fatalf("ProfileDown after session stopped: %v", err)
	}
	if len(fake.downCalls) != 1 {
		t.Errorf("expected 1 down call after session stopped, got %d", len(fake.downCalls))
	}
}

// TestChangeProfileDockerPreflightKeepsSessionAlive is the regression guard for
// the bricking bug: switching to a dockerized profile whose docker preflight
// fails deterministically (cwd not visible in the container) must leave the
// live session alive and the profile unchanged, rather than killing it and
// persisting a profile that can never revive.
func TestChangeProfileDockerPreflightKeepsSessionAlive(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	mgr, fake, state := dockerTestManager(t)
	t.Setenv("ARGUS_HOME", state)
	requireDedicatedSocketUnder(t, state)
	if _, err := shared.EnsureTmuxStateDir(); err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	if _, err := shared.SeedTmuxConfig(); err != nil {
		t.Fatalf("SeedTmuxConfig: %v", err)
	}
	makeProfile(t, state, "work", true)

	// Old (host) profile with a pre_destroy hook that records when it runs, so
	// we can assert teardown does NOT happen when the docker preflight fails.
	makeProfile(t, state, "old", false)
	destroyMarker := filepath.Join(t.TempDir(), "pre_destroy.ran")
	if err := os.WriteFile(filepath.Join(state, "profiles", "old", "hooks", "pre_destroy.sh"),
		[]byte("#!/bin/bash\ntouch "+destroyMarker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A real, existing cwd outside both home and the state dir, so the
	// dockerized target's PathVisible check fails deterministically.
	outsideCwd := t.TempDir()

	tmuxName := fmt.Sprintf("docker-preflight-%d", time.Now().UnixNano())
	if err := NewSession(tmuxName, outsideCwd, ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { KillSession(tmuxName) })
	old := "old"
	if err := mgr.db.CreateSession(&db.Session{
		ID: "sess-pf", Name: "x", TmuxName: tmuxName,
		WorkingDirectory: outsideCwd, ProviderType: "shell", Profile: &old,
	}); err != nil {
		t.Fatal(err)
	}

	work := "work"
	if _, err := mgr.ChangeProfile("sess-pf", &work); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ChangeProfile: got %v, want ErrInvalidInput", err)
	}
	if !HasSession(tmuxName) {
		t.Error("expected session to survive a failed docker preflight (not killed)")
	}
	// The preflight runs before the old profile's pre_destroy teardown, so a
	// failed preflight must leave the old profile's setup intact.
	if _, err := os.Stat(destroyMarker); err == nil {
		t.Error("old profile pre_destroy ran despite failed docker preflight (teardown before validation)")
	}
	s, err := mgr.db.GetSession("sess-pf")
	if err != nil {
		t.Fatal(err)
	}
	if ptrStr(s.Profile) != "old" {
		t.Errorf("profile changed despite preflight failure: %q, want %q", ptrStr(s.Profile), "old")
	}
	// PathVisible fails before ensureStackUp, so the stack was never started.
	if len(fake.upCalls) != 0 {
		t.Errorf("expected no compose up on preflight failure, got %v", fake.upCalls)
	}
}

// A docker `up` failure must surface as ErrStackStart (not a bare error) so the
// API can map it to a clear client message instead of a generic 500. Covers
// both the explicit ProfileUp path and the lazy prepareDockerTarget path.
func TestProfileUpWrapsStackStartFailure(t *testing.T) {
	mgr, fake, state := dockerTestManager(t)
	makeProfile(t, state, "work", true)
	fake.upErr = errors.New("docker compose up -d: exit status 1: daemon unreachable")

	err := mgr.ProfileUp("work")
	if !errors.Is(err, ErrStackStart) {
		t.Fatalf("ProfileUp: got %v, want ErrStackStart", err)
	}
	if !strings.Contains(err.Error(), "daemon unreachable") {
		t.Errorf("error %q should retain the docker failure detail", err)
	}
}

func TestPrepareDockerTargetWrapsStackStartFailure(t *testing.T) {
	mgr, fake, state := dockerTestManager(t)
	makeProfile(t, state, "work", true)
	fake.upErr = errors.New("docker compose up -d: exit status 1: daemon unreachable")

	// state is under the mounted state dir, so PathVisible passes and the lazy
	// ensureStackUp call is reached.
	err := mgr.prepareDockerTarget("work", state)
	if !errors.Is(err, ErrStackStart) {
		t.Fatalf("prepareDockerTarget: got %v, want ErrStackStart", err)
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
	if got["plain"].Type != ProfileTypeHost {
		t.Errorf("plain should be host type: %+v", got["plain"])
	}
	if got["work"].Type != ProfileTypeDocker {
		t.Errorf("work should be docker type: %+v", got["work"])
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
