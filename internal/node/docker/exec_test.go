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
		" agent ",
		"bash /home/jeev/.argus/tmp/argus-inner-sess_1.sh",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ExecCommand missing %q in:\n%s", want, got)
		}
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
}
