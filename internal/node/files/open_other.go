//go:build !unix

package files

import "os"

// openForViewer without O_NONBLOCK, which non-Unix platforms do not offer for
// this purpose. No Argus node target lands here today; if one ever does, a FIFO
// on that platform can still block the open, and this is where that is fixed.
func openForViewer(path string) (*os.File, error) {
	return os.Open(path)
}
