package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bxnlabs/argus/cmd/argus/cli"
	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/node"
	"github.com/bxnlabs/argus/internal/registry"
	"github.com/bxnlabs/argus/internal/shared"
	ts "github.com/bxnlabs/argus/internal/tailscale"
	"github.com/bxnlabs/argus/internal/web"
	"github.com/spf13/cobra"
)

var cfg *config.Config

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var configFile string

	rootCmd := &cobra.Command{
		Use:   "argus",
		Short: "Argus — node session manager",
		Long:  "Argus runs a unified instance serving the web UI and node API.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			cfg, err = config.Load(config.Options{ConfigFile: configFile})
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstance(cmd.Context())
		},
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "path to config file (default ~/.argus/config.toml)")

	rootCmd.AddCommand(
		newMigrateCmd(),
		cli.NewSessionCmd(),
		cli.NewProfileCmd(),
		cli.NewInternalCmd(),
		cli.NewGitCmd(),
	)

	return rootCmd
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

// newNodeID returns a random hex id for a manual node. A crypto/rand failure is
// unrecoverable (and would otherwise yield an all-zero, colliding id), so panic
// rather than return a bad id — matching the stdlib/uuid convention.
func newNodeID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("newNodeID: crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b[:])
}

func writeDiscovery(addr string) {
	dp, err := node.DefaultDiscoveryPath()
	if err != nil {
		log.Printf("warning: cannot determine discovery path: %v", err)
		return
	}
	if err := node.WriteDiscoveryFile(dp, addr); err != nil {
		log.Printf("warning: cannot write discovery file: %v", err)
	}
}

func removeDiscovery() {
	dp, err := node.DefaultDiscoveryPath()
	if err != nil {
		return
	}
	node.RemoveDiscoveryFile(dp)
}

// runInstance starts the node API and SPA as a single unified instance.
func runInstance(ctx context.Context) error {
	listeners, discoveryAddr, tsFQDN, tsServer, err := makeListeners(ctx, cfg.Tailscale, cfg.BindAddress, cfg.Port)
	if err != nil {
		return err
	}
	if tsServer != nil {
		defer func() {
			if err := tsServer.Close(); err != nil {
				log.Printf("warning: tailscale shutdown: %v", err)
			}
		}()
	}

	var baseURL string
	if tsFQDN != "" {
		baseURL = "http://" + tsFQDN
	}

	nodeHandler, database, cors, cleanup, err := node.Setup(cfg, baseURL)
	if err != nil {
		return err
	}
	defer cleanup()

	mux := http.NewServeMux()
	mux.Handle("/api/node/", http.StripPrefix("/api/node", nodeHandler))
	mux.Handle("/", web.NewSPAHandler(""))

	// Self entry: same-origin from the browser (URL empty). Its dedup key is
	// the tailnet URL (if any) so a discovered copy of self is collapsed.
	osHostname, _ := os.Hostname()
	selfName := deriveSelfName(cfg.Tailscale.HostnamePrefix, osHostname)
	self := registry.Node{
		ID:     "local",
		Name:   selfName,
		URL:    "",
		Source: registry.SourceLocal,
		Self:   true,
	}

	var discover registry.DiscoverFunc
	if tsServer != nil {
		tag := cfg.Tailscale.DiscoveryTag
		discover = func(ctx context.Context) ([]registry.Node, error) {
			// Discovery is best-effort and feeds the initial UI load; bound it so
			// a stalled Tailscale LocalClient can't hang /api/nodes. On timeout
			// Peers returns an error and List degrades to local+manual.
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			peers, err := tsServer.Peers(ctx)
			if err != nil {
				return nil, err
			}
			var nodes []registry.Node
			for _, p := range peers {
				if !hasTag(p.Tags, tag) {
					continue
				}
				// Plan 2 lists only currently-reachable peers; offline tagged
				// nodes are omitted rather than shown as down. (A future plan may
				// surface offline state in the UI instead of hiding them.)
				if !p.Online {
					continue
				}
				host := strings.TrimSuffix(p.DNSName, ".")
				if host == "" {
					continue
				}
				peerURL := "http://" + host // :80 implicit
				nodes = append(nodes, registry.NodeFromDiscovery(host, peerURL))
			}
			return nodes, nil
		}
	}

	// baseURL is "http://<fqdn>" (or "" when no Tailscale) — the local node's
	// canonical URL, used to drop a discovered copy of self from the listing.
	regSvc := registry.New(database, self, baseURL, discover)
	// Guard the registry routes with the SAME Host + Origin policy as the node
	// API (the cors wrapper from node.Setup): they're served on the top-level mux
	// (not behind node.Setup's CORS) and mutate state from JSON without a
	// Content-Type check, so an unguarded route is a CSRF/rebinding surface for
	// any page the user visits.
	regHandlers := registry.NewHandlers(regSvc, newNodeID, cors)
	regHandlers.Register(mux)

	return serve(listeners, discoveryAddr, mux, "argus")
}

// buildListeners creates TCP listeners for each address.
// On failure, any already-opened listeners are closed.
func buildListeners(addrs []string) ([]net.Listener, error) {
	if len(addrs) == 0 {
		return nil, fmt.Errorf("listen: no addresses provided")
	}
	var listeners []net.Listener
	for _, a := range addrs {
		ln, err := net.Listen("tcp", a)
		if err != nil {
			for _, l := range listeners {
				l.Close()
			}
			return nil, fmt.Errorf("listen on %s: %w", a, err)
		}
		listeners = append(listeners, ln)
	}
	return listeners, nil
}

func listenAddr(bindAddress string, port int) string {
	return net.JoinHostPort(bindAddress, strconv.Itoa(port))
}

// sanitizeDNSCompliantHostname normalises s into a valid DNS label:
// strips the domain part (after first dot), lowercases, keeps only [a-z0-9-],
// trims leading/trailing hyphens, and caps length at 63 characters.
func sanitizeDNSCompliantHostname(s string) string {
	if i := strings.IndexByte(s, '.'); i != -1 {
		s = s[:i]
	}
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	s = strings.Trim(b.String(), "-")
	if len(s) > 63 {
		s = strings.TrimRight(s[:63], "-")
	}
	return s
}

// deriveSelfName produces the local node's display name. It prefers the
// configured Tailscale hostname prefix, falling back to the OS hostname, and
// applies the same DNS-label sanitisation as Tailscale node names so a macOS
// host reported as "bumblebee.local" displays as "bumblebee". Returns
// "this node" when neither yields a usable label.
func deriveSelfName(prefix, osHostname string) string {
	name := sanitizeDNSCompliantHostname(prefix)
	if name == "" {
		name = sanitizeDNSCompliantHostname(osHostname)
	}
	if name == "" {
		name = "this node"
	}
	return name
}

// deriveHostname builds the Tailscale node hostname from prefix.
// If prefix is empty, defaults to "argus-<os.Hostname()>".
// The result is sanitised to a DNS-compliant label.
func deriveHostname(prefix string) string {
	hostname := prefix
	if hostname == "" {
		h, err := os.Hostname()
		if err != nil {
			return ""
		}
		hostname = "argus-" + sanitizeDNSCompliantHostname(h)
	}
	return sanitizeDNSCompliantHostname(hostname)
}

func makeListeners(ctx context.Context, tsCfg config.TailscaleConfig, bindAddress string, port int) (listeners []net.Listener, discoveryAddr string, tsFQDN string, tsServer *ts.Server, err error) {
	if !tsCfg.Enabled {
		ip := net.ParseIP(bindAddress)

		if ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
			// Specific non-loopback IP: add a loopback listener so CLI can reach the node.
			addr := listenAddr(bindAddress, port)
			loopback := listenAddr("127.0.0.1", port)
			lns, err := buildListeners([]string{addr, loopback})
			if err != nil {
				return nil, "", "", nil, err
			}
			return lns, lns[1].Addr().String(), "", nil, nil
		}

		addr := listenAddr(bindAddress, port)
		lns, err := buildListeners([]string{addr})
		if err != nil {
			return nil, "", "", nil, err
		}

		if ip != nil && ip.IsUnspecified() {
			// Wildcard covers loopback; normalize discovery to 127.0.0.1.
			_, actualPort, _ := net.SplitHostPort(lns[0].Addr().String())
			return lns, net.JoinHostPort("127.0.0.1", actualPort), "", nil, nil
		}

		// Loopback bind: discovery is the listener's actual address.
		return lns, lns[0].Addr().String(), "", nil, nil
	}

	// Tailscale enabled: loopback + tsnet listeners
	hostname := deriveHostname(tsCfg.HostnamePrefix)
	if hostname == "" {
		return nil, "", "", nil, fmt.Errorf("determine hostname: failed to resolve OS hostname")
	}

	// Secure the state root before creating Tailscale state under it. This is
	// the one serving path that bypasses config.Load's auto-discovery securing
	// (explicit --config) and node.Setup's own securing. Tailscale already needs
	// a resolvable home here, so this adds no new requirement for the
	// Tailscale-disabled instance, which returns above.
	argusHome, err := shared.EnsureStateDir()
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("prepare state dir: %w", err)
	}
	stateDir := filepath.Join(argusHome, "tailscale", hostname)

	srv := ts.New(hostname, tsCfg.AuthKey, stateDir, uint16(tsCfg.Port))

	upCtx, upCancel := context.WithTimeout(ctx, 30*time.Second)
	defer upCancel()

	if err := srv.Up(upCtx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, "", "", nil, fmt.Errorf("tailscale: timed out waiting for node to start (30s). If this is a first run, set ARGUS_TAILSCALE_AUTH_KEY or tailscale.auth_key. Hostname: %s, state dir: %s", hostname, stateDir)
		}
		return nil, "", "", nil, fmt.Errorf("tailscale: %w", err)
	}

	fqdn := srv.FQDN()

	// Loopback listener for CLI access
	loopbackAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	loopbackLn, err := net.Listen("tcp", loopbackAddr)
	if err != nil {
		srv.Close()
		return nil, "", "", nil, fmt.Errorf("listen on %s: %w", loopbackAddr, err)
	}

	// tsnet listener for tailnet access.
	const tailnetHTTPPort = 80
	// tsnet listeners are userspace on this node's own tailnet IP, so binding
	// :80 needs no privilege and cannot collide with the host's port 80.
	tsLn, err := srv.Listen("tcp", ":"+strconv.Itoa(tailnetHTTPPort))
	if err != nil {
		loopbackLn.Close()
		srv.Close()
		return nil, "", "", nil, fmt.Errorf("tailscale listen: %w", err)
	}

	discoveryAddr = loopbackLn.Addr().String()
	return []net.Listener{loopbackLn, tsLn}, discoveryAddr, fqdn, srv, nil
}

// serve starts an HTTP server on the given listeners with graceful shutdown.
func serve(listeners []net.Listener, discoveryAddr string, handler http.Handler, name string) error {
	if len(listeners) == 0 {
		return fmt.Errorf("serve: no listeners provided")
	}

	srv := &http.Server{
		Handler: handler,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(done)

	serveErr := make(chan error, len(listeners))
	for _, ln := range listeners {
		go func(l net.Listener) {
			log.Printf("%s listening on %s", name, l.Addr().String())
			if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
				serveErr <- err
			}
		}(ln)
	}

	if discoveryAddr != "" {
		writeDiscovery(discoveryAddr)
		defer removeDiscovery()
	}

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-done:
	}
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	return nil
}
