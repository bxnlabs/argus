package main

import (
	"context"
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
	"github.com/bxnlabs/argus/internal/node"
	"github.com/bxnlabs/argus/internal/config"
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
		Short: "Argus — agent session manager",
		Long:  "Argus runs a combined web server and agent API, or individual components.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			cfg, err = config.Load(config.Options{ConfigFile: configFile})
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCombined(cmd.Context())
		},
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "path to config file (default ~/.argus/config.toml)")

	rootCmd.AddCommand(
		newServerCmd(),
		newAgentCmd(),
		cli.NewSessionCmd(),
		cli.NewInternalCmd(),
	)

	return rootCmd
}

func newServerCmd() *cobra.Command {
	var webDir string

	cmd := &cobra.Command{
		Use:          "server",
		Short:        "Start only the SPA frontend server",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			mux := http.NewServeMux()
			mux.Handle("/", web.NewSPAHandler(webDir))

			listeners, _, tsCloser, err := makeListeners(cmd.Context(), cfg.Tailscale, cfg.Server.BindAddress, cfg.Server.Port, "server")
			if err != nil {
				return err
			}
			if tsCloser != nil {
				defer func() {
					if err := tsCloser(); err != nil {
						log.Printf("warning: tailscale shutdown: %v", err)
					}
				}()
			}

			return serve(listeners, "", mux, "argus server")
		},
	}

	cmd.Flags().StringVar(&webDir, "web", "", "Override embedded SPA with local directory")

	return cmd
}

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "agent",
		Short:        "Start only the agent API",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			agentHandler, cleanup, err := node.Setup(cfg)
			if err != nil {
				return err
			}
			defer cleanup()

			mux := http.NewServeMux()
			mux.Handle("/agent/", http.StripPrefix("/agent", agentHandler))

			listeners, discoveryAddr, tsCloser, err := makeListeners(cmd.Context(), cfg.Tailscale, cfg.Node.BindAddress, cfg.Node.Port, "agent")
			if err != nil {
				return err
			}
			if tsCloser != nil {
				defer func() {
					if err := tsCloser(); err != nil {
						log.Printf("warning: tailscale shutdown: %v", err)
					}
				}()
			}

			return serve(listeners, discoveryAddr, mux, "argus agent")
		},
	}

	return cmd
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

// runCombined starts the agent and SPA on a single port.
func runCombined(ctx context.Context) error {
	agentHandler, cleanup, err := node.Setup(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	mux := http.NewServeMux()
	mux.Handle("/agent/", http.StripPrefix("/agent", agentHandler))
	mux.Handle("/", web.NewSPAHandler(""))

	listeners, discoveryAddr, tsCloser, err := makeListeners(ctx, cfg.Tailscale, cfg.Server.BindAddress, cfg.Server.Port, "combined")
	if err != nil {
		return err
	}
	if tsCloser != nil {
		defer func() {
			if err := tsCloser(); err != nil {
				log.Printf("warning: tailscale shutdown: %v", err)
			}
		}()
	}

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

// deriveHostname builds the Tailscale node hostname from prefix and mode.
// In split mode (server/agent), a mode suffix is appended.
// If prefix is empty, defaults to "argus-<os.Hostname()>".
// The result is sanitised to a DNS-compliant label.
func deriveHostname(prefix, mode string) string {
	hostname := prefix
	if hostname == "" {
		h, err := os.Hostname()
		if err != nil {
			return ""
		}
		hostname = "argus-" + sanitizeDNSCompliantHostname(h)
	}
	if mode == "server" || mode == "agent" {
		hostname = hostname + "-" + mode
	}
	return sanitizeDNSCompliantHostname(hostname)
}

func makeListeners(ctx context.Context, tsCfg config.TailscaleConfig, bindAddress string, port int, mode string) (listeners []net.Listener, discoveryAddr string, tsCloser func() error, err error) {
	if !tsCfg.Enabled {
		ip := net.ParseIP(bindAddress)

		if ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
			// Specific non-loopback IP: add a loopback listener so CLI can reach the agent.
			addr := listenAddr(bindAddress, port)
			loopback := listenAddr("127.0.0.1", port)
			lns, err := buildListeners([]string{addr, loopback})
			if err != nil {
				return nil, "", nil, err
			}
			return lns, lns[1].Addr().String(), nil, nil
		}

		addr := listenAddr(bindAddress, port)
		lns, err := buildListeners([]string{addr})
		if err != nil {
			return nil, "", nil, err
		}

		if ip != nil && ip.IsUnspecified() {
			// Wildcard covers loopback; normalize discovery to 127.0.0.1.
			_, actualPort, _ := net.SplitHostPort(lns[0].Addr().String())
			return lns, net.JoinHostPort("127.0.0.1", actualPort), nil, nil
		}

		// Loopback bind: discovery is the listener's actual address.
		return lns, lns[0].Addr().String(), nil, nil
	}

	// Tailscale enabled: loopback + tsnet listeners
	hostname := deriveHostname(tsCfg.HostnamePrefix, mode)
	if hostname == "" {
		return nil, "", nil, fmt.Errorf("determine hostname: failed to resolve OS hostname")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", nil, fmt.Errorf("determine home directory: %w", err)
	}
	stateDir := filepath.Join(home, ".argus", "tailscale", hostname)

	tsServer := ts.New(hostname, tsCfg.AuthKey, stateDir)

	upCtx, upCancel := context.WithTimeout(ctx, 30*time.Second)
	defer upCancel()

	if err := tsServer.Up(upCtx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, "", nil, fmt.Errorf("tailscale: timed out waiting for node to start (30s). If this is a first run, set ARGUS_TAILSCALE_AUTH_KEY or tailscale.auth_key. Hostname: %s, state dir: %s", hostname, stateDir)
		}
		return nil, "", nil, fmt.Errorf("tailscale: %w", err)
	}

	// Loopback listener for CLI access
	loopbackAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	loopbackLn, err := net.Listen("tcp", loopbackAddr)
	if err != nil {
		tsServer.Close()
		return nil, "", nil, fmt.Errorf("listen on %s: %w", loopbackAddr, err)
	}

	// tsnet listener for tailnet access
	tsLn, err := tsServer.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		loopbackLn.Close()
		tsServer.Close()
		return nil, "", nil, fmt.Errorf("tailscale listen: %w", err)
	}

	discoveryAddr = loopbackLn.Addr().String()
	return []net.Listener{loopbackLn, tsLn}, discoveryAddr, tsServer.Close, nil
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
