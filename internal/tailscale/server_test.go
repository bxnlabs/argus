package tailscale

import (
	"testing"
)

func TestNew(t *testing.T) {
	s := New("test-host", "tskey-auth-xxx", t.TempDir(), 0)
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.ts == nil {
		t.Fatal("ts field is nil")
	}
	if s.ts.Hostname != "test-host" {
		t.Errorf("Hostname = %q, want %q", s.ts.Hostname, "test-host")
	}
	if s.ts.AuthKey != "tskey-auth-xxx" {
		t.Errorf("AuthKey = %q, want %q", s.ts.AuthKey, "tskey-auth-xxx")
	}
	if s.ts.Port != 0 {
		t.Errorf("Port = %d, want 0", s.ts.Port)
	}
	if s.started {
		t.Error("started should be false initially")
	}
}

func TestNew_WithPort(t *testing.T) {
	s := New("test-host", "", t.TempDir(), 41642)
	if s.ts.Port != 41642 {
		t.Errorf("Port = %d, want 41642", s.ts.Port)
	}
}

func TestClose_NotStarted(t *testing.T) {
	s := New("test-host", "", t.TempDir(), 0)
	if err := s.Close(); err != nil {
		t.Fatalf("Close on unstarted server: %v", err)
	}
}
