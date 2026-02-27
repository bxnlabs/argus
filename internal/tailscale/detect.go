package tailscale

import (
	"context"
	"net/netip"

	"tailscale.com/client/local"
)

// DetectIPs queries the local Tailscale daemon for this node's Tailscale IPs.
// Returns both IPv4 (100.x.x.x) and IPv6 (fd7a:...) addresses when available.
// Returns nil and no error if Tailscale is not running or has no IPs.
func DetectIPs(ctx context.Context) ([]netip.Addr, error) {
	status, err := local.Status(ctx)
	if err != nil {
		return nil, err
	}
	if status.Self == nil {
		return nil, nil
	}
	return status.Self.TailscaleIPs, nil
}
