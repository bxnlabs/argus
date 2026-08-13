package files

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scriptedSource lets the two stages of classify observe different instants of
// the same file — which is the whole hazard the staging has to survive, and is
// not reproducible against a real file without losing a race on purpose.
type scriptedSource struct {
	seq     []byte // what the sequential reads yield
	off     int
	probeOK bool // whether the byte at the ceiling exists, independent of seq
}

func (s *scriptedSource) Read(p []byte) (int, error) {
	if s.off >= len(s.seq) {
		return 0, io.EOF
	}
	n := copy(p, s.seq[s.off:])
	s.off += n
	return n, nil
}

func (s *scriptedSource) ReadAt(p []byte, _ int64) (int, error) {
	if !s.probeOK {
		return 0, io.EOF
	}
	p[0] = 'x'
	return 1, nil
}

// filler is non-NUL on purpose: a zeroed slice is BINARY by this package's
// heuristic, which would short-circuit these cases before they reach the probe.
func filler(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return b
}

func TestClassifyRaces(t *testing.T) {
	const maxBytes int64 = 20000

	// The probe says the file fits, then it grows before the sequential read.
	// The bound after the probe is what still refuses it — this is the property
	// the old single bounded read provided, and staging must not lose it.
	t.Run("growth after the probe is still refused", func(t *testing.T) {
		src := &scriptedSource{seq: filler(int(maxBytes) + 1), probeOK: false}
		data, isBinary, isLarge, err := classify(src, maxBytes)
		if err != nil {
			t.Fatal(err)
		}
		if !isLarge || isBinary || data != nil {
			t.Errorf("got large=%v binary=%v len(data)=%d, want large with no bytes",
				isLarge, isBinary, len(data))
		}
	})

	// The probe says the file is over the ceiling, then it is truncated: only
	// the sniff prefix is still there. We report large with no bytes rather than
	// handing back a torn partial file. Both statements are true of *some*
	// instant of a file being rewritten; the etag moves with it, so the next
	// poll settles it.
	t.Run("truncation after the probe reports large with no bytes", func(t *testing.T) {
		src := &scriptedSource{seq: filler(sniffSize), probeOK: true}
		data, _, isLarge, err := classify(src, maxBytes)
		if err != nil {
			t.Fatal(err)
		}
		if !isLarge || data != nil {
			t.Errorf("got large=%v len(data)=%d, want large with no bytes", isLarge, len(data))
		}
	})

	// The case a stat-size shortcut gets wrong: a size said large, but by the
	// time bytes are read the file fits. Nothing here trusts a size, so it reads.
	t.Run("a file that fits at probe time is read normally", func(t *testing.T) {
		body := filler(sniffSize + 100)
		src := &scriptedSource{seq: body, probeOK: false}
		data, _, isLarge, err := classify(src, maxBytes)
		if err != nil {
			t.Fatal(err)
		}
		if isLarge {
			t.Error("isLarge = true, want false")
		}
		if string(data) != string(body) {
			t.Errorf("len(data) = %d, want %d", len(data), len(body))
		}
	})

	// The performance property, pinned exactly rather than by timing: once the
	// probe has decided, the ceiling is never read.
	t.Run("reads nothing past a decided verdict", func(t *testing.T) {
		src := &scriptedSource{seq: filler(int(maxBytes) + 1), probeOK: true}
		if _, _, _, err := classify(src, maxBytes); err != nil {
			t.Fatal(err)
		}
		if src.off > sniffSize {
			t.Errorf("consumed %d bytes sequentially, want at most the %d-byte sniff",
				src.off, sniffSize)
		}
	})
}

func TestReadForViewerStagedBoundaries(t *testing.T) {
	dir := t.TempDir()

	write := func(name string, body []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, body, 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// A binary file that is ALSO over the ceiling reports both flags. Returning
	// early on the binary verdict would drop isLarge and silently change the
	// response shape — the client happens to check isBinary first, so the change
	// would be invisible until something else read isLarge.
	t.Run("an oversized binary reports both flags", func(t *testing.T) {
		body := make([]byte, 2*sniffSize)
		body[3] = 0 // NUL inside the sniff window
		for i := range body {
			if i != 3 {
				body[i] = 'a'
			}
		}
		view, err := ReadForViewer(write("big.bin", body), int64(sniffSize), "")
		if err != nil {
			t.Fatal(err)
		}
		if !view.IsBinary || !view.IsLarge {
			t.Errorf("IsBinary=%v IsLarge=%v, want both true", view.IsBinary, view.IsLarge)
		}
		if view.Content != "" {
			t.Errorf("Content = %q, want empty", view.Content)
		}
	})

	// The composition test: the head from stage 1 and the remainder from stage 3
	// must join back into exactly the file. Position-encoded content, because a
	// repeated character cannot tell a dropped head from a duplicated one.
	t.Run("content spanning the sniff boundary is exact", func(t *testing.T) {
		var sb strings.Builder
		for i := 0; sb.Len() < 4*sniffSize; i++ {
			fmt.Fprintf(&sb, "%08d\n", i)
		}
		body := []byte(sb.String())

		view, err := ReadForViewer(write("spanning.txt", body), 1<<20, "")
		if err != nil {
			t.Fatal(err)
		}
		if view.IsLarge || view.IsBinary {
			t.Fatalf("IsLarge=%v IsBinary=%v, want both false", view.IsLarge, view.IsBinary)
		}
		if view.Content != string(body) {
			t.Errorf("content mismatch: got %d bytes, want %d", len(view.Content), len(body))
		}
	})

	// The exact point where the sniff fills but nothing follows it.
	t.Run("a file exactly the sniff size is returned whole", func(t *testing.T) {
		body := []byte(strings.Repeat("a", sniffSize))
		view, err := ReadForViewer(write("exact.txt", body), 1<<20, "")
		if err != nil {
			t.Fatal(err)
		}
		if view.IsLarge || len(view.Content) != sniffSize {
			t.Errorf("IsLarge=%v len(Content)=%d, want false and %d",
				view.IsLarge, len(view.Content), sniffSize)
		}
	})
}
