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

func TestSendKeysArgs(t *testing.T) {
	got := sendKeysArgs("claude-sess_abc", []string{"Escape", "C-c"})
	want := []string{"send-keys", "-t", "claude-sess_abc", "Escape", "C-c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
