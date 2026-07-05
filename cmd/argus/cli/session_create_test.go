package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderNewOutput_ID(t *testing.T) {
	raw := json.RawMessage(`{"id":"sess_abc","name":"foo"}`)
	var buf bytes.Buffer
	if err := renderNewOutput(&buf, raw, sessionInfo{ID: "sess_abc"}, false); err != nil {
		t.Fatalf("renderNewOutput: %v", err)
	}
	if got := buf.String(); got != "sess_abc\n" {
		t.Errorf("output = %q, want %q", got, "sess_abc\n")
	}
}

func TestRenderNewOutput_JSON(t *testing.T) {
	raw := json.RawMessage(`{"id":"sess_abc","name":"foo"}`)
	var buf bytes.Buffer
	if err := renderNewOutput(&buf, raw, sessionInfo{ID: "sess_abc"}, true); err != nil {
		t.Fatalf("renderNewOutput: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\"id\": \"sess_abc\"") {
		t.Errorf("output = %q, want indented json with id", out)
	}
}
