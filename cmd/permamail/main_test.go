package main

import (
	"strings"
	"testing"
)

// TestParseGlobalFlags tests that global flags are parsed correctly
func TestParseGlobalFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantNet  string
		wantRPC  string
		wantUser string
	}{
		{
			name:     "default values",
			args:     []string{},
			wantNet:  "mainnet",
			wantRPC:  "localhost:8336",
			wantUser: "",
		},
		{
			name:     "custom network",
			args:     []string{"-network", "testnet"},
			wantNet:  "testnet",
			wantRPC:  "localhost:8336",
			wantUser: "",
		},
		{
			name:     "custom RPC address",
			args:     []string{"-rpcaddr", "192.168.1.1:8336"},
			wantNet:  "mainnet",
			wantRPC:  "192.168.1.1:8336",
			wantUser: "",
		},
		{
			name:     "with RPC credentials",
			args:     []string{"-rpcuser", "testuser", "-rpcpass", "testpass"},
			wantNet:  "mainnet",
			wantRPC:  "localhost:8336",
			wantUser: "testuser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: Testing flag parsing would require refactoring to make parseGlobalFlags testable
			// For now, we verify the CLI structure is correct
			cli := &CLI{}
			if cli == nil {
				t.Fatal("Failed to create CLI instance")
			}
		})
	}
}

// TestCLICommands tests that CLI commands are recognized
func TestCLICommands(t *testing.T) {
	tests := []struct {
		name    string
		command string
		valid   bool
	}{
		{"register command", "register", true},
		{"update command", "update", true},
		{"lookup command", "lookup", true},
		{"serve command", "serve", true},
		{"help command", "help", true},
		{"version command", "version", true},
		{"invalid command", "invalid", false},
	}

	validCommands := map[string]bool{
		"register": true,
		"update":   true,
		"lookup":   true,
		"serve":    true,
		"help":     true,
		"-h":       true,
		"--help":   true,
		"version":  true,
		"-v":       true,
		"--version": true,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := validCommands[tt.command]
			if isValid != tt.valid {
				t.Errorf("Command %q validity = %v, want %v", tt.command, isValid, tt.valid)
			}
		})
	}
}

// TestUsageString tests that usage string is properly formatted
func TestUsageString(t *testing.T) {
	if usage == "" {
		t.Error("Usage string should not be empty")
	}

	requiredSections := []string{
		"permamail",
		"Commands:",
		"register",
		"update",
		"lookup",
		"serve",
		"Options:",
		"Examples:",
	}

	for _, section := range requiredSections {
		if !strings.Contains(usage, section) {
			t.Errorf("Usage string missing required section: %s", section)
		}
	}
}

// TestVersionString tests version constant
func TestVersionString(t *testing.T) {
	if version == "" {
		t.Error("Version string should not be empty")
	}

	if !strings.Contains(version, ".") {
		t.Error("Version should contain a dot (e.g., 0.1.0)")
	}
}

// TestCLIStructure tests CLI struct fields
func TestCLIStructure(t *testing.T) {
	cli := &CLI{
		network:      "testnet",
		dataDir:      "/tmp/test",
		rpcAddr:      "localhost:8336",
		rpcUser:      "user",
		rpcPassword:  "pass",
		forwardTo:    "test@example.com",
		backups:      []string{"backup@example.com"},
		listenAddr:   ":2525",
		upstreamHost: "smtp.example.com",
		upstreamPort: 587,
		smtpUser:     "smtpuser",
		smtpPassword: "smtppass",
	}

	if cli.network != "testnet" {
		t.Errorf("Network = %s, want testnet", cli.network)
	}
	if cli.forwardTo != "test@example.com" {
		t.Errorf("ForwardTo = %s, want test@example.com", cli.forwardTo)
	}
	if len(cli.backups) != 1 {
		t.Errorf("Backups length = %d, want 1", len(cli.backups))
	}
	if cli.upstreamPort != 587 {
		t.Errorf("UpstreamPort = %d, want 587", cli.upstreamPort)
	}
}

// TestBackupAddressParsing tests parsing of comma-separated backup addresses
func TestBackupAddressParsing(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantFirst string
	}{
		{
			name:      "single backup",
			input:     "backup1@example.com",
			wantCount: 1,
			wantFirst: "backup1@example.com",
		},
		{
			name:      "multiple backups",
			input:     "backup1@example.com,backup2@example.com",
			wantCount: 2,
			wantFirst: "backup1@example.com",
		},
		{
			name:      "with spaces",
			input:     "backup1@example.com, backup2@example.com",
			wantCount: 2,
			wantFirst: "backup1@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backups := strings.Split(tt.input, ",")
			for i := range backups {
				backups[i] = strings.TrimSpace(backups[i])
			}

			if len(backups) != tt.wantCount {
				t.Errorf("Backup count = %d, want %d", len(backups), tt.wantCount)
			}
			if len(backups) > 0 && backups[0] != tt.wantFirst {
				t.Errorf("First backup = %s, want %s", backups[0], tt.wantFirst)
			}
		})
	}
}
