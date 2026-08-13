//go:build unix

package files

import (
	"errors"
	"net"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO under a browsed directory is listed as an ordinary file, so the viewer
// will happily request its content. Opening one for reading blocks until a
// writer appears — with no writer that is forever, and there is no server write
// timeout to reap the parked goroutine. The open itself is what has to be
// non-blocking (O_NONBLOCK), so this asserts on elapsed time and not only on
// the error: a descriptor check alone would never be reached.
func TestReadForViewerRefusesFIFOWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unsupported here: %v", err)
	}

	type result struct {
		view *FileView
		err  error
	}
	done := make(chan result, 1)
	go func() {
		view, err := ReadForViewer(fifo, 1024, "")
		done <- result{view, err}
	}()

	select {
	case got := <-done:
		if !errors.Is(got.err, ErrNotRegular) {
			t.Errorf("err = %v, want ErrNotRegular", got.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ReadForViewer blocked on a FIFO instead of refusing it")
	}
}

// The open follows symlinks, so a link to a regular file must still read.
func TestReadForViewerFollowsSymlinkToRegularFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "real.txt", "hello")
	target := filepath.Join(dir, "real.txt")

	link := filepath.Join(dir, "link.txt")
	if err := syscall.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported here: %v", err)
	}

	view, err := ReadForViewer(link, 1024, "")
	if err != nil {
		t.Fatalf("ReadForViewer(symlink) = %v, want success", err)
	}
	if view.Content != "hello" {
		t.Errorf("content = %q, want %q", view.Content, "hello")
	}
}

// A Unix socket is refused by open(2) itself rather than by the descriptor
// check, so without classifying the open error it surfaces as a server fault
// instead of "not a regular file". The tree lists sockets as ordinary files, so
// the viewer will ask for one.
func TestReadForViewerRefusesSocket(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "s.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable here: %v", err)
	}
	defer l.Close()

	if _, err := ReadForViewer(sock, 1024, ""); !errors.Is(err, ErrNotRegular) {
		t.Errorf("err = %v, want ErrNotRegular", err)
	}
}
