package status

import "regexp"

// BusyIndicators are text strings that indicate the agent is actively working.
var BusyIndicators = []string{
	"esc to interrupt",
	"(esc to interrupt)",
	"\u00b7 esc to interrupt",          // · esc to interrupt
	"running\u2026",                     // Running… (active tool execution)
	"background task still running",    // 1 background task still running (↓ to manage)
	"background tasks still running",   // N background tasks still running (↓ to manage)
}

// SpinnerChars are braille spinner characters used by CLI tools.
var SpinnerChars = []string{
	"\u280b", "\u2819", "\u2839", "\u2838", "\u283c", "\u2834", "\u2826", "\u2827", "\u2807", "\u280f",
}

// StatusLinePattern matches Claude Code's status line format: a word followed by
// the Unicode ellipsis (U+2026) and then an opening parenthesis with timing info,
// e.g. "✶ Tinkering… (2m 31s · thinking)". The trailing \s*\( distinguishes real
// status lines from truncated terminal text like "shift+tab to cycl…".
// Applied against lowercased content so it matches regardless of case.
var StatusLinePattern = regexp.MustCompile(`\w+\x{2026}\s*\(`)

// WaitingPatterns are regex patterns that indicate the agent is waiting for user input.
var WaitingPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\[Y/n\]`),
	regexp.MustCompile(`(?i)\[y/N\]`),
	regexp.MustCompile(`(?i)Allow\?`),
	regexp.MustCompile(`(?i)Approve\?`),
	regexp.MustCompile(`(?i)Continue\?`),
	regexp.MustCompile(`(?i)Press Enter to`),
	regexp.MustCompile(`(?i)waiting for input`),
	regexp.MustCompile(`(?i)\(yes/no\)`),
	regexp.MustCompile(`(?i)Do you want to`),
	regexp.MustCompile(`(?i)Enter to (?:confirm|select).*Esc to cancel`),
	regexp.MustCompile(`(?i)Claude has written up a plan`),
	regexp.MustCompile(`(?i)Yes, allow all`),
	regexp.MustCompile(`(?i)allow all edits`),
	regexp.MustCompile(`(?i)allow all commands`),
}
