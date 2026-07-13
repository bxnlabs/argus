package cli

import (
	"reflect"
	"testing"
)

func TestCapturePaneArgs_Visible(t *testing.T) {
	got := capturePaneArgs("claude-sess_abc", false)
	want := []string{"capture-pane", "-p", "-t", "claude-sess_abc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCapturePaneArgs_All(t *testing.T) {
	got := capturePaneArgs("claude-sess_abc", true)
	want := []string{"capture-pane", "-p", "-S", "-", "-t", "claude-sess_abc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSliceLines_Head(t *testing.T) {
	got, err := sliceLines("a\nb\nc\nd\n", 2, 0)
	if err != nil {
		t.Fatalf("sliceLines: %v", err)
	}
	if got != "a\nb\n" {
		t.Errorf("got %q, want %q", got, "a\nb\n")
	}
}

func TestSliceLines_Tail(t *testing.T) {
	got, err := sliceLines("a\nb\nc\nd\n", 0, 2)
	if err != nil {
		t.Fatalf("sliceLines: %v", err)
	}
	if got != "c\nd\n" {
		t.Errorf("got %q, want %q", got, "c\nd\n")
	}
}

func TestSliceLines_BothError(t *testing.T) {
	if _, err := sliceLines("a\nb\n", 1, 1); err == nil {
		t.Fatal("expected error when both head and tail are set")
	}
}

func TestSliceLines_None(t *testing.T) {
	got, err := sliceLines("a\nb\n", 0, 0)
	if err != nil {
		t.Fatalf("sliceLines: %v", err)
	}
	if got != "a\nb\n" {
		t.Errorf("got %q, want %q", got, "a\nb\n")
	}
}

func TestSliceLines_HeadBeyondLen(t *testing.T) {
	got, err := sliceLines("a\nb\n", 10, 0)
	if err != nil {
		t.Fatalf("sliceLines: %v", err)
	}
	if got != "a\nb\n" {
		t.Errorf("got %q, want %q", got, "a\nb\n")
	}
}

// TestSliceLines_NoTrailingNewline verifies that a slice of input without a
// trailing newline does not gain one — the doc contract preserves the input's
// trailing newline rather than always appending it.
func TestSliceLines_NoTrailingNewline(t *testing.T) {
	head, err := sliceLines("a\nb\nc", 2, 0)
	if err != nil {
		t.Fatalf("sliceLines head: %v", err)
	}
	if head != "a\nb" {
		t.Errorf("head got %q, want %q", head, "a\nb")
	}
	tail, err := sliceLines("a\nb\nc", 0, 2)
	if err != nil {
		t.Fatalf("sliceLines tail: %v", err)
	}
	if tail != "b\nc" {
		t.Errorf("tail got %q, want %q", tail, "b\nc")
	}
}

func TestPeekCmd_HeadTailConflict(t *testing.T) {
	cmd := newPeekCmd()
	cmd.SetArgs([]string{"some-session", "--head", "1", "--tail", "1"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when --head and --tail are used together")
	}
}

func TestPeekCmd_NoArgs(t *testing.T) {
	cmd := newPeekCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing session arg")
	}
}
