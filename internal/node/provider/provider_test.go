package provider

import (
	"strings"
	"testing"
)

func TestAllProviders(t *testing.T) {
	all := All()
	if len(all) != 3 {
		t.Errorf("len = %d, want 3", len(all))
	}
	ids := map[ProviderType]bool{}
	for _, p := range all {
		ids[p.ID] = true
	}
	for _, want := range []ProviderType{ProviderClaude, ProviderCodex, ProviderShell} {
		if !ids[want] {
			t.Errorf("missing provider: %s", want)
		}
	}
}

func TestIsValid(t *testing.T) {
	if !IsValid(ProviderClaude) {
		t.Error("claude should be valid")
	}
	if IsValid(ProviderType("opencode")) {
		t.Error("opencode should not be valid")
	}
}

func TestBuildCommandClaude(t *testing.T) {
	cmd, err := BuildCommand(ProviderClaude, BuildCommandOptions{
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
	cmd, err := BuildCommand(ProviderClaude, BuildCommandOptions{
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
	cmd, err := BuildCommand(ProviderCodex, BuildCommandOptions{
		AutoApprove: true,
		Model:       "gpt-4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cmd, "--dangerously-bypass-approvals-and-sandbox") {
		t.Errorf("got %q, want --dangerously-bypass-approvals-and-sandbox", cmd)
	}
	if !strings.Contains(cmd, "--model 'gpt-4'") {
		t.Errorf("got %q, want --model 'gpt-4'", cmd)
	}
}

func TestBuildCommandShell(t *testing.T) {
	cmd, err := BuildCommand(ProviderShell, BuildCommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "" {
		t.Errorf("got %q, want empty", cmd)
	}
}

func TestBuildCommandUnknown(t *testing.T) {
	_, err := BuildCommand(ProviderType("unknown"), BuildCommandOptions{})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestBuildCommandClaudeModel(t *testing.T) {
	cmd, err := BuildCommand(ProviderClaude, BuildCommandOptions{
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
	cmd, err := BuildCommand(ProviderClaude, BuildCommandOptions{
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
		agent   ProviderType
		wantSet bool
	}{
		{ProviderClaude, true},
		{ProviderCodex, true},
		{ProviderShell, false},
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

func TestSessionIDPatternExtraction(t *testing.T) {
	tests := []struct {
		name   string
		agent  ProviderType
		output string
		wantID string
	}{
		{
			name:  "claude exit output",
			agent: ProviderClaude,
			output: ` ▐▛███▜▌   Claude Code v2.1.71
▝▜█████▛▘  Opus 4.6 · Claude Max
  ▘▘ ▝▝    ~/Workspace/repos/bxnlabs/argus

────────────────────────────────────────────────────────────────────────────────────────────────────── ▪▪▪ Medium /model ─
❯ Try "how do I log an error?"
──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  Press Ctrl-C again to exit

Resume this session with:
claude --resume e9ed7eb1-5fa8-40ca-b718-bc747ea4e38e`,
			wantID: "e9ed7eb1-5fa8-40ca-b718-bc747ea4e38e",
		},
		{
			name:  "codex exit output",
			agent: ProviderCodex,
			output: `╭───────────────────────────────────────────────────╮
│ >_ OpenAI Codex (v0.111.0)                        │
│                                                   │
│ model:     gpt-5.3-codex xhigh   /model to change │
│ directory: ~/Workspace/repos/bxnlabs/argus        │
╰───────────────────────────────────────────────────╯

› this is a test


• Ready. Tell me what you want to work on.
Token usage: total=1,362 input=1,154 (+ 7,424 cached) output=208 (reasoning 191)
To continue this session, run codex resume 019cce43-57d3-7842-9f1d-732711edbf25`,
			wantID: "019cce43-57d3-7842-9f1d-732711edbf25",
		},
		{
			name:   "no match in output",
			agent:  ProviderClaude,
			output: "Some random output without a session ID",
			wantID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := GetSessionIDPattern(tt.agent)
			if pattern == "" {
				t.Fatal("expected non-empty pattern")
			}
			got := extractSessionID(pattern, tt.output)
			if got != tt.wantID {
				t.Errorf("extractSessionID() = %q, want %q", got, tt.wantID)
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

func TestGeminiUnregistered(t *testing.T) {
	const gemini = ProviderType("gemini")

	if IsValid(gemini) {
		t.Error("gemini should no longer be a registered provider")
	}
	if _, err := Get(gemini); err == nil {
		t.Error("Get(gemini) should return an error")
	}
	if _, err := BuildCommand(gemini, BuildCommandOptions{AutoApprove: true}); err == nil {
		t.Error("BuildCommand(gemini) should return an error")
	}
	if pat := GetSessionIDPattern(gemini); pat != "" {
		t.Errorf("GetSessionIDPattern(gemini) = %q, want empty", pat)
	}
	for _, p := range All() {
		if p.ID == gemini {
			t.Error("All() still lists gemini")
		}
	}
}
