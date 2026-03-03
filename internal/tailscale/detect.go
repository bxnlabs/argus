package tailscale

import (
	"context"
	"fmt"
	"net/netip"

	"tailscale.com/client/local"
)

// DetectIPs queries the local Tailscale daemon for this node's Tailscale IPs,
// validating that the node is connected to the specified tailnet. Returns an
// error if the daemon is unreachable, not connected to any tailnet, or connected
// to a different tailnet than expected.
func DetectIPs(ctx context.Context, tailnet string) ([]netip.Addr, error) {
	status, err := local.Status(ctx)
	if err != nil {
		return nil, err
	}
	if status == nil || status.Self == nil {
		return nil, nil
	}
	if status.CurrentTailnet == nil {
		return nil, fmt.Errorf("not connected to any tailnet")
	}
	if status.CurrentTailnet.Name != tailnet {
		return nil, fmt.Errorf("connected to tailnet %q, want %q",
			status.CurrentTailnet.Name, tailnet)
	}
	return status.Self.TailscaleIPs, nil
}
