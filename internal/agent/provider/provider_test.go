package provider

import (
	"strings"
	"testing"
)

func TestAllProviders(t *testing.T) {
	all := All()
	if len(all) != 4 {
		t.Errorf("len = %d, want 4", len(all))
	}
	ids := map[AgentType]bool{}
	for _, p := range all {
		ids[p.ID] = true
	}
	for _, want := range []AgentType{AgentClaude, AgentCodex, AgentGemini, AgentShell} {
		if !ids[want] {
			t.Errorf("missing provider: %s", want)
		}
	}
}

func TestIsValid(t *testing.T) {
	if !IsValid(AgentClaude) {
		t.Error("claude should be valid")
	}
	if IsValid(AgentType("opencode")) {
		t.Error("opencode should not be valid")
	}
}

func TestBuildCommandClaude(t *testing.T) {
	cmd, err := BuildCommand(AgentClaude, BuildCommandOptions{
		AutoApprove: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "claude --dangerously-skip-permissions" {
		t.Errorf("got %q", cmd)
	}
}

func TestBuildCommandClaudeResume(t *testing.T) {
	cmd, err := BuildCommand(AgentClaude, BuildCommandOptions{
		AutoApprove: true,
		SessionID:   "abc123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cmd, "--resume 'abc123'") {
		t.Errorf("got %q, want --resume 'abc123'", cmd)
	}
}

func TestBuildCommandCodex(t *testing.T) {
	cmd, err := BuildCommand(AgentCodex, BuildCommandOptions{
		AutoApprove: true,
		Model:       "gpt-4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cmd, "--approval-mode full-auto") {
		t.Errorf("got %q, want --approval-mode", cmd)
	}
	if !strings.Contains(cmd, "--model 'gpt-4'") {
		t.Errorf("got %q, want --model 'gpt-4'", cmd)
	}
}

func TestBuildCommandGemini(t *testing.T) {
	cmd, err := BuildCommand(AgentGemini, BuildCommandOptions{
		AutoApprove: true,
		Model:       "gemini-pro",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cmd, "--yolomode") {
		t.Errorf("got %q, want --yolomode", cmd)
	}
	if !strings.Contains(cmd, "-m 'gemini-pro'") {
		t.Errorf("got %q, want -m 'gemini-pro'", cmd)
	}
}

func TestBuildCommandShell(t *testing.T) {
	cmd, err := BuildCommand(AgentShell, BuildCommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "" {
		t.Errorf("got %q, want empty", cmd)
	}
}

func TestBuildCommandUnknown(t *testing.T) {
	_, err := BuildCommand(AgentType("unknown"), BuildCommandOptions{})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestBuildCommandClaudeModel(t *testing.T) {
	cmd, err := BuildCommand(AgentClaude, BuildCommandOptions{
		Model: "opus",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cmd, "--model 'opus'") {
		t.Errorf("got %q, want --model 'opus'", cmd)
	}
}

func TestBuildCommandEscapesModelAndSessionID(t *testing.T) {
	cmd, err := BuildCommand(AgentClaude, BuildCommandOptions{
		SessionID: "; rm -rf /",
		Model:     "$(whoami)",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cmd, "'; rm -rf /'") {
		t.Errorf("SessionID not escaped: %q", cmd)
	}
	if !strings.Contains(cmd, "'$(whoami)'") {
		t.Errorf("Model not escaped: %q", cmd)
	}
}

func TestGetSessionIDPattern(t *testing.T) {
	tests := []struct {
		agent   AgentType
		wantSet bool
	}{
		{AgentClaude, true},
		{AgentCodex, true},
		{AgentGemini, true},
		{AgentShell, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.agent), func(t *testing.T) {
			pat := GetSessionIDPattern(tt.agent)
			if tt.wantSet && pat == "" {
				t.Errorf("expected pattern for %s", tt.agent)
			}
			if !tt.wantSet && pat != "" {
				t.Errorf("unexpected pattern for %s: %s", tt.agent, pat)
			}
		})
	}
}

func TestShellEscapeEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", "''"},
		{"consecutive quotes", "'''", `''\'''\'''\'''`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellEscape(tt.input)
			if got != tt.want {
				t.Errorf("shellEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
