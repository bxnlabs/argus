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

// ComposeUpOpts selects how aggressively `up` refreshes the stack's images.
// The zero value (lazy path) reconciles to the compose file's desired state
// without rebuilding or re-pulling; the explicit refresh path sets both.
type ComposeUpOpts struct {
	Build bool // rebuild local build: images (--build)
	Pull  bool // re-pull registry images (--pull always)
}

// Up brings the stack up in detached mode. `up -d` is idempotent: it leaves
// unchanged running containers alone and recreates ones whose compose config
// drifted (including a dead service). opts force image refresh on top of that.
// Output (including image build/pull progress) is attached to the error on
// failure.
func (CLICompose) Up(ctx context.Context, project, file string, env []string, opts ComposeUpOpts) error {
	args := []string{"-p", project, "-f", file, "up", "-d"}
	if opts.Build {
		args = append(args, "--build")
	}
	if opts.Pull {
		args = append(args, "--pull", "always")
	}
	return run(ctx, env, args...)
}

// Down tears the stack down.
func (CLICompose) Down(ctx context.Context, project, file string, env []string) error {
	return run(ctx, env, "-p", project, "-f", file, "down")
}

func run(ctx context.Context, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return nil
}
