package search

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
	searchTimeout             = 10 * time.Second
	maxOutputBuffer     int64 = 5 * 1024 * 1024 // 5MB
	maxResultsHardLimit       = 100
)

// limitedWriter wraps a bytes.Buffer and fails fast when the output exceeds
// a configured limit, preventing unbounded memory growth from large ripgrep output.
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

// runRipgrep executes ripgrep in dir with the given args.
// Sets LC_ALL=C for consistent output. Output is bounded by maxBuffer.
// Exit code 1 (no matches) is NOT treated as an error — returns empty string.
func runRipgrep(ctx context.Context, dir string, maxBuffer int64, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = dir
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
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("rg: %s", errMsg)
	}

	return stdout.buf.String(), nil
}
