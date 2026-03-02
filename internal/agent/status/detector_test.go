package status

import (
	"testing"
)

func TestCheckBusyIndicators(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"empty", "", false},
		{"normal output", "hello world\nfoo bar", false},
		{"esc to interrupt", "some output\nesc to interrupt\n", true},
		{"parenthesized", "line1\n(esc to interrupt)\n", true},
		{"dot prefix", "line1\n\u00b7 esc to interrupt\n", true},
		{"spinner char", "line1\nline2\n\u280b processing\n", true},
		{"status line thinking", "line1\n\u2726 cogitating\u2026 (2m 31s \u00b7 thinking)\n", true},
		{"status line tokens", "line1\n\u2736 tinkering\u2026 (1m 5s \u00b7 \u2193 3.4k tokens)\n", true},
		{"status line novel word", "line1\n* xyzfracking\u2026 (5s \u00b7 thinking)\n", true},
		{"word without ellipsis", "line1\ncogitating something\n", false},
		{"truncated text", "line1\n\u23f5\u23f5 bypass permissions on (shift+tab to cycl\u2026\n", false},
		{"running ellipsis", "line1\n  \u23bf  running\u2026\n", true},
		{"background tasks running plural", "output\n\u203b Brewed for 53s \u00b7 3 background tasks still running (\u2193 to manage)\n", true},
		{"background task running singular", "output\n\u203b Brewed for 10s \u00b7 1 background task still running (\u2193 to manage)\n", true},
		{"old scrollback ignored", "esc to interrupt\n" + repeat("safe line\n", 15), false},
		// Mobile scenarios: -J rejoins wrapped lines into logical lines
		{"mobile: background tasks on long logical line", "output\n※ Brewed for 53s · 3 background tasks still running (↓ to manage)\n", true},
		{"mobile: status line on long logical line", "output\n✶ Tinkering… (2m 31s · thinking)\n", true},
		{"mobile: height-clipped no indicators present", "some truncated output\npartial content here\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkBusyIndicators(tt.content); got != tt.want {
				t.Errorf("checkBusyIndicators() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckWaitingPatterns(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"empty", "", false},
		{"normal", "hello world", false},
		{"Y/n prompt", "Continue? [Y/n]", true},
		{"allow", "Allow? (y/n)", true},
		{"press enter", "Press Enter to continue", true},
		{"yes allow all", "  1. Yes, allow all", true},
		{"plan exit full", " Claude has written up a plan and is ready to execute. Would you like to proceed?\n\n \u276f 1. Yes, clear context (13% used) and bypass permissions\n   2. Yes, and bypass permissions\n   3. Yes, manually approve edits\n   4. Type here to tell Claude what to change\n\n ctrl-g to edit in Vim \u00b7 ~/.claude/plans/temporal-finding-meteor.md", true},
		{"old scrollback", "[Y/n]\n" + repeat("safe\n", 15), false},
		// Mobile scenario: -J rejoins wrapped plan prompt into logical lines
		{"mobile: plan exit prompt on long logical line", " Claude has written up a plan and is ready to execute. Would you like to proceed?\n\n ❯ 1. Yes, clear context (13% used) and bypass permissions\n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkWaitingPatterns(tt.content); got != tt.want {
				t.Errorf("checkWaitingPatterns() = %v, want %v", got, tt.want)
			}
		})
	}
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
