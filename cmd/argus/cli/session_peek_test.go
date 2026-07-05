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
