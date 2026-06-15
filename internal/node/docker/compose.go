package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CLICompose runs the `docker compose` CLI against a profile's stack.
type CLICompose struct{}

// NewCLICompose returns a CLICompose.
func NewCLICompose() *CLICompose { return &CLICompose{} }

// Up brings the stack up in detached mode. Output (including image build/pull
// progress) is attached to the error on failure.
func (CLICompose) Up(ctx context.Context, project, file string, env []string) error {
	return run(ctx, env, "-p", project, "-f", file, "up", "-d")
}

// Down tears the stack down.
func (CLICompose) Down(ctx context.Context, project, file string, env []string) error {
	return run(ctx, env, "-p", project, "-f", file, "down")
}

// IsUp reports whether any service container of the stack is running.
func (CLICompose) IsUp(ctx context.Context, project, file string, env []string) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "-p", project, "-f", file, "ps", "--status", "running", "-q")
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("docker compose ps: %w", withStderr(err))
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func run(ctx context.Context, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return nil
}

// withStderr appends an *exec.ExitError's captured stderr to the error text.
func withStderr(err error) error {
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
	}
	return err
}
