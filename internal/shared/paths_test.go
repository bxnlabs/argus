package shared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}
	cleanHome := filepath.Clean(home)

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		// Valid paths
		{name: "home directory", path: "~", want: cleanHome},
		{name: "tilde subpath", path: "~/projects", want: filepath.Join(cleanHome, "projects")},
		{name: "absolute in home", path: cleanHome + "/foo/bar", want: filepath.Join(cleanHome, "foo", "bar")},
		{name: "redundant slashes", path: cleanHome + "//foo//bar", want: filepath.Join(cleanHome, "foo", "bar")},
		{name: "dot in path", path: cleanHome + "/foo/./bar", want: filepath.Join(cleanHome, "foo", "bar")},
		{name: "dotdot within home", path: cleanHome + "/foo/bar/../baz", want: filepath.Join(cleanHome, "foo", "baz")},

		// Traversal attempts
		{name: "traverse above home", path: cleanHome + "/../../etc/passwd", wantErr: true},
		{name: "traverse via tilde", path: "~/../../../etc/passwd", wantErr: true},
		{name: "absolute outside home", path: "/etc/passwd", wantErr: true},
		{name: "root", path: "/", wantErr: true},
		// Note: bare ".." resolves relative to CWD via filepath.Abs, so it may
		// be valid if CWD is within home. Test absolute traversal instead.
		{name: "absolute traverse to root", path: "/../../../..", wantErr: true},
		{name: "deep traversal", path: cleanHome + "/foo/../../../../etc/shadow", wantErr: true},

		// Edge cases
		{name: "empty path", path: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeExpandPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("SafeExpandPath(%q) = %q, want error", tt.path, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("SafeExpandPath(%q) returned error: %v", tt.path, err)
			}
			if got != tt.want {
				t.Errorf("SafeExpandPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestSafeExpandPath_Symlinks(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}
	cleanHome := filepath.Clean(home)

	// Create a temp directory inside home for symlink tests.
	tmpDir, err := os.MkdirTemp(cleanHome, "safe-path-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	// Create a real subdirectory inside home.
	realDir := filepath.Join(tmpDir, "realdir")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("failed to create realdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "file.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Symlink inside home → another location inside home (should succeed).
	goodLink := filepath.Join(tmpDir, "good-link")
	if err := os.Symlink(realDir, goodLink); err != nil {
		t.Fatalf("failed to create good symlink: %v", err)
	}

	// Symlink inside home → /tmp (outside home).
	evilLink := filepath.Join(tmpDir, "evil-link")
	if err := os.Symlink("/tmp", evilLink); err != nil {
		t.Fatalf("failed to create evil symlink: %v", err)
	}

	t.Run("symlink within home is allowed", func(t *testing.T) {
		got, err := SafeExpandPath(goodLink + "/file.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should return the lexical (symlink) path, not the resolved target.
		want := goodLink + "/file.txt"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("symlink escaping home is blocked", func(t *testing.T) {
		_, err := SafeExpandPath(evilLink + "/somefile")
		if err == nil {
			t.Fatal("expected error for symlink escaping home, got nil")
		}
		if !strings.Contains(err.Error(), "outside home directory") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("symlink escaping home bare", func(t *testing.T) {
		_, err := SafeExpandPath(evilLink)
		if err == nil {
			t.Fatal("expected error for symlink escaping home, got nil")
		}
	})

	t.Run("new file in symlinked dir within home", func(t *testing.T) {
		got, err := SafeExpandPath(goodLink + "/newfile.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := goodLink + "/newfile.txt"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("new file under evil symlink is blocked", func(t *testing.T) {
		_, err := SafeExpandPath(evilLink + "/newfile.txt")
		if err == nil {
			t.Fatal("expected error for new file under evil symlink, got nil")
		}
	})
}

func TestStateDir(t *testing.T) {
	t.Run("honors ARGUS_HOME", func(t *testing.T) {
		t.Setenv("ARGUS_HOME", "/custom/argus/home")
		got, err := StateDir()
		if err != nil {
			t.Fatal(err)
		}
		if got != "/custom/argus/home" {
			t.Errorf("StateDir() = %q, want /custom/argus/home", got)
		}
	})

	t.Run("defaults to ~/.argus when ARGUS_HOME unset", func(t *testing.T) {
		t.Setenv("ARGUS_HOME", "")
		home := t.TempDir()
		t.Setenv("HOME", home)
		got, err := StateDir()
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".argus")
		if got != want {
			t.Errorf("StateDir() = %q, want %q", got, want)
		}
	})
}

func TestCleanPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}
	cleanHome := filepath.Clean(home)

	// Get CWD for relative path test
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{name: "absolute path", path: "/etc", want: "/etc"},
		{name: "root", path: "/", want: "/"},
		{name: "tilde", path: "~", want: cleanHome},
		{name: "tilde subpath", path: "~/projects", want: filepath.Join(cleanHome, "projects")},
		{name: "cleans dotdot", path: "/etc/../usr", want: "/usr"},
		{name: "cleans double slashes", path: "/usr//local//bin", want: "/usr/local/bin"},
		{name: "cleans dot", path: "/usr/./local", want: "/usr/local"},
		{name: "relative path", path: "foo/bar", want: filepath.Join(cwd, "foo/bar")},
		{name: "tilde dotdot escapes home", path: "~/../../etc", want: filepath.Clean(filepath.Join(cleanHome, "../../etc"))},
		{name: "empty path", path: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CleanPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("CleanPath(%q) = %q, want error", tt.path, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("CleanPath(%q) returned error: %v", tt.path, err)
			}
			if got != tt.want {
				t.Errorf("CleanPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
