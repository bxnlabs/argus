//go:build unix

package files

import (
	"os"
	"syscall"
)

// openForViewer opens a path without letting the path's type block the open.
//
// O_NONBLOCK is the whole point. Opening a FIFO for reading otherwise waits for
// a writer to arrive — forever, if none does — so a descriptor-based type check
// after a plain open is unreachable for exactly the file types it exists to
// reject, and the handler goroutine parks on a path the file tree lists as an
// ordinary file. With the flag the open returns immediately whatever the path
// turns out to be, and the caller decides from the descriptor.
//
// Deliberately NOT a stat-then-open pair: that decides from a name, which
// another process can retarget between the two calls, and it would put a second
// look at the path into a function whose contract is that there is only one.
// O_NONBLOCK has no effect on reads from a regular file, so the bytes this
// returns are read exactly as before.
func openForViewer(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		// Some non-regular files are refused by open(2) itself rather than by
		// the descriptor check — a Unix socket gives ENXIO, not a descriptor to
		// inspect. The tree lists those as ordinary files, so classify them the
		// way an opened one would be classified: a 400 saying what the path is,
		// not a 500 saying the server broke. Statting only to explain a failure
		// decides nothing about a file that opened, so the one-descriptor rule
		// for successful reads is untouched.
		if info, statErr := os.Stat(path); statErr == nil && !info.Mode().IsRegular() {
			return nil, ErrNotRegular
		}
		// Shaped like os.Open's error so the handler's os.ErrNotExist and
		// os.ErrPermission checks keep matching.
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}
