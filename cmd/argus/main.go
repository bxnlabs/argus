package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bxnlabs/argus/cmd/argus/cli"
	"github.com/bxnlabs/argus/internal/agent"
	"github.com/bxnlabs/argus/internal/web"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		port   int
		dbPath string
	)

	rootCmd := &cobra.Command{
		Use:   "argus",
		Short: "Argus — agent session manager",
		Long:  "Argus runs a combined web server and agent API, or individual components.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCombined(port, dbPath)
		},
		SilenceUsage: true,
	}

	rootCmd.Flags().IntVar(&port, "port", 3000, "HTTP server port")
	rootCmd.Flags().StringVar(&dbPath, "db", "~/.argus/agent.db", "SQLite database path")

	rootCmd.AddCommand(
		newServerCmd(),
		newAgentCmd(),
		cli.NewSessionCmd(),
	)

	return rootCmd
}

func newServerCmd() *cobra.Command {
	var (
		port   int
		webDir string
	)

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start only the SPA frontend server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			mux := http.NewServeMux()
			mux.Handle("/", web.NewSPAHandler(webDir))
			return serve(fmt.Sprintf(":%d", port), mux, "argus server", nil)
		},
	}

	cmd.Flags().IntVar(&port, "port", 3000, "HTTP server port")
	cmd.Flags().StringVar(&webDir, "web", "", "Override embedded SPA with local directory")

	return cmd
}

func newAgentCmd() *cobra.Command {
	var (
		port   int
		dbPath string
	)

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Start only the agent API",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			agentHandler, cleanup, err := agent.Setup(agent.Config{DBPath: dbPath})
			if err != nil {
				return err
			}
			defer cleanup()

			mux := http.NewServeMux()
			mux.Handle("/agent/", http.StripPrefix("/agent", agentHandler))

			addr := fmt.Sprintf(":%d", port)
			return serve(addr, mux, "argus agent", func(a string) {
				writeDiscovery(a)
			})
		},
	}

	cmd.Flags().IntVar(&port, "port", 3011, "HTTP server port")
	cmd.Flags().StringVar(&dbPath, "db", "~/.argus/agent.db", "SQLite database path")

	return cmd
}

func writeDiscovery(addr string) {
	// Normalize wildcard bind addresses (e.g. [::]:3000, 0.0.0.0:3000)
	// to loopback so the CLI's loopback validation accepts them.
	if host, port, err := net.SplitHostPort(addr); err == nil {
		if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
			addr = "127.0.0.1:" + port
		}
	}

	dp, err := agent.DefaultDiscoveryPath()
	if err != nil {
		log.Printf("warning: cannot determine discovery path: %v", err)
		return
	}
	if err := agent.WriteDiscoveryFile(dp, addr); err != nil {
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
func runCombined(port int, dbPath string) error {
	agentHandler, cleanup, err := agent.Setup(agent.Config{DBPath: dbPath})
	if err != nil {
		return err
	}
	defer cleanup()

	mux := http.NewServeMux()
	mux.Handle("/agent/", http.StripPrefix("/agent", agentHandler))
	mux.Handle("/", web.NewSPAHandler(""))

	addr := fmt.Sprintf(":%d", port)
	return serve(addr, mux, "argus", func(a string) {
		writeDiscovery(a)
	})
}

// serve starts an HTTP server with graceful shutdown.
func serve(addr string, handler http.Handler, name string, onListening func(addr string)) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	srv := &http.Server{
		Handler: handler,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(done)

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("%s listening on %s", name, ln.Addr().String())
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			serveErr <- err
		}
	}()

	if onListening != nil {
		onListening(ln.Addr().String())
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
