package tailscale

import (
	"net/netip"
	"strings"
	"testing"

	"tailscale.com/ipn/ipnstate"
)

func TestDetectIPs_NilStatus(t *testing.T) {
	_, err := DetectIPs(nil)
	if err == nil {
		t.Fatal("expected error for nil status, got nil")
	}
	if !strings.Contains(err.Error(), "no status") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDetectIPs_NilSelf(t *testing.T) {
	status := &ipnstate.Status{Self: nil}
	_, err := DetectIPs(status)
	if err == nil {
		t.Fatal("expected error for nil Self, got nil")
	}
	if !strings.Contains(err.Error(), "no status") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDetectIPs_ReturnsIPs(t *testing.T) {
	ip4 := netip.MustParseAddr("100.64.0.1")
	ip6 := netip.MustParseAddr("fd7a:115c:a1e0::1")
	status := &ipnstate.Status{
		Self: &ipnstate.PeerStatus{
			TailscaleIPs: []netip.Addr{ip4, ip6},
		},
	}
	addrs, err := DetectIPs(status)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("got %d IPs, want 2", len(addrs))
	}
	if addrs[0] != ip4 {
		t.Errorf("addrs[0] = %v, want %v", addrs[0], ip4)
	}
	if addrs[1] != ip6 {
		t.Errorf("addrs[1] = %v, want %v", addrs[1], ip6)
	}
}

func TestDetectIPs_EmptyIPs(t *testing.T) {
	status := &ipnstate.Status{
		Self: &ipnstate.PeerStatus{
			TailscaleIPs: nil,
		},
	}
	addrs, err := DetectIPs(status)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(addrs) != 0 {
		t.Errorf("got %d IPs, want 0", len(addrs))
	}
}

func TestValidateTailnet_NilStatus(t *testing.T) {
	err := ValidateTailnet(nil, "example.ts.net")
	if err == nil {
		t.Fatal("expected error for nil status, got nil")
	}
	if !strings.Contains(err.Error(), "not connected to any tailnet") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateTailnet_NilCurrentTailnet(t *testing.T) {
	status := &ipnstate.Status{CurrentTailnet: nil}
	err := ValidateTailnet(status, "example.ts.net")
	if err == nil {
		t.Fatal("expected error for nil CurrentTailnet, got nil")
	}
	if !strings.Contains(err.Error(), "not connected to any tailnet") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateTailnet_Matching(t *testing.T) {
	status := &ipnstate.Status{
		CurrentTailnet: &ipnstate.TailnetStatus{
			MagicDNSSuffix: "example.ts.net",
		},
	}
	if err := ValidateTailnet(status, "example.ts.net"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTailnet_Mismatch(t *testing.T) {
	status := &ipnstate.Status{
		CurrentTailnet: &ipnstate.TailnetStatus{
			MagicDNSSuffix: "other.ts.net",
		},
	}
	err := ValidateTailnet(status, "example.ts.net")
	if err == nil {
		t.Fatal("expected error for tailnet mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "other.ts.net") {
		t.Errorf("error should mention actual tailnet: %v", err)
	}
	if !strings.Contains(err.Error(), "example.ts.net") {
		t.Errorf("error should mention expected tailnet: %v", err)
	}
}

func TestValidateTailnet_MatchesMagicDNSSuffixNotName(t *testing.T) {
	status := &ipnstate.Status{
		CurrentTailnet: &ipnstate.TailnetStatus{
			Name:           "my-tailnet",
			MagicDNSSuffix: "my-tailnet.ts.net",
		},
	}
	// Should match on MagicDNSSuffix, not Name.
	if err := ValidateTailnet(status, "my-tailnet.ts.net"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Name alone should NOT match.
	err := ValidateTailnet(status, "my-tailnet")
	if err == nil {
		t.Fatal("expected error when matching against Name instead of MagicDNSSuffix")
	}
}
