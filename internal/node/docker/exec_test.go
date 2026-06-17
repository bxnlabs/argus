package docker

import (
	"strings"
	"testing"
)

func TestExecCommand(t *testing.T) {
	got := ExecCommand(ExecOptions{
		Project: "argus-work",
		File:    "/home/jeev/.argus/profiles/work/docker-compose.yml",
		Workdir: "/home/jeev/repo/wt",
		UID:     "1000",
		GID:     "1000",
		Service: "agent",
		Command: "bash /home/jeev/.argus/tmp/argus-inner-sess_1.sh",
	})

	for _, want := range []string{
		"docker compose",
		"-p 'argus-work'",
		"-f '/home/jeev/.argus/profiles/work/docker-compose.yml'",
		"exec",
		"-w '/home/jeev/repo/wt'",
		"-u '1000:1000'",
		" 'agent' ",
		"bash /home/jeev/.argus/tmp/argus-inner-sess_1.sh",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ExecCommand missing %q in:\n%s", want, got)
		}
	}
}

func TestExecCommandEnvPrefix(t *testing.T) {
	got := ExecCommand(ExecOptions{
		Project: "argus-work",
		File:    "/x/compose.yaml",
		Service: "agent",
		Env:     []string{"ARGUS_HOST_HOME=/home/jeev", "ARGUS_UID=1000"},
		Command: "true",
	})
	// Env pairs are shell-quoted and prefixed before `docker compose` so it
	// re-interpolates ${ARGUS_*} at exec time.
	for _, want := range []string{
		"ARGUS_HOST_HOME='/home/jeev' ",
		"ARGUS_UID='1000' ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ExecCommand missing env prefix %q in:\n%s", want, got)
		}
	}
	if i := strings.Index(got, "docker compose"); i < strings.Index(got, "ARGUS_HOST_HOME") {
		t.Errorf("env prefixes must precede `docker compose`: %s", got)
	}
}

func TestExecCommandQuotesSingleQuotes(t *testing.T) {
	got := ExecCommand(ExecOptions{
		Project: "argus-work",
		File:    "/x/it's/compose.yaml",
		Service: "agent",
		Command: "true",
	})
	if !strings.Contains(got, `'/x/it'\''s/compose.yaml'`) {
		t.Errorf("path with single quote not escaped: %s", got)
	}
	if strings.Contains(got, "-w ") {
		t.Errorf("expected no -w flag when Workdir empty: %s", got)
	}
	if strings.Contains(got, "-u ") {
		t.Errorf("expected no -u flag when UID empty: %s", got)
	}
}
