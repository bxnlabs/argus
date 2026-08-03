package terminal

import (
	"errors"
	"testing"
	"time"
)

// Compose-mode text must be injected as a bracketed paste so a TUI (e.g. Claude
// Code) receives multiline input as ONE paste event instead of one Enter per
// line. Without this, each embedded newline (normalized to CR) is read as a
// submit, splitting one message into multiple prompts (BXN-110).

func TestComposePasteBlockMultilineWrapsBracketedPaste(t *testing.T) {
	got := string(composePasteBlock("line1\nline2\nline3"))
	want := "\x1b[200~line1\rline2\rline3\x1b[201~"
	if got != want {
		t.Fatalf("multiline block\n got: %q\nwant: %q", got, want)
	}
}

func TestComposePasteBlockNormalizesCRLFAndCR(t *testing.T) {
	got := string(composePasteBlock("a\r\nb\nc"))
	want := "\x1b[200~a\rb\rc\x1b[201~"
	if got != want {
		t.Fatalf("normalization\n got: %q\nwant: %q", got, want)
	}
}

// The paste block must NEVER contain the submit Return. The Return is delivered
// as a separate, delayed write (see injectCompose). If the Return is appended
// to the block it coalesces into the same PTY read/parser feed as the closing
// marker, and a bracketed-paste-aware TUI applies the paste asynchronously and
// processes the Return in the same tick against stale input — so the Return is
// swallowed and nothing submits (the intermittent "Enter didn't send" bug).
func TestComposePasteBlockNeverContainsReturn(t *testing.T) {
	for _, in := range []string{"hello", "a\nb", "trailing\n", ""} {
		block := string(composePasteBlock(in))
		if block == "" {
			continue
		}
		if got := block[len(block)-len("\x1b[201~"):]; got != "\x1b[201~" {
			t.Fatalf("block for %q must end at the close marker, ended with %q", in, got)
		}
		for i := 0; i < len(block); i++ {
			if block[i] == '\r' && i > len("\x1b[200~")-1 {
				// CRs are only legal as in-paste line separators, never after
				// the close marker.
				if i >= len(block)-len("\x1b[201~") {
					t.Fatalf("block for %q contains a CR at/after the close marker: %q", in, block)
				}
			}
		}
	}
}

// recordingWriter captures each Write call as a distinct entry so tests can
// assert that the submit Return is a SEPARATE write from the paste block.
type recordingWriter struct{ writes [][]byte }

func (w *recordingWriter) Write(p []byte) (int, error) {
	cp := make([]byte, len(p))
	copy(cp, p)
	w.writes = append(w.writes, cp)
	return len(p), nil
}

func TestInjectComposeSubmitWritesReturnSeparately(t *testing.T) {
	w := &recordingWriter{}
	slept := false
	sleep := func(time.Duration) { slept = true }

	if err := injectCompose(w, sleep, "line1\nline2", true); err != nil {
		t.Fatalf("injectCompose: %v", err)
	}

	if len(w.writes) != 2 {
		t.Fatalf("submit must produce exactly 2 writes (paste block, then Return), got %d: %q", len(w.writes), w.writes)
	}
	if string(w.writes[0]) != "\x1b[200~line1\rline2\x1b[201~" {
		t.Fatalf("first write must be the paste block, got %q", w.writes[0])
	}
	if string(w.writes[1]) != "\r" {
		t.Fatalf("second write must be the lone submit Return, got %q", w.writes[1])
	}
	if !slept {
		t.Fatalf("submit must delay before writing the Return so it lands in a separate PTY read")
	}
}

func TestInjectComposeNoSubmitWritesOnlyBlock(t *testing.T) {
	w := &recordingWriter{}
	if err := injectCompose(w, func(time.Duration) {}, "hello", false); err != nil {
		t.Fatalf("injectCompose: %v", err)
	}
	if len(w.writes) != 1 {
		t.Fatalf("no-submit must produce exactly 1 write, got %d: %q", len(w.writes), w.writes)
	}
	if string(w.writes[0]) != "\x1b[200~hello\x1b[201~" {
		t.Fatalf("write must be the paste block only, got %q", w.writes[0])
	}
}

// orderRecorder records exitPaneMode calls and PTY writes in one ordered log so
// tests can assert the copy-mode cancel happens BEFORE the paste block.
type orderRecorder struct{ events []string }

func (r *orderRecorder) Write(p []byte) (int, error) {
	r.events = append(r.events, "write:"+string(p))
	return len(p), nil
}

func TestHandleTextMessageCancelsPaneModeBeforePaste(t *testing.T) {
	r := &orderRecorder{}
	exitMode := func() error {
		r.events = append(r.events, "exit")
		return nil
	}

	if err := handleTextMessage(r, func(time.Duration) {}, exitMode, "hello", true); err != nil {
		t.Fatalf("handleTextMessage: %v", err)
	}

	if len(r.events) != 3 {
		t.Fatalf("want 3 events (exit, paste, return), got %d: %v", len(r.events), r.events)
	}
	if r.events[0] != "exit" {
		t.Fatalf("copy-mode cancel must come first, got %q", r.events[0])
	}
	if r.events[1] != "write:\x1b[200~hello\x1b[201~" {
		t.Fatalf("second event must be the paste block, got %q", r.events[1])
	}
	if r.events[2] != "write:\r" {
		t.Fatalf("third event must be the submit Return, got %q", r.events[2])
	}
}

func TestHandleTextMessageIgnoresExitPaneModeError(t *testing.T) {
	r := &orderRecorder{}
	exitMode := func() error { return errors.New("no current mode") }

	if err := handleTextMessage(r, func(time.Duration) {}, exitMode, "hello", false); err != nil {
		t.Fatalf("a failing cancel must not fail the send: %v", err)
	}
	if len(r.events) != 1 || r.events[0] != "write:\x1b[200~hello\x1b[201~" {
		t.Fatalf("paste must still be written, got %v", r.events)
	}
}

func TestHandleTextMessageSkipsNilExitMode(t *testing.T) {
	r := &orderRecorder{}

	if err := handleTextMessage(r, func(time.Duration) {}, nil, "hello", false); err != nil {
		t.Fatalf("handleTextMessage: %v", err)
	}
	if len(r.events) != 1 || r.events[0] != "write:\x1b[200~hello\x1b[201~" {
		t.Fatalf("nil exitMode must write only the paste block, got %v", r.events)
	}
}

func TestNewExitPaneModeNilForRawShellRoute(t *testing.T) {
	// The raw-shell route (/ws/terminal) has no tmux session, so there is no
	// pane to cancel.
	if newExitPaneMode("") != nil {
		t.Fatal("empty session name must yield a nil exitMode")
	}
	if newExitPaneMode("argus-abc123") == nil {
		t.Fatal("named session must yield a non-nil exitMode")
	}
}
