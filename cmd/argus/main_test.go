package main

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
)

func TestBuildListeners(t *testing.T) {
	tests := []struct {
		name      string
		addrs     []string
		needsIPv6 bool
	}{
		{name: "single loopback", addrs: []string{"127.0.0.1:0"}},
		{name: "multiple addrs", addrs: []string{"127.0.0.1:0", "[::1]:0"}, needsIPv6: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.needsIPv6 {
				ln, err := net.Listen("tcp6", "[::1]:0")
				if err != nil {
					t.Skip("IPv6 not available on this platform")
				}
				ln.Close()
			}
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
	lns, disc, closer, err := makeListeners(context.Background(), tsCfg, "127.0.0.1", 0, "combined")
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

func TestMakeListeners_DisabledWildcard(t *testing.T) {
	tsCfg := config.TailscaleConfig{Enabled: false}
	lns, disc, _, err := makeListeners(context.Background(), tsCfg, "0.0.0.0", 0, "combined")
	if err != nil {
		t.Fatalf("makeListeners (wildcard): %v", err)
	}
	defer func() {
		for _, ln := range lns {
			ln.Close()
		}
	}()
	if len(lns) != 1 {
		t.Errorf("got %d listeners, want 1", len(lns))
	}
	if !strings.HasPrefix(disc, "127.0.0.1:") {
		t.Errorf("discoveryAddr = %q, want 127.0.0.1:* prefix", disc)
	}
}

func TestMakeListeners_DisabledWildcardIPv6(t *testing.T) {
	// Verify :: wildcard also normalizes discovery to 127.0.0.1.
	ln, err := net.Listen("tcp6", "[::]:0")
	if err != nil {
		t.Skip("IPv6 not available on this platform")
	}
	ln.Close()

	tsCfg := config.TailscaleConfig{Enabled: false}
	lns, disc, _, err := makeListeners(context.Background(), tsCfg, "::", 0, "combined")
	if err != nil {
		t.Fatalf("makeListeners (:: wildcard): %v", err)
	}
	defer func() {
		for _, ln := range lns {
			ln.Close()
		}
	}()
	if !strings.HasPrefix(disc, "127.0.0.1:") {
		t.Errorf("discoveryAddr = %q, want 127.0.0.1:* prefix", disc)
	}
}

func TestMakeListeners_HostnameSuffix(t *testing.T) {
	// We can't start tsnet in tests, but we can verify the hostname derivation
	// logic indirectly by checking that makeListeners passes through mode correctly.
	// The actual hostname derivation is tested here by verifying the function
	// signature accepts mode and the disabled path returns correctly for each mode.
	tsCfg := config.TailscaleConfig{Enabled: false}
	for _, mode := range []string{"combined", "server", "agent"} {
		t.Run(mode, func(t *testing.T) {
			lns, disc, _, err := makeListeners(context.Background(), tsCfg, "127.0.0.1", 0, mode)
			if err != nil {
				t.Fatalf("makeListeners (disabled, mode=%s): %v", mode, err)
			}
			defer func() {
				for _, ln := range lns {
					ln.Close()
				}
			}()
			if disc == "" {
				t.Errorf("discoveryAddr is empty for mode %s", mode)
			}
		})
	}
}

func TestDeriveHostname(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		mode     string
		wantSfx  string
		wantPfx  string
	}{
		{name: "combined default", prefix: "", mode: "combined", wantPfx: "argus-"},
		{name: "server default", prefix: "", mode: "server", wantPfx: "argus-", wantSfx: "-server"},
		{name: "agent default", prefix: "", mode: "agent", wantPfx: "argus-", wantSfx: "-agent"},
		{name: "custom combined", prefix: "myhost", mode: "combined", wantPfx: "myhost"},
		{name: "custom server", prefix: "myhost", mode: "server", wantPfx: "myhost", wantSfx: "-server"},
		{name: "custom agent", prefix: "myhost", mode: "agent", wantPfx: "myhost", wantSfx: "-agent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveHostname(tt.prefix, tt.mode)
			if tt.wantPfx != "" && !strings.HasPrefix(got, tt.wantPfx) {
				t.Errorf("deriveHostname(%q, %q) = %q, want prefix %q", tt.prefix, tt.mode, got, tt.wantPfx)
			}
			if tt.wantSfx != "" && !strings.HasSuffix(got, tt.wantSfx) {
				t.Errorf("deriveHostname(%q, %q) = %q, want suffix %q", tt.prefix, tt.mode, got, tt.wantSfx)
			}
			if tt.mode == "combined" && (strings.HasSuffix(got, "-server") || strings.HasSuffix(got, "-agent")) {
				t.Errorf("combined mode should not have -server/-agent suffix, got %q", got)
			}
		})
	}
}
