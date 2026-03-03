package tailscale

import (
	"context"
	"fmt"
	"net/netip"

	"tailscale.com/client/local"
	"tailscale.com/ipn/ipnstate"
)

// FetchStatus queries the local Tailscale daemon for the current node status.
func FetchStatus(ctx context.Context) (*ipnstate.Status, error) {
	return local.Status(ctx)
}

// DetectIPs extracts Tailscale IPs from a daemon status.
// Returns an error if the status or self peer info is nil.
func DetectIPs(status *ipnstate.Status) ([]netip.Addr, error) {
	if status == nil || status.Self == nil {
		return nil, fmt.Errorf("tailscale daemon returned no status")
	}
	return status.Self.TailscaleIPs, nil
}

// ValidateTailnet checks that the node is connected to the expected tailnet
// by comparing against the MagicDNSSuffix. Returns an error if the node is
// not connected to any tailnet or is connected to a different one.
func ValidateTailnet(status *ipnstate.Status, expected string) error {
	if status == nil || status.CurrentTailnet == nil {
		return fmt.Errorf("not connected to any tailnet")
	}
	if status.CurrentTailnet.MagicDNSSuffix != expected {
		return fmt.Errorf("connected to tailnet %q, want %q",
			status.CurrentTailnet.MagicDNSSuffix, expected)
	}
	return nil
}
