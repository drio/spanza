package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/drio/spanza/client"
	"github.com/drio/spanza/relay"
	"github.com/drio/spanza/server"
	"github.com/peterbourgon/ff/v3/ffcli"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	rootCmd := newRootCmd()
	if err := rootCmd.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	return rootCmd.Run(context.Background())
}

func newRootCmd() *ffcli.Command {
	rootfs := flag.NewFlagSet("spanza", flag.ExitOnError)

	return &ffcli.Command{
		Name:       "spanza",
		ShortUsage: "spanza <subcommand> [flags]",
		ShortHelp:  "WireGuard relay tool for NAT traversal",
		LongHelp: `spanza is a relay tool that forwards WireGuard packets over WebSocket/TLS
to enable peer communication when UDP traffic is blocked.`,
		Subcommands: []*ffcli.Command{
			newServerCmd(),
			newClientCmd(),
			newVersionCmd(),
		},
		FlagSet: rootfs,
		Exec: func(ctx context.Context, args []string) error {
			return flag.ErrHelp
		},
	}
}

func newServerCmd() *ffcli.Command {
	fs := flag.NewFlagSet("spanza server", flag.ExitOnError)
	udpAddr := fs.String("udp-addr", ":51820", "UDP listen address")
	wsAddr := fs.String("ws-addr", ":8443", "WebSocket/TLS listen address")
	certFile := fs.String("cert", "cert.pem", "TLS certificate file")
	keyFile := fs.String("key", "key.pem", "TLS key file")

	return &ffcli.Command{
		Name:       "server",
		ShortUsage: "spanza server [flags]",
		ShortHelp:  "Run in server mode",
		LongHelp: `Run spanza in server mode. The server binds to UDP and TCP/WebSocket ports,
inspects incoming WireGuard packets, and relays them to the appropriate peer.`,
		FlagSet: fs,
		Exec: func(ctx context.Context, args []string) error {
			return runServer(ctx, *udpAddr, *wsAddr, *certFile, *keyFile)
		},
	}
}

func newClientCmd() *ffcli.Command {
	fs := flag.NewFlagSet("spanza client", flag.ExitOnError)
	listenAddr := fs.String("listen", ":51820", "UDP listen address")
	serverAddr := fs.String("server", "localhost:51820", "Server UDP address")

	return &ffcli.Command{
		Name:       "client",
		ShortUsage: "spanza client [flags]",
		ShortHelp:  "Run in client (sidecar) mode",
		LongHelp: `Run spanza in client mode. The client listens on a UDP port and forwards
WireGuard packets to the server over UDP.`,
		FlagSet: fs,
		Exec: func(ctx context.Context, args []string) error {
			return runClient(ctx, *listenAddr, *serverAddr)
		},
	}
}

func newVersionCmd() *ffcli.Command {
	return &ffcli.Command{
		Name:       "version",
		ShortUsage: "spanza version",
		ShortHelp:  "Print version information",
		Exec: func(ctx context.Context, args []string) error {
			fmt.Printf("spanza v%s\n", version)
			fmt.Printf("WireGuard relay tool for NAT traversal\n")
			return nil
		},
	}
}

func runServer(ctx context.Context, udpAddr, wsAddr, certFile, keyFile string) error {
	fmt.Printf("Starting server mode...\n")
	fmt.Printf("  UDP address: %s\n", udpAddr)

	// Create registry and processor
	registry := relay.NewRegistry()
	processor := relay.NewProcessor(registry)

	// Create server configuration
	cfg := &server.ServerConfig{
		UDPAddr:   udpAddr,
		Registry:  registry,
		Processor: processor,
	}

	// Create server
	srv, err := server.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}
	defer srv.Close()

	// Set up signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("Server running. Press Ctrl+C to stop.\n")

	// Run server (blocks until context cancelled or error)
	if err := srv.Run(ctx); err != nil && err != context.Canceled {
		return fmt.Errorf("server error: %w", err)
	}

	fmt.Printf("Server stopped.\n")
	return nil
}

func runClient(ctx context.Context, listenAddr, serverAddr string) error {
	fmt.Printf("Starting client mode...\n")
	fmt.Printf("  Listen address: %s\n", listenAddr)
	fmt.Printf("  Server address: %s\n", serverAddr)

	// Create client configuration
	cfg := &client.ClientConfig{
		ListenAddr: listenAddr,
		ServerAddr: serverAddr,
	}

	// Create client
	c, err := client.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	// Set up signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("Client running. Press Ctrl+C to stop.\n")

	// Run client (blocks until context cancelled or error)
	if err := c.Run(ctx); err != nil && err != context.Canceled {
		return fmt.Errorf("client error: %w", err)
	}

	fmt.Printf("Client stopped.\n")
	return nil
}
