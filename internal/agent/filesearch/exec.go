package filesearch

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	fdTimeout               = 5 * time.Second
	maxOutputBuffer   int64 = 1 * 1024 * 1024 // 1MB -- fd outputs paths only
)

// limitedWriter wraps a bytes.Buffer and fails fast when the output exceeds
// a configured limit, preventing unbounded memory growth from large fd output.
type limitedWriter struct {
	buf   bytes.Buffer
	limit int64
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if int64(w.buf.Len())+int64(len(p)) > w.limit {
		return 0, fmt.Errorf("output exceeds %d bytes", w.limit)
	}
	return w.buf.Write(p)
}

// runFd executes fd with the given args in searchDir.
// Sets LC_ALL=C for consistent output. Output is bounded by maxBuffer.
// Exit code 1 (no matches) is NOT treated as an error -- returns empty string.
func runFd(ctx context.Context, searchDir string, maxBuffer int64, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "fd", args...)
	cmd.Dir = searchDir
	cmd.Env = append(os.Environ(), "LC_ALL=C")

	stdout := &limitedWriter{limit: maxBuffer}
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Exit code 1 = no matches, which is expected
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return "", nil
		}
		// Context timeout/cancel: return whatever partial output fd
		// produced before being killed instead of failing the request.
		if ctx.Err() != nil {
			return stdout.buf.String(), nil
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("fd: %s", errMsg)
	}

	return stdout.buf.String(), nil
}

// IsAvailable checks if fd is installed and runnable.
func IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "fd", "--version")
	return cmd.Run() == nil
}
