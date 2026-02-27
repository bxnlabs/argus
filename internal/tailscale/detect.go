package tailscale

import (
	"context"
	"net/netip"

	"tailscale.com/client/local"
)

// DetectIPs queries the local Tailscale daemon for this node's Tailscale IPs.
// Returns both IPv4 (100.x.x.x) and IPv6 (fd7a:...) addresses when available.
// Returns an error if the Tailscale daemon is not reachable, or nil with no
// error if the daemon is running but has no IPs assigned.
func DetectIPs(ctx context.Context) ([]netip.Addr, error) {
	status, err := local.Status(ctx)
	if err != nil {
		return nil, err
	}
	if status == nil || status.Self == nil {
		return nil, nil
	}
	return status.Self.TailscaleIPs, nil
}
