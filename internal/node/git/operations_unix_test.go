// syscall.Mkfifo is absent on solaris and illumos, and a runtime t.Skip
// cannot help when the package fails to compile.
//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A plain os.Open on a FIFO blocks until a writer appears — a hang no deadline
// in this package can interrupt, which would wedge the diff poller for good.
func TestCountFileLinesSkipsNonRegularFiles(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(fifo, link); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, path string }{
		{"direct fifo", fifo},
		{"symlink to fifo", link},
	} {
		t.Run(tc.name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() {
				_, err := countFileLines(context.Background(), tc.path)
				done <- err
			}()

			select {
			case err := <-done:
				if err == nil {
					t.Error("expected an error for a non-regular file")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("countFileLines blocked instead of rejecting the file")
			}
		})
	}
}

// The type check must apply to the descriptor that was actually opened. A
// check by path followed by an open by path are two different files whenever
// something rewrites the working tree in between.
func TestOpenRegularValidatesTheDescriptor(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(real, []byte("one\ntwo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("regular file opens and reads", func(t *testing.T) {
		f, err := openRegular(real)
		if err != nil {
			t.Fatalf("expected a regular file to open: %v", err)
		}
		defer f.Close()

		fi, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if !fi.Mode().IsRegular() {
			t.Error("expected the descriptor to report a regular file")
		}
		// The unix path sets O_NONBLOCK; reads through it must still work.
		count, err := countFileLines(context.Background(), real)
		if err != nil || count != 2 {
			t.Errorf("count = %d (%v), want 2", count, err)
		}
	})

	// O_NOFOLLOW: a symlink swapped in for a regular file must not be followed.
	t.Run("symlink is refused", func(t *testing.T) {
		link := filepath.Join(dir, "link.txt")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		f, err := openRegular(link)
		if err == nil {
			f.Close()
			t.Fatal("expected a symlink to be refused")
		}
		if !strings.Contains(err.Error(), "symbolic link") && !strings.Contains(err.Error(), "ELOOP") {
			t.Logf("refused with: %v", err)
		}
	})
}
