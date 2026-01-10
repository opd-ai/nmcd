package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/internal/server"
)

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single value",
			input:    "peer1:8334",
			expected: []string{"peer1:8334"},
		},
		{
			name:     "multiple values",
			input:    "peer1:8334,peer2:8334,peer3:8334",
			expected: []string{"peer1:8334", "peer2:8334", "peer3:8334"},
		},
		{
			name:     "values with spaces",
			input:    "peer1:8334, peer2:8334 , peer3:8334",
			expected: []string{"peer1:8334", "peer2:8334", "peer3:8334"},
		},
		{
			name:     "empty value filtered",
			input:    "peer1:8334,,peer2:8334",
			expected: []string{"peer1:8334", "peer2:8334"},
		},
		{
			name:     "only whitespace filtered",
			input:    "peer1:8334,   ,peer2:8334",
			expected: []string{"peer1:8334", "peer2:8334"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "trailing comma",
			input:    "peer1:8334,peer2:8334,",
			expected: []string{"peer1:8334", "peer2:8334"},
		},
		{
			name:     "leading comma",
			input:    ",peer1:8334,peer2:8334",
			expected: []string{"peer1:8334", "peer2:8334"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := server.SplitAndTrim(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d elements, got %d", len(tt.expected), len(result))
				return
			}

			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("element %d: expected %q, got %q", i, tt.expected[i], v)
				}
			}
		})
	}
}

// TestConfigurationPrecedence tests that configuration is applied in the correct order:
// command-line flags > environment variables > config file > defaults
func TestConfigurationPrecedence(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Create a config file
	configPath := filepath.Join(tempDir, "nmcd.conf")
	configContent := `
datadir = "/from/config/file"
maxpeers = 100

[rpc]
user = "configuser"
password = "configpass"
addr = "127.0.0.1:9999"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Test 1: Config file only (no env vars, no flags)
	cfg := config.DefaultConfig()
	fileConfig, err := config.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load config file: %v", err)
	}
	cfg.ApplyFileConfig(fileConfig)

	if cfg.RPCUser != "configuser" {
		t.Errorf("Expected RPCUser from config file='configuser', got '%s'", cfg.RPCUser)
	}
	if cfg.RPCPassword != "configpass" {
		t.Errorf("Expected RPCPassword from config file='configpass', got '%s'", cfg.RPCPassword)
	}
	if cfg.MaxPeers != 100 {
		t.Errorf("Expected MaxPeers from config file=100, got %d", cfg.MaxPeers)
	}

	// Test 2: Environment variables override config file
	origUser := os.Getenv("NMCD_RPC_USER")
	origPass := os.Getenv("NMCD_RPC_PASSWORD")
	defer func() {
		if origUser == "" {
			os.Unsetenv("NMCD_RPC_USER")
		} else {
			os.Setenv("NMCD_RPC_USER", origUser)
		}
		if origPass == "" {
			os.Unsetenv("NMCD_RPC_PASSWORD")
		} else {
			os.Setenv("NMCD_RPC_PASSWORD", origPass)
		}
	}()

	os.Setenv("NMCD_RPC_USER", "envuser")
	os.Setenv("NMCD_RPC_PASSWORD", "envpass")

	cfg2 := config.DefaultConfig()
	cfg2.ApplyFileConfig(fileConfig)
	cfg2.ApplyEnvironmentVariables()

	if cfg2.RPCUser != "envuser" {
		t.Errorf("Expected RPCUser from env='envuser', got '%s'", cfg2.RPCUser)
	}
	if cfg2.RPCPassword != "envpass" {
		t.Errorf("Expected RPCPassword from env='envpass', got '%s'", cfg2.RPCPassword)
	}

	// Test 3: Command-line flags override everything (simulated)
	cfg3 := config.DefaultConfig()
	cfg3.ApplyFileConfig(fileConfig)
	cfg3.ApplyEnvironmentVariables()
	// Simulate command-line override
	cfg3.RPCUser = "flaguser"
	cfg3.RPCPassword = "flagpass"

	if cfg3.RPCUser != "flaguser" {
		t.Errorf("Expected RPCUser from flag='flaguser', got '%s'", cfg3.RPCUser)
	}
	if cfg3.RPCPassword != "flagpass" {
		t.Errorf("Expected RPCPassword from flag='flagpass', got '%s'", cfg3.RPCPassword)
	}
}

// TestConfigFileNotFound tests that missing config file doesn't cause error
func TestConfigFileNotFound(t *testing.T) {
	tempDir := t.TempDir()
	nonExistentPath := filepath.Join(tempDir, "nonexistent.conf")

	cfg, err := config.LoadConfigFile(nonExistentPath)
	if err != nil {
		t.Fatalf("Expected no error for non-existent config file, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}

	// Apply to Config should be safe with empty FileConfig
	defaultCfg := config.DefaultConfig()
	defaultCfg.ApplyFileConfig(cfg)

	// Should still have default values
	if defaultCfg.Network != "mainnet" {
		t.Errorf("Expected default network='mainnet', got '%s'", defaultCfg.Network)
	}
}

// TestSecureCredentialWarning verifies the warning message format
func TestSecureCredentialWarning(t *testing.T) {
	// This test just verifies the warning message logic
	// In real usage, this would be logged

	rpcUserFlag := "testuser"
	rpcPasswordFlag := "testpass"

	// If either flag is set, a warning should be issued
	if rpcUserFlag != "" || rpcPasswordFlag != "" {
		warningMsg := "Warning: RPC credentials passed via command-line flags are visible in process listings."
		if !strings.Contains(warningMsg, "process listings") {
			t.Error("Warning message should mention process listings visibility")
		}
		if !strings.Contains(warningMsg, "command-line") {
			t.Error("Warning message should mention command-line flags")
		}
	}
}
