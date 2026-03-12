package tailscale

import (
	"testing"
)

func TestNew(t *testing.T) {
	s := New("test-host", "tskey-auth-xxx", t.TempDir())
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
	if s.started {
		t.Error("started should be false initially")
	}
}

func TestClose_NotStarted(t *testing.T) {
	s := New("test-host", "", t.TempDir())
	if err := s.Close(); err != nil {
		t.Fatalf("Close on unstarted server: %v", err)
	}
}
