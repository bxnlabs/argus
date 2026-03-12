package main

import (
	"context"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
)

func TestBuildListeners(t *testing.T) {
	tests := []struct {
		name  string
		addrs []string
	}{
		{name: "single loopback", addrs: []string{"127.0.0.1:0"}},
		{name: "multiple addrs", addrs: []string{"127.0.0.1:0", "[::1]:0"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lns, err := buildListeners(tt.addrs)
			if err != nil {
				t.Fatalf("buildListeners(%v): %v", tt.addrs, err)
			}
			defer func() {
				for _, ln := range lns {
					ln.Close()
				}
			}()
			if len(lns) != len(tt.addrs) {
				t.Errorf("got %d listeners, want %d", len(lns), len(tt.addrs))
			}
		})
	}
}

func TestBuildListeners_Empty(t *testing.T) {
	_, err := buildListeners(nil)
	if err == nil {
		t.Fatal("expected error for empty addrs, got nil")
	}
}

func TestBuildListeners_InvalidAddr(t *testing.T) {
	_, err := buildListeners([]string{"127.0.0.1:0", "invalid-addr"})
	if err == nil {
		t.Fatal("expected error for invalid addr, got nil")
	}
}

func TestMakeListeners_Disabled(t *testing.T) {
	tsCfg := config.TailscaleConfig{Enabled: false}
	lns, disc, closer, err := makeListeners(context.Background(), tsCfg, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("makeListeners (disabled): %v", err)
	}
	defer func() {
		for _, ln := range lns {
			ln.Close()
		}
	}()
	if len(lns) != 1 {
		t.Errorf("got %d listeners, want 1", len(lns))
	}
	if disc == "" {
		t.Error("discoveryAddr is empty")
	}
	if closer != nil {
		t.Error("tsCloser should be nil when Tailscale is disabled")
	}
}
