package files

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// sniffSize is how many bytes to read for binary detection.
const sniffSize = 8192

// ViewerMaxBytes is the largest file the viewer will transfer. Past it the
// node reports the size and sends no bytes: Monaco has nothing useful to do
// with a file that big, and the transfer is the expensive part.
const ViewerMaxBytes int64 = 5 << 20 // 5 MB

// ErrNotRegular reports a path that exists but is not a regular file — a
// directory, a device, a socket. Callers map it to a 400.
var ErrNotRegular = errors.New("path is not a regular file")

// IsBinary checks if data contains null bytes (same heuristic as V1).
func IsBinary(data []byte) bool {
	return bytes.ContainsRune(data, 0)
}

// ViewerETag identifies a file version for the poll's unchanged check.
//
// Size and modification time, not a hash of the content: the poll only needs
// to notice that a file moved, and a stat answers that without reading it at
// all. Nanoseconds are what make this safe — it is the same shape as the
// Last-Modified revalidation removed from this path for truncating to whole
// seconds, and only the resolution separates them. The residual blind spot is
// a write landing on both the same size and the same nanosecond, which stays
// invisible until someone hits reload.
func ViewerETag(info os.FileInfo) string {
	return fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano())
}

// ReadForViewer returns a file as the viewer needs it: the bytes, or the
// reason there are none. A `known` etag matching the file's current one comes
// back as Unchanged with no bytes read and none sent.
//
// Everything comes from one open descriptor and never from a second look at
// the path. That is the whole point of the function. Size, the binary verdict
// and the bytes have to describe the same version of the file, and an agent
// rewriting one underneath an open pane is the normal case here — a
// stat-then-open pair can classify version A as small text and then hand back
// version B, which is how a 100MB binary ends up being decoded into an editor.
//
// The read is staged so a verdict costs only the bytes that produced it: the
// sniff prefix decides binary, and a single positional probe at the ceiling
// decides large. IsLarge is never asserted from the stat, only from bytes this
// descriptor actually yielded — at the probe, or in the bounded read after it —
// so a file truncated between the two is refused rather than reported large on
// a size nobody read. The bound survives the probe for the opposite race: a
// file that grows after it is still refused rather than transferred in full.
//
// Size is the size of the version the etag names, not the authority on IsLarge.
// The two can disagree while a file is being rewritten mid-call; the etag moves
// with it, so the next poll settles it.
//
// The two truncation timings resolve differently, deliberately. Truncated
// BEFORE the probe: the probe reports EOF and the bounded read returns whatever
// is there. Truncated AFTER a probe that succeeded: the verdict stands, because
// the descriptor did yield the byte — reporting large with no bytes is a
// truthful statement about that instant, where a partial read would hand back a
// torn file under an etag describing a different one.
//
// Requires maxBytes >= 0.
func ReadForViewer(path string, maxBytes int64, known string) (*FileView, error) {
	// Non-blocking, so that the type check below is actually reachable for the
	// types it rejects — a plain open of a FIFO waits for a writer and never
	// returns. See openForViewer.
	f, err := openForViewer(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrNotRegular
	}

	// Taken before the read, and from the descriptor rather than the path.
	// Both matter. A validator computed AFTER the read can describe a version
	// newer than the bytes being returned; the next poll would match it, report
	// unchanged, and leave the pane on content the node has already superseded
	// with nothing to move it off. Computing it first can only cost one
	// redundant re-read on the next poll, which then self-corrects.
	etag := ViewerETag(info)
	if known != "" && known == etag {
		return &FileView{ETag: etag, Unchanged: true}, nil
	}

	data, isBinary, isLarge, err := classify(f, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return &FileView{
		ETag:     etag,
		Size:     info.Size(),
		IsBinary: isBinary,
		IsLarge:  isLarge,
		// nil unless the bytes are actually being sent.
		Content: string(data),
	}, nil
}

// viewerSource is the open descriptor as the staged read uses it: a sequential
// reader, plus positional probes that do not move its offset. An interface only
// so a test can script growth and truncation between the stages; production
// always passes an *os.File.
type viewerSource interface {
	io.Reader
	io.ReaderAt
}

// classify reads as little as it can and still answer. data is nil whenever the
// answer is "no bytes for you".
func classify(src viewerSource, maxBytes int64) (data []byte, isBinary, isLarge bool, err error) {
	// Never longer than the budget: the ceiling still has to bind on a file that
	// is over it and shorter than sniffSize.
	sniffN := min(int64(sniffSize), maxBytes+1)
	head, err := io.ReadAll(io.LimitReader(src, sniffN))
	if err != nil {
		return nil, false, false, err
	}
	isBinary = IsBinary(head)

	// A short read is EOF, which is where a single bounded ReadAll would have
	// stopped too. Nothing follows, so there is nothing to probe and no more to
	// read.
	if int64(len(head)) < sniffN {
		if isBinary {
			return nil, true, false, nil
		}
		return head, false, false, nil
	}

	// The sniff itself can already settle it: when the ceiling is below
	// sniffSize, a filled head IS more bytes than the ceiling allows, and those
	// bytes came from this descriptor. Asking again would only invite a probe
	// that disagrees with what has already been read.
	isLarge = int64(len(head)) > maxBytes

	if !isLarge {
		// One byte at the ceiling, from the descriptor rather than the stat. A
		// successful read proves the file held maxBytes+1 bytes at this instant;
		// EOF proves it did not. ReadAt is pread(2) — it never moves the offset
		// the sequential read below resumes from.
		//
		// Run even when the sniff already said binary. Skipping it would be one
		// syscall cheaper and would quietly change the answer: an oversized
		// binary reports both flags today, and only one of them if skipped.
		var probe [1]byte
		n, perr := src.ReadAt(probe[:], maxBytes)
		switch {
		// n, not perr: a ReaderAt is permitted to return the byte AND io.EOF
		// together, and the byte is the evidence. *os.File returns (1, nil).
		case n == len(probe):
			isLarge = true
		case errors.Is(perr, io.EOF):
		case perr != nil:
			// The probe is an optimisation, not the authority. A descriptor that
			// serves sequential reads but refuses positional ones (some FUSE and
			// pseudo-files) was readable before this staging existed, and must
			// stay readable: fall through and let the bounded read below decide,
			// exactly as it used to.
		}
	}

	// Neither verdict renders, so the bytes would be dead weight over the wire —
	// and not reading them is the point: a 20GB file now costs one sniff and one
	// byte rather than one ceiling.
	if isBinary || isLarge {
		return nil, isBinary, isLarge, nil
	}

	// Still bounded rather than trusting the probe: a file that grows after it
	// is refused here instead of being transferred in full.
	rest, err := io.ReadAll(io.LimitReader(src, maxBytes+1-int64(len(head))))
	if err != nil {
		return nil, false, false, err
	}
	if int64(len(head)+len(rest)) > maxBytes {
		return nil, false, true, nil
	}
	return append(head, rest...), false, false, nil
}

// ListDirectory lists files and directories, filtering excluded names.
// maxDepth controls recursion depth (V1 uses: recursive ? 2 : 1).
func ListDirectory(dir string, recursive bool, maxDepth int) ([]FileNode, error) {
	return listDir(dir, recursive, maxDepth, 0)
}

func listDir(dir string, recursive bool, maxDepth, depth int) ([]FileNode, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	nodes := []FileNode{}
	for _, entry := range entries {
		if shouldExclude(entry.Name()) {
			continue
		}

		node := FileNode{
			Name: entry.Name(),
			Path: filepath.Join(dir, entry.Name()),
		}

		if entry.IsDir() {
			node.Type = "directory"
			if recursive && depth < maxDepth {
				children, err := listDir(node.Path, true, maxDepth, depth+1)
				if err == nil {
					node.Children = children
				}
				// Silently skip unreadable directories (V1 parity)
			}
		} else {
			node.Type = "file"
			// V1 parity: extension without leading dot
			ext := filepath.Ext(entry.Name())
			if ext != "" {
				node.Extension = strings.ToLower(ext[1:]) // trim leading dot
			}
			if info, err := entry.Info(); err == nil {
				node.Size = info.Size()
			}
		}

		nodes = append(nodes, node)
	}

	// Sort: directories first, then case-insensitive alphabetical
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type != nodes[j].Type {
			return nodes[i].Type == "directory"
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})

	return nodes, nil
}
