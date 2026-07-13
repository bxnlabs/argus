package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveSendInput_Inline(t *testing.T) {
	got, err := resolveSendInput("hello", true, "", strings.NewReader(""), false)
	if err != nil {
		t.Fatalf("resolveSendInput: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestResolveSendInput_File(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(p, []byte("from file"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveSendInput("", false, p, strings.NewReader(""), false)
	if err != nil {
		t.Fatalf("resolveSendInput: %v", err)
	}
	if string(got) != "from file" {
		t.Errorf("got %q, want %q", got, "from file")
	}
}

func TestResolveSendInput_Stdin(t *testing.T) {
	got, err := resolveSendInput("", false, "", strings.NewReader("piped"), false)
	if err != nil {
		t.Fatalf("resolveSendInput: %v", err)
	}
	if string(got) != "piped" {
		t.Errorf("got %q, want %q", got, "piped")
	}
}

func TestResolveSendInput_TextAndFileError(t *testing.T) {
	if _, err := resolveSendInput("hello", true, "in.txt", strings.NewReader(""), false); err == nil {
		t.Fatal("expected error when both text and --file are given")
	}
}

func TestResolveSendInput_TTYNoInputError(t *testing.T) {
	if _, err := resolveSendInput("", false, "", strings.NewReader(""), true); err == nil {
		t.Fatal("expected error when no input and stdin is a TTY")
	}
}

func TestLoadBufferArgs(t *testing.T) {
	got := loadBufferArgs("argus-send")
	want := []string{"load-buffer", "-b", "argus-send", "-"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPasteBufferArgs(t *testing.T) {
	got := pasteBufferArgs("argus-send", "claude-sess_abc")
	want := []string{"paste-buffer", "-d", "-p", "-b", "argus-send", "-t", "claude-sess_abc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestKeysModeKeys_NoEnter(t *testing.T) {
	got, err := keysModeKeys("Escape C-c", false)
	if err != nil {
		t.Fatalf("keysModeKeys: %v", err)
	}
	want := []string{"Escape", "C-c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestKeysModeKeys_Enter(t *testing.T) {
	got, err := keysModeKeys("Escape", true)
	if err != nil {
		t.Fatalf("keysModeKeys: %v", err)
	}
	want := []string{"Escape", "Enter"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestKeysModeKeys_EmptyError(t *testing.T) {
	if _, err := keysModeKeys("   ", true); err == nil {
		t.Fatal("expected error when no keys to send")
	}
}

func TestSendKeysArgs(t *testing.T) {
	got := sendKeysArgs("claude-sess_abc", []string{"Escape", "C-c"})
	want := []string{"send-keys", "-t", "claude-sess_abc", "Escape", "C-c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSendCmd_NoArgs(t *testing.T) {
	cmd := newSendCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing session arg")
	}
}

func TestSendCmd_TooManyArgs(t *testing.T) {
	cmd := newSendCmd()
	cmd.SetArgs([]string{"a", "b", "c"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for too many args")
	}
}

// TestSendCmd_TextAndFileConflict verifies the text+--file conflict is rejected
// through the command layer before any node/tmux access.
func TestSendCmd_TextAndFileConflict(t *testing.T) {
	cmd := newSendCmd()
	cmd.SetArgs([]string{"some-session", "hello", "--file", "in.txt"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when both text and --file are given")
	}
}
