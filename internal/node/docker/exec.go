package docker

import "strings"

// ExecOptions describe a `docker compose exec` invocation that runs Command
// inside a profile stack's service.
type ExecOptions struct {
	Project string
	File    string
	Workdir string
	UID     string
	GID     string
	Service string
	Command string // raw shell command, e.g. "bash /path/inner.sh"
}

// ExecCommand builds the shell command string that runs Command inside the
// profile's service, for embedding in the host tmux init script. Workdir,
// File, and Project are single-quoted; Command is appended verbatim (the
// caller is responsible for quoting any paths inside it).
func ExecCommand(o ExecOptions) string {
	var b strings.Builder
	b.WriteString("docker compose")
	b.WriteString(" -p " + shellQuote(o.Project))
	b.WriteString(" -f " + shellQuote(o.File))
	b.WriteString(" exec")
	if o.Workdir != "" {
		b.WriteString(" -w " + shellQuote(o.Workdir))
	}
	if o.UID != "" {
		b.WriteString(" -u " + shellQuote(o.UID+":"+o.GID))
	}
	b.WriteString(" " + o.Service + " ")
	b.WriteString(o.Command)
	return b.String()
}

// shellQuote returns a single-quoted shell string with internal single quotes
// escaped as '\''.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
