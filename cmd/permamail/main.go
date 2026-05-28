// Package main implements the permamail CLI tool for managing email forwarding via Namecoin.
//
// The permamail tool provides a simple command-line interface for:
//   - Registering new .bit email addresses (NAME_NEW + NAME_FIRSTUPDATE)
//   - Updating email forwarding configuration (NAME_UPDATE)
//   - Looking up current email configuration (name_show)
//   - Running an SMTP relay server for .bit email forwarding
//
// See ROADMAP.md Phase 4 for implementation details.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/opd-ai/nmcd/bridge"
	"github.com/opd-ai/nmcd/client"
	"github.com/opd-ai/nmcd/internal/version"
	"github.com/opd-ai/nmcd/mail"
)

var usage = `permamail - Decentralized email forwarding via Namecoin

Usage:
  permamail <command> [options]

Commands:
  register <name> --forward <email>     Register a new .bit email address
  update <name> --forward <email>       Update email forwarding configuration
  lookup <name>                         Display current forwarding configuration
  serve                                 Start SMTP relay server

Options:
  --network <net>      Network to use (mainnet, testnet, regtest) [default: mainnet]
  --datadir <dir>      Data directory [default: ~/.nmcd]
  --rpcaddr <addr>     Namecoin RPC address [default: localhost:8336]
  --rpcuser <user>     RPC username
  --rpcpass <pass>     RPC password
  --forward <email>    Email address to forward to (for register/update)
  --backup <email>     Backup email address (optional, can specify multiple)
  --listen <addr>      SMTP listen address [default: :2525]
  --upstream <host>    Upstream SMTP server
  --upstreamport <n>   Upstream SMTP port [default: 587]
  --smtpuser <user>    Upstream SMTP username
  --smtppass <pass>    Upstream SMTP password

Examples:
  # Register new .bit email address
  permamail register alice --forward user@gmail.com

  # Update with backup address
  permamail update alice --forward newemail@proton.me --backup backup@proton.me

  # Look up configuration
  permamail lookup alice

  # Start SMTP relay server
  permamail serve --upstream smtp.sendgrid.net --upstreamport 587 \
                  --smtpuser apikey --smtppass <api-key>

Version: ` + version.Version + `
`

// CLI holds command-line configuration
type CLI struct {
	// Global options
	network     string
	dataDir     string
	rpcAddr     string
	rpcUser     string
	rpcPassword string

	// Register/update options
	forwardTo string
	backups   []string

	// Serve options
	listenAddr   string
	upstreamHost string
	upstreamPort int
	smtpUser     string
	smtpPassword string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "%s", usage)
		os.Exit(1)
	}

	command := os.Args[1]
	cli := &CLI{}

	switch command {
	case "register":
		if err := cli.register(os.Args[2:]); err != nil {
			log.Fatalf("Register failed: %v", err)
		}
	case "update":
		if err := cli.update(os.Args[2:]); err != nil {
			log.Fatalf("Update failed: %v", err)
		}
	case "lookup":
		if err := cli.lookup(os.Args[2:]); err != nil {
			log.Fatalf("Lookup failed: %v", err)
		}
	case "serve":
		if err := cli.serve(os.Args[2:]); err != nil {
			log.Fatalf("Serve failed: %v", err)
		}
	case "help", "-h", "--help":
		fmt.Fprintf(os.Stdout, "%s", usage)
		os.Exit(0)
	case "version", "-v", "--version":
		fmt.Printf("permamail version %s\n", version.Version)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n%s", command, usage)
		os.Exit(1)
	}
}

// parseGlobalFlags parses flags common to all commands
func (c *CLI) parseGlobalFlags(fs *flag.FlagSet) {
	homeDir, _ := os.UserHomeDir()
	defaultDataDir := filepath.Join(homeDir, ".nmcd")

	fs.StringVar(&c.network, "network", "mainnet", "Network to use (mainnet, testnet, regtest)")
	fs.StringVar(&c.dataDir, "datadir", defaultDataDir, "Data directory")
	fs.StringVar(&c.rpcAddr, "rpcaddr", "localhost:8336", "Namecoin RPC address")
	fs.StringVar(&c.rpcUser, "rpcuser", "", "RPC username")
	fs.StringVar(&c.rpcPassword, "rpcpass", "", "RPC password")
}

// parseBackupAddresses parses a comma-separated string of backup email addresses
// and returns a slice of trimmed addresses. Empty strings and whitespace-only
// entries are filtered out.
func parseBackupAddresses(backupAddrs string) []string {
	if backupAddrs == "" {
		return nil
	}

	parts := strings.Split(backupAddrs, ",")
	backups := make([]string, 0, len(parts))

	for _, addr := range parts {
		trimmed := strings.TrimSpace(addr)
		if trimmed != "" {
			backups = append(backups, trimmed)
		}
	}

	return backups
}

// nameCommandConfig holds the result of parsing a name command (register or update).
type nameCommandConfig struct {
	name     string
	mailJSON []byte
}

// parseNameCommand parses common flags and arguments for register/update commands.
func (c *CLI) parseNameCommand(cmdName string, args []string) (*nameCommandConfig, error) {
	fs := flag.NewFlagSet(cmdName, flag.ExitOnError)
	c.parseGlobalFlags(fs)

	var backupAddrs string
	fs.StringVar(&c.forwardTo, "forward", "", "Email address to forward to (required)")
	fs.StringVar(&backupAddrs, "backup", "", "Backup email addresses (comma-separated)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	c.backups = parseBackupAddresses(backupAddrs)

	if fs.NArg() < 1 {
		return nil, fmt.Errorf("name required: permamail %s <name> --forward <email>", cmdName)
	}
	name := fs.Arg(0)

	if c.forwardTo == "" {
		return nil, fmt.Errorf("--forward flag required")
	}

	nc, err := c.createClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer nc.Close()

	mailConfig := map[string]interface{}{"email": c.forwardTo}
	if len(c.backups) > 0 {
		mailConfig["backup"] = c.backups
	}

	mailJSON, err := json.Marshal(mailConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create mail config: %w", err)
	}

	return &nameCommandConfig{name: name, mailJSON: mailJSON}, nil
}

// register implements the register command
func (c *CLI) register(args []string) error {
	cfg, err := c.parseNameCommand("register", args)
	if err != nil {
		return err
	}

	fmt.Printf("Registering name: %s\n", cfg.name)
	fmt.Printf("Forward to: %s\n", c.forwardTo)
	if len(c.backups) > 0 {
		fmt.Printf("Backup addresses: %v\n", c.backups)
	}
	fmt.Printf("Mail config: %s\n", string(cfg.mailJSON))

	fmt.Println("\nNOTE: Name registration requires NAME_NEW + NAME_FIRSTUPDATE operations.")
	fmt.Println("This requires a running nmcd node with wallet enabled.")
	fmt.Println("\nTo complete registration, use the nmcd RPC interface:")
	fmt.Printf("1. Create NAME_NEW commitment (wait 12 blocks)\n")
	fmt.Printf("2. NAME_FIRSTUPDATE with value: %s\n", string(cfg.mailJSON))
	fmt.Println("\nFull wallet integration is planned for future releases.")

	return nil
}

// update implements the update command
func (c *CLI) update(args []string) error {
	cfg, err := c.parseNameCommand("update", args)
	if err != nil {
		return err
	}

	fmt.Printf("Updating name: %s\n", cfg.name)
	fmt.Printf("New forward address: %s\n", c.forwardTo)
	if len(c.backups) > 0 {
		fmt.Printf("Backup addresses: %v\n", c.backups)
	}
	fmt.Printf("New mail config: %s\n", string(cfg.mailJSON))

	fmt.Println("\nNOTE: Name updates require the NAME_UPDATE operation.")
	fmt.Println("This requires a running nmcd node with wallet enabled.")
	fmt.Println("\nTo complete update, use the nmcd RPC interface:")
	fmt.Printf("  name_update \"%s\" \"%s\"\n", cfg.name, string(cfg.mailJSON))
	fmt.Println("\nFull wallet integration is planned for future releases.")

	return nil
}

// lookup implements the lookup command
func (c *CLI) lookup(args []string) error {
	fs := flag.NewFlagSet("lookup", flag.ExitOnError)
	c.parseGlobalFlags(fs)

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Get name from positional argument
	if fs.NArg() < 1 {
		return fmt.Errorf("name required: permamail lookup <name>")
	}
	name := fs.Arg(0)

	// Create Namecoin client
	nc, err := c.createClient()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer nc.Close()

	// Create bridge resolver
	resolver := bridge.NewNamecoinBridge(nc)

	// Lookup mail configuration
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mailConfig, err := resolver.LookupMail(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to lookup name: %w", err)
	}

	// Display configuration
	fmt.Printf("Name: %s\n", name)
	fmt.Printf("Forward to: %s\n", mailConfig.ForwardTo)
	if len(mailConfig.BackupAddrs) > 0 {
		fmt.Printf("Backup addresses:\n")
		for _, addr := range mailConfig.BackupAddrs {
			fmt.Printf("  - %s\n", addr)
		}
	}
	if len(mailConfig.PublicKey) > 0 {
		fmt.Printf("Public key: %d bytes\n", len(mailConfig.PublicKey))
	}

	return nil
}

// serve implements the serve command
func (c *CLI) serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	c.parseGlobalFlags(fs)

	fs.StringVar(&c.listenAddr, "listen", ":2525", "SMTP listen address")
	fs.StringVar(&c.upstreamHost, "upstream", "", "Upstream SMTP server (required)")
	fs.IntVar(&c.upstreamPort, "upstreamport", 587, "Upstream SMTP port")
	fs.StringVar(&c.smtpUser, "smtpuser", "", "Upstream SMTP username")
	fs.StringVar(&c.smtpPassword, "smtppass", "", "Upstream SMTP password")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Validate required flags
	if c.upstreamHost == "" {
		return fmt.Errorf("--upstream flag required")
	}

	// Create Namecoin client
	nc, err := c.createClient()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer nc.Close()

	// Create bridge resolver
	resolver := bridge.NewNamecoinBridge(nc)

	// Create mail router with 1 hour cache TTL
	router := mail.NewRouter(resolver, time.Hour)

	// Create relay configuration
	relayConfig := mail.DefaultRelayConfig()
	relayConfig.ListenAddr = c.listenAddr
	relayConfig.UpstreamHost = c.upstreamHost
	relayConfig.UpstreamPort = c.upstreamPort

	// Setup upstream authentication if credentials provided
	if c.smtpUser != "" && c.smtpPassword != "" {
		relayConfig.UpstreamAuth = smtp.PlainAuth(
			"",
			c.smtpUser,
			c.smtpPassword,
			c.upstreamHost,
		)
	}

	// Create and start relay
	relay := mail.NewRelay(router, relayConfig)

	fmt.Printf("Starting SMTP relay server...\n")
	fmt.Printf("Listen address: %s\n", c.listenAddr)
	fmt.Printf("Upstream SMTP: %s:%d\n", c.upstreamHost, c.upstreamPort)
	fmt.Printf("Network: %s\n", c.network)
	fmt.Printf("\n")

	if err := relay.Start(); err != nil {
		return fmt.Errorf("failed to start relay: %w", err)
	}
	defer relay.Stop()

	fmt.Printf("SMTP relay running. Press Ctrl+C to stop.\n")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)
	<-sigChan

	fmt.Println("\nShutting down...")
	return nil
}

// createClient constructs a NameClient configured for daemon mode using the
// current CLI settings. It reads network, data directory, and RPC credentials
// from the CLI fields populated by global flags, and always sets Mode to
// client.ModeDaemon so that the client talks to a running nmcd instance rather
// than embedding its own chain. The returned client must be closed by the
// caller when no longer needed.
func (c *CLI) createClient() (client.NameClient, error) {
	cfg := &client.Config{
		Mode:        client.ModeDaemon, // Use daemon mode for CLI
		Network:     c.network,
		DataDir:     c.dataDir,
		RPCAddr:     c.rpcAddr,
		RPCUser:     c.rpcUser,
		RPCPassword: c.rpcPassword,
	}

	return client.NewClient(cfg)
}
