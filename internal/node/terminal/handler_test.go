package terminal

import "testing"

// Compose-mode text must be injected as a bracketed paste so a TUI (e.g. Claude
// Code) receives multiline input as ONE paste event instead of one Enter per
// line. Without this, each embedded newline (normalized to CR) is read as a
// submit, splitting one message into multiple prompts (BXN-110).

func TestComposePayloadMultilineWrapsBracketedPaste(t *testing.T) {
	got := string(composePayload("line1\nline2\nline3", false))
	want := "\x1b[200~line1\rline2\rline3\x1b[201~"
	if got != want {
		t.Fatalf("multiline payload\n got: %q\nwant: %q", got, want)
	}
}

func TestComposePayloadSubmitAppendsReturnOutsidePaste(t *testing.T) {
	got := string(composePayload("line1\nline2", true))
	want := "\x1b[200~line1\rline2\x1b[201~\r"
	if got != want {
		t.Fatalf("submit payload\n got: %q\nwant: %q", got, want)
	}
	// The submit CR must fall OUTSIDE the closing marker so the TUI registers
	// it as a distinct Return keypress rather than part of the pasted text.
	if got[len(got)-len("\x1b[201~\r"):] != "\x1b[201~\r" {
		t.Fatalf("submit CR must come after the close marker, got %q", got)
	}
}

func TestComposePayloadNormalizesCRLFAndCR(t *testing.T) {
	got := string(composePayload("a\r\nb\nc", false))
	want := "\x1b[200~a\rb\rc\x1b[201~"
	if got != want {
		t.Fatalf("normalization\n got: %q\nwant: %q", got, want)
	}
}

func TestComposePayloadSingleLineNoSubmit(t *testing.T) {
	got := string(composePayload("hello", false))
	want := "\x1b[200~hello\x1b[201~"
	if got != want {
		t.Fatalf("single line\n got: %q\nwant: %q", got, want)
	}
}
