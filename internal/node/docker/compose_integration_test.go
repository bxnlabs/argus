package docker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLICompose_RoundTrip(t *testing.T) {
	if os.Getenv("ARGUS_DOCKER_IT") != "1" {
		t.Skip("set ARGUS_DOCKER_IT=1 to run docker integration test")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	dir := t.TempDir()
	compose := filepath.Join(dir, "docker-compose.yml")
	// A minimal keep-alive service named like the agent service.
	content := "services:\n  agent:\n    image: busybox\n    command: sleep 300\n"
	if err := os.WriteFile(compose, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewCLICompose()
	project := "argus-it-roundtrip"
	env := Env("/tmp", dir)
	ctx := context.Background()

	t.Cleanup(func() { _ = c.Down(ctx, project, compose, env) })

	// running reports whether any service container of the stack is running,
	// via the docker CLI directly (the package no longer exposes a status probe).
	running := func() bool {
		out, err := exec.Command("docker", "compose", "-p", project, "-f", compose, "ps", "--status", "running", "-q").Output()
		if err != nil {
			t.Fatalf("docker compose ps: %v", err)
		}
		return len(strings.TrimSpace(string(out))) > 0
	}

	if err := c.Up(ctx, project, compose, env, ComposeUpOpts{}); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if !running() {
		t.Fatal("expected stack to be up")
	}
	if err := c.Down(ctx, project, compose, env); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if running() {
		t.Fatal("expected stack to be down")
	}
}
