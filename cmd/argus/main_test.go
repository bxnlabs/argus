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
	lns, disc, _, tsServer, err := makeListeners(context.Background(), tsCfg, "127.0.0.1", 0)
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
	if tsServer != nil {
		t.Error("tsServer should be nil when Tailscale is disabled")
	}
}

func TestMakeListeners_DisabledWildcard(t *testing.T) {
	tsCfg := config.TailscaleConfig{Enabled: false}
	lns, disc, _, _, err := makeListeners(context.Background(), tsCfg, "0.0.0.0", 0)
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
	lns, disc, _, _, err := makeListeners(context.Background(), tsCfg, "::", 0)
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

func TestSanitizeDNSCompliantHostname(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "simple", in: "bumblebee", want: "bumblebee"},
		{name: "fqdn local", in: "bumblebee.local", want: "bumblebee"},
		{name: "fqdn corporate", in: "host.corp.example.com", want: "host"},
		{name: "uppercase", in: "BUMBLEBEE", want: "bumblebee"},
		{name: "uppercase fqdn", in: "BUMBLEBEE.LOCAL", want: "bumblebee"},
		{name: "with underscores", in: "my_host", want: "myhost"},
		{name: "with spaces", in: "my host", want: "myhost"},
		{name: "leading hyphens", in: "--myhost", want: "myhost"},
		{name: "trailing hyphens", in: "myhost--", want: "myhost"},
		{name: "both hyphens", in: "--myhost--", want: "myhost"},
		{name: "interior hyphens preserved", in: "my-cool-host", want: "my-cool-host"},
		{name: "mixed invalid chars", in: "my@host!name#1", want: "myhostname1"},
		{name: "dots only in domain", in: "ok.not.this", want: "ok"},
		{name: "empty", in: "", want: ""},
		{name: "all invalid", in: "!!!...", want: ""},
		{name: "path traversal", in: "../../etc/passwd", want: ""},
		{name: "numeric", in: "12345", want: "12345"},
		{name: "63 char limit", in: strings.Repeat("a", 70), want: strings.Repeat("a", 63)},
		{name: "truncation removes trailing hyphen", in: strings.Repeat("a", 62) + "-b", want: strings.Repeat("a", 62) + "-"},
		// After truncation at 63 chars we get 62×a + "-", trailing hyphen should be trimmed.
	}
	// Fix the truncation test expectation: trailing hyphen trimmed.
	tests[len(tests)-1].want = strings.Repeat("a", 62)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeDNSCompliantHostname(tt.in)
			if got != tt.want {
				t.Errorf("sanitizeDNSCompliantHostname(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDeriveSelfName(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		hostname string
		want     string
	}{
		{name: "mac local stripped", prefix: "", hostname: "bumblebee.local", want: "bumblebee"},
		{name: "plain hostname", prefix: "", hostname: "bumblebee", want: "bumblebee"},
		{name: "corporate fqdn", prefix: "", hostname: "host.corp.example.com", want: "host"},
		{name: "prefix wins over hostname", prefix: "myhost", hostname: "bumblebee.local", want: "myhost"},
		{name: "prefix sanitised", prefix: "My-Box.local", hostname: "bumblebee", want: "my-box"},
		{name: "no name available", prefix: "", hostname: "", want: "this node"},
		{name: "unsanitisable hostname falls back", prefix: "", hostname: "!!!", want: "this node"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveSelfName(tt.prefix, tt.hostname)
			if got != tt.want {
				t.Errorf("deriveSelfName(%q, %q) = %q, want %q", tt.prefix, tt.hostname, got, tt.want)
			}
		})
	}
}

func TestDeriveHostname(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		wantPfx string
	}{
		{name: "default", prefix: "", wantPfx: "argus-"},
		{name: "custom", prefix: "myhost", wantPfx: "myhost"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveHostname(tt.prefix)
			if tt.wantPfx != "" && !strings.HasPrefix(got, tt.wantPfx) {
				t.Errorf("deriveHostname(%q) = %q, want prefix %q", tt.prefix, got, tt.wantPfx)
			}
			if strings.HasSuffix(got, "-server") || strings.HasSuffix(got, "-node") {
				t.Errorf("hostname should not have -server/-node suffix, got %q", got)
			}
		})
	}
}
