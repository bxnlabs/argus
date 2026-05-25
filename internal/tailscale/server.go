package tailscale

import (
	"context"
	"net"

	"tailscale.com/tsnet"

	"github.com/bxnlabs/argus/internal/shared"
)

// Server wraps a tsnet.Server with started-state tracking for safe Close().
type Server struct {
	ts      *tsnet.Server
	started bool
}

// New creates a Server configured to join the tailnet with the given hostname.
// stateDir is created with 0o700 permissions if it does not exist.
// port sets the WireGuard UDP port; 0 means auto-select.
func New(hostname, authKey, stateDir string, port uint16) *Server {
	return &Server{
		ts: &tsnet.Server{
			Hostname: hostname,
			AuthKey:  authKey,
			Dir:      stateDir,
			Port:     port,
		},
	}
}

// Up starts the tsnet backend and blocks until the node reaches Running state.
// The caller should pass a context with a timeout (e.g. 30 seconds).
func (s *Server) Up(ctx context.Context) error {
	if err := shared.EnsureSecureDir(s.ts.Dir); err != nil {
		return err
	}
	if _, err := s.ts.Up(ctx); err != nil {
		return err
	}
	s.started = true
	return nil
}

// Listen creates a net.Listener on the tsnet interface.
// Must be called after Up() succeeds.
func (s *Server) Listen(network, addr string) (net.Listener, error) {
	return s.ts.Listen(network, addr)
}

// FQDN returns the Tailscale fully-qualified domain name after Up() succeeds.
// Returns empty string if the server hasn't started or has no cert domains.
func (s *Server) FQDN() string {
	if !s.started {
		return ""
	}
	domains := s.ts.CertDomains()
	if len(domains) == 0 {
		return ""
	}
	return domains[0]
}

// Close shuts down the tsnet node. If Up() was never called, Close is a no-op.
func (s *Server) Close() error {
	if !s.started {
		return nil
	}
	return s.ts.Close()
}
