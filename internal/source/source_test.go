package source_test

import (
	"os"
	"testing"

	"github.com/bxnlabs/argus/internal/source"
)

func TestResolveLocalPath(t *testing.T) {
	dir := t.TempDir()
	src, err := source.Resolve(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.IsRemote() {
		t.Fatal("expected local source")
	}
	if src.LocalPath != dir {
		t.Errorf("expected LocalPath %q, got %q", dir, src.LocalPath)
	}
}

func TestResolveNonexistentPathTreatedAsRemote(t *testing.T) {
	// A path that doesn't exist and isn't a valid git URL → error
	_, err := source.Resolve("/definitely/does/not/exist/on/this/system")
	if err == nil {
		t.Fatal("expected error for nonexistent path that is not a git URL")
	}
}

func TestResolveOrgRepoShorthand(t *testing.T) {
	src, err := source.Resolve("bxnlabs/argus")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !src.IsRemote() {
		t.Fatal("expected remote source")
	}
	if src.Host != "github.com" {
		t.Errorf("expected Host %q, got %q", "github.com", src.Host)
	}
	if src.Org != "bxnlabs" {
		t.Errorf("expected Org %q, got %q", "bxnlabs", src.Org)
	}
	if src.Repo != "argus" {
		t.Errorf("expected Repo %q, got %q", "argus", src.Repo)
	}
	if src.RemoteURL != "https://github.com/bxnlabs/argus.git" {
		t.Errorf("unexpected RemoteURL %q", src.RemoteURL)
	}
}

func TestResolveHTTPSURL(t *testing.T) {
	src, err := source.Resolve("https://github.com/bxnlabs/argus.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !src.IsRemote() {
		t.Fatal("expected remote source")
	}
	if src.Host != "github.com" || src.Org != "bxnlabs" || src.Repo != "argus" {
		t.Errorf("unexpected parsed fields: host=%q org=%q repo=%q", src.Host, src.Org, src.Repo)
	}
}

func TestResolveHTTPSURLWithoutDotGit(t *testing.T) {
	src, err := source.Resolve("https://github.com/bxnlabs/argus")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.RemoteURL != "https://github.com/bxnlabs/argus.git" {
		t.Errorf("expected .git suffix appended, got %q", src.RemoteURL)
	}
}

func TestResolveSSHURL(t *testing.T) {
	src, err := source.Resolve("git@github.com:bxnlabs/argus.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !src.IsRemote() {
		t.Fatal("expected remote source")
	}
	if src.Host != "github.com" || src.Org != "bxnlabs" || src.Repo != "argus" {
		t.Errorf("unexpected parsed fields: host=%q org=%q repo=%q", src.Host, src.Org, src.Repo)
	}
	if src.RemoteURL != "https://github.com/bxnlabs/argus.git" {
		t.Errorf("unexpected RemoteURL %q", src.RemoteURL)
	}
}

func TestParentKeyLocal(t *testing.T) {
	src := &source.Source{LocalPath: "/Users/jeevb/repos/argus"}
	got := src.ParentKey()
	want := "--Users--jeevb--repos--argus"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestParentKeyRemote(t *testing.T) {
	src := &source.Source{
		RemoteURL: "https://github.com/bxnlabs/argus.git",
		Host:      "github.com",
		Org:       "bxnlabs",
		Repo:      "argus",
	}
	got := src.ParentKey()
	want := "github.com--bxnlabs--argus"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestResolveInvalidInput(t *testing.T) {
	cases := []string{
		"not-a-path-or-url",
		"git@missing-colon",
		"https://",
	}
	for _, tc := range cases {
		_, err := source.Resolve(tc)
		if err == nil {
			t.Errorf("expected error for input %q", tc)
		}
	}
}

// Ensure home dir is accessible (sanity check for tilde expansion).
func TestResolveTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	src, err := source.Resolve("~")
	if err != nil {
		t.Fatalf("unexpected error resolving ~: %v", err)
	}
	if src.LocalPath != home {
		t.Errorf("expected LocalPath %q, got %q", home, src.LocalPath)
	}
}
