package docker

import "strings"

// ExecOptions describe a `docker compose exec` invocation that runs Args
// inside a profile stack's service.
type ExecOptions struct {
	Project string
	File    string
	Workdir string
	Service string
	Env     []string // KEY=VALUE pairs prefixed before `docker compose` so it
	// re-interpolates ${ARGUS_*} at exec time, matching what `up` saw.
	Args []string // command and its arguments, e.g. {"bash", "/path/inner.sh"}.
	// Each element is shell-quoted for you, so paths with spaces or special
	// characters are safe — pass them unquoted.
}

// ExecCommand builds the shell command string that runs Args inside the
// profile's service, for embedding in the host tmux init script. Every field —
// Workdir, File, Project, the Env values, and each element of Args — is
// single-quoted, so no caller-side quoting is required.
func ExecCommand(o ExecOptions) string {
	var b strings.Builder
	for _, kv := range o.Env {
		k, v, _ := strings.Cut(kv, "=")
		b.WriteString(k + "=" + shellQuote(v) + " ")
	}
	b.WriteString("docker compose")
	b.WriteString(" -p " + shellQuote(o.Project))
	b.WriteString(" -f " + shellQuote(o.File))
	b.WriteString(" exec")
	if o.Workdir != "" {
		b.WriteString(" -w " + shellQuote(o.Workdir))
	}
	b.WriteString(" " + shellQuote(o.Service))
	for _, arg := range o.Args {
		b.WriteString(" " + shellQuote(arg))
	}
	return b.String()
}

// shellQuote returns a single-quoted shell string with internal single quotes
// escaped as '\''.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
