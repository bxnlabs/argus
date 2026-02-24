package main

import (
	"context"
	"flag"
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
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "server":
			if err := runServer(os.Args[2:]); err != nil {
				log.Fatalf("argus server: %v", err)
			}
			return
		case "agent":
			if err := runAgent(os.Args[2:]); err != nil {
				log.Fatalf("argus agent: %v", err)
			}
			return
		case "session":
			if err := cli.Run(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	if err := runCombined(); err != nil {
		log.Fatalf("argus: %v", err)
	}
}

func writeDiscovery(addr string) {
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
func runCombined() error {
	fs := flag.NewFlagSet("argus", flag.ExitOnError)
	port := fs.Int("port", 3000, "HTTP server port")
	dbPath := fs.String("db", "~/.argus/agent.db", "SQLite database path")
	fs.Parse(os.Args[1:])

	if args := fs.Args(); len(args) > 0 {
		fmt.Fprintf(os.Stderr, "argus: unknown command %q\n\nUsage: argus [server|agent|session] [flags]\n", args[0])
		os.Exit(2)
	}

	agentHandler, cleanup, err := agent.Setup(agent.Config{DBPath: *dbPath})
	if err != nil {
		return err
	}
	defer cleanup()

	mux := http.NewServeMux()
	mux.Handle("/agent/", http.StripPrefix("/agent", agentHandler))
	mux.Handle("/", web.NewSPAHandler(""))

	addr := fmt.Sprintf(":%d", *port)
	return serve(addr, mux, "argus", func(a string) {
		writeDiscovery(a)
	})
}

// runServer starts only the SPA frontend server.
func runServer(args []string) error {
	fs := flag.NewFlagSet("argus server", flag.ExitOnError)
	port := fs.Int("port", 3000, "HTTP server port")
	webDir := fs.String("web", "", "Override embedded SPA with local directory")
	fs.Parse(args)

	mux := http.NewServeMux()
	mux.Handle("/", web.NewSPAHandler(*webDir))

	return serve(fmt.Sprintf(":%d", *port), mux, "argus server", nil)
}

// runAgent starts only the agent API.
func runAgent(args []string) error {
	fs := flag.NewFlagSet("argus agent", flag.ExitOnError)
	port := fs.Int("port", 3011, "HTTP server port")
	dbPath := fs.String("db", "~/.argus/agent.db", "SQLite database path")
	fs.Parse(args)

	agentHandler, cleanup, err := agent.Setup(agent.Config{DBPath: *dbPath})
	if err != nil {
		return err
	}
	defer cleanup()

	mux := http.NewServeMux()
	mux.Handle("/agent/", http.StripPrefix("/agent", agentHandler))

	addr := fmt.Sprintf(":%d", *port)
	return serve(addr, mux, "argus agent", func(a string) {
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
