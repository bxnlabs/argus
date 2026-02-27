package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/bxnlabs/argus/cmd/argus/cli"
	"github.com/bxnlabs/argus/internal/agent"
	"github.com/bxnlabs/argus/internal/config"
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
			return runCombined()
		},
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "path to config file (default ~/.argus/config.toml)")

	rootCmd.AddCommand(
		newServerCmd(),
		newAgentCmd(),
		cli.NewSessionCmd(),
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
			return serve(listenAddrs(cfg.Server.BindAddress, cfg.Server.Port), mux, "argus server", nil)
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
			agentHandler, cleanup, err := agent.Setup(cfg)
			if err != nil {
				return err
			}
			defer cleanup()

			mux := http.NewServeMux()
			mux.Handle("/agent/", http.StripPrefix("/agent", agentHandler))

			return serve(listenAddrs(cfg.Agent.BindAddress, cfg.Agent.Port), mux, "argus agent", func(a string) {
				writeDiscovery(a)
			})
		},
	}

	return cmd
}

func writeDiscovery(addr string) {
	// Always write a loopback address so the CLI can reach the agent
	// regardless of the configured bind address.
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		log.Printf("warning: cannot parse listen address: %v", err)
		return
	}
	loopback := net.JoinHostPort("127.0.0.1", port)

	dp, err := agent.DefaultDiscoveryPath()
	if err != nil {
		log.Printf("warning: cannot determine discovery path: %v", err)
		return
	}
	if err := agent.WriteDiscoveryFile(dp, loopback); err != nil {
		log.Printf("warning: cannot write discovery file: %v", err)
	}
}

func removeDiscovery() {
	dp, err := agent.DefaultDiscoveryPath()
	if err != nil {
		return
	}
	agent.RemoveDiscoveryFile(dp)
}

// runCombined starts the agent and SPA on a single port.
func runCombined() error {
	agentHandler, cleanup, err := agent.Setup(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	mux := http.NewServeMux()
	mux.Handle("/agent/", http.StripPrefix("/agent", agentHandler))
	mux.Handle("/", web.NewSPAHandler(""))

	return serve(listenAddrs(cfg.Server.BindAddress, cfg.Server.Port), mux, "argus", func(a string) {
		writeDiscovery(a)
	})
}

// listenAddrs returns the list of TCP addresses to listen on. It always
// includes the primary bindAddr:port. If bindAddr is a specific non-loopback
// IP (not unspecified), it appends 127.0.0.1:port so the local CLI can
// always reach the agent via loopback.
func listenAddrs(bindAddr string, port int) []string {
	primary := net.JoinHostPort(bindAddr, strconv.Itoa(port))
	ip := net.ParseIP(bindAddr)
	if ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
		loopback := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		return []string{primary, loopback}
	}
	return []string{primary}
}

// serve starts an HTTP server on the given addresses with graceful shutdown.
// The first address is treated as the primary; its resolved listen address is
// passed to the onListening callback (used to write the discovery file).
func serve(addrs []string, handler http.Handler, name string, onListening func(addr string)) error {
	if len(addrs) == 0 {
		return fmt.Errorf("listen: no addresses provided")
	}

	var listeners []net.Listener
	for _, a := range addrs {
		ln, err := net.Listen("tcp", a)
		if err != nil {
			// Clean up any listeners we already opened.
			for _, l := range listeners {
				l.Close()
			}
			return fmt.Errorf("listen on %s: %w", a, err)
		}
		listeners = append(listeners, ln)
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

	if onListening != nil {
		onListening(listeners[0].Addr().String())
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
