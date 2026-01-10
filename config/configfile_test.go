package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfigFile_NotExists tests loading a non-existent config file.
// Should return empty config without error.
func TestLoadConfigFile_NotExists(t *testing.T) {
	nonExistentPath := filepath.Join(t.TempDir(), "nonexistent.conf")

	cfg, err := LoadConfigFile(nonExistentPath)
	if err != nil {
		t.Fatalf("Expected no error for non-existent file, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}
}

// TestLoadConfigFile_Valid tests loading a valid TOML config file.
func TestLoadConfigFile_Valid(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "nmcd.conf")

	// Create a valid config file
	// Note: In TOML, top-level fields must come before any [section] declarations
	configContent := `
datadir = "/custom/data/dir"
prometheusaddr = "127.0.0.1:9090"
maxpeers = 50

[rpc]
user = "testuser"
password = "testpassword"
addr = "127.0.0.1:18336"

[network]
type = "testnet"
listen = ["0.0.0.0:18334", "127.0.0.1:18334"]
addpeer = ["peer1.example.com:18334", "peer2.example.com:18334"]
`

	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load config
	cfg, err := LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify RPC settings
	if cfg.RPC.User != "testuser" {
		t.Errorf("Expected RPC.User='testuser', got '%s'", cfg.RPC.User)
	}
	if cfg.RPC.Password != "testpassword" {
		t.Errorf("Expected RPC.Password='testpassword', got '%s'", cfg.RPC.Password)
	}
	if cfg.RPC.Addr != "127.0.0.1:18336" {
		t.Errorf("Expected RPC.Addr='127.0.0.1:18336', got '%s'", cfg.RPC.Addr)
	}

	// Verify network settings
	if cfg.Network.Type != "testnet" {
		t.Errorf("Expected Network.Type='testnet', got '%s'", cfg.Network.Type)
	}
	if len(cfg.Network.Listen) != 2 {
		t.Errorf("Expected 2 listen addresses, got %d", len(cfg.Network.Listen))
	}
	if len(cfg.Network.AddPeer) != 2 {
		t.Errorf("Expected 2 addpeer addresses, got %d", len(cfg.Network.AddPeer))
	}

	// Verify general settings
	if cfg.DataDir != "/custom/data/dir" {
		t.Errorf("Expected DataDir='/custom/data/dir', got '%s'", cfg.DataDir)
	}
	if cfg.PrometheusAddr != "127.0.0.1:9090" {
		t.Errorf("Expected PrometheusAddr='127.0.0.1:9090', got '%s'", cfg.PrometheusAddr)
	}
	if cfg.MaxPeers != 50 {
		t.Errorf("Expected MaxPeers=50, got %d", cfg.MaxPeers)
	}
}

// TestLoadConfigFile_Invalid tests loading an invalid TOML file.
func TestLoadConfigFile_Invalid(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid.conf")

	// Create an invalid config file (malformed TOML)
	invalidContent := `
[rpc
user = "incomplete
`

	if err := os.WriteFile(configPath, []byte(invalidContent), 0o600); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Try to load config - should fail
	_, err := LoadConfigFile(configPath)
	if err == nil {
		t.Fatal("Expected error for invalid TOML, got nil")
	}
}

// TestApplyFileConfig tests applying file config to Config struct.
func TestApplyFileConfig(t *testing.T) {
	cfg := DefaultConfig()

	fileConfig := &FileConfig{}
	fileConfig.RPC.User = "fileuser"
	fileConfig.RPC.Password = "filepassword"
	fileConfig.RPC.Addr = "127.0.0.1:9999"
	fileConfig.Network.Type = "regtest"
	fileConfig.Network.Listen = []string{"127.0.0.1:19444"}
	fileConfig.Network.AddPeer = []string{"peer.example.com:19444"}
	fileConfig.DataDir = "/tmp/testdata"
	fileConfig.PrometheusAddr = "127.0.0.1:8888"
	fileConfig.MaxPeers = 42

	cfg.ApplyFileConfig(fileConfig)

	// Verify all settings were applied
	if cfg.RPCUser != "fileuser" {
		t.Errorf("Expected RPCUser='fileuser', got '%s'", cfg.RPCUser)
	}
	if cfg.RPCPassword != "filepassword" {
		t.Errorf("Expected RPCPassword='filepassword', got '%s'", cfg.RPCPassword)
	}
	if cfg.RPCAddr != "127.0.0.1:9999" {
		t.Errorf("Expected RPCAddr='127.0.0.1:9999', got '%s'", cfg.RPCAddr)
	}
	if cfg.Network != "regtest" {
		t.Errorf("Expected Network='regtest', got '%s'", cfg.Network)
	}
	if len(cfg.ListenAddrs) != 1 || cfg.ListenAddrs[0] != "127.0.0.1:19444" {
		t.Errorf("Expected ListenAddrs=['127.0.0.1:19444'], got %v", cfg.ListenAddrs)
	}
	if len(cfg.AddPeers) != 1 || cfg.AddPeers[0] != "peer.example.com:19444" {
		t.Errorf("Expected AddPeers=['peer.example.com:19444'], got %v", cfg.AddPeers)
	}
	if cfg.DataDir != "/tmp/testdata" {
		t.Errorf("Expected DataDir='/tmp/testdata', got '%s'", cfg.DataDir)
	}
	if cfg.PrometheusAddr != "127.0.0.1:8888" {
		t.Errorf("Expected PrometheusAddr='127.0.0.1:8888', got '%s'", cfg.PrometheusAddr)
	}
	if cfg.MaxPeers != 42 {
		t.Errorf("Expected MaxPeers=42, got %d", cfg.MaxPeers)
	}
}

// TestApplyFileConfig_PartialOverride tests that file config only overrides non-empty values.
func TestApplyFileConfig_PartialOverride(t *testing.T) {
	cfg := DefaultConfig()
	originalRPCAddr := cfg.RPCAddr
	originalNetwork := cfg.Network

	// Create file config with only some fields set
	fileConfig := &FileConfig{}
	fileConfig.RPC.User = "newuser"
	// Leave RPC.Password, RPC.Addr empty

	cfg.ApplyFileConfig(fileConfig)

	// User should be updated
	if cfg.RPCUser != "newuser" {
		t.Errorf("Expected RPCUser='newuser', got '%s'", cfg.RPCUser)
	}

	// Other fields should remain at defaults
	if cfg.RPCPassword != "" {
		t.Errorf("Expected RPCPassword='', got '%s'", cfg.RPCPassword)
	}
	if cfg.RPCAddr != originalRPCAddr {
		t.Errorf("Expected RPCAddr to remain '%s', got '%s'", originalRPCAddr, cfg.RPCAddr)
	}
	if cfg.Network != originalNetwork {
		t.Errorf("Expected Network to remain '%s', got '%s'", originalNetwork, cfg.Network)
	}
}

// TestApplyEnvironmentVariables tests reading config from environment variables.
func TestApplyEnvironmentVariables(t *testing.T) {
	// Save original env vars
	origVars := map[string]string{
		"NMCD_RPC_USER":        os.Getenv("NMCD_RPC_USER"),
		"NMCD_RPC_PASSWORD":    os.Getenv("NMCD_RPC_PASSWORD"),
		"NMCD_RPC_ADDR":        os.Getenv("NMCD_RPC_ADDR"),
		"NMCD_PROMETHEUS_ADDR": os.Getenv("NMCD_PROMETHEUS_ADDR"),
		"NMCD_NETWORK":         os.Getenv("NMCD_NETWORK"),
		"NMCD_DATADIR":         os.Getenv("NMCD_DATADIR"),
	}

	// Restore env vars after test
	defer func() {
		for k, v := range origVars {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	// Set test environment variables
	os.Setenv("NMCD_RPC_USER", "envuser")
	os.Setenv("NMCD_RPC_PASSWORD", "envpassword")
	os.Setenv("NMCD_RPC_ADDR", "127.0.0.1:7777")
	os.Setenv("NMCD_PROMETHEUS_ADDR", "127.0.0.1:6666")
	os.Setenv("NMCD_NETWORK", "testnet")
	os.Setenv("NMCD_DATADIR", "/tmp/envdata")

	cfg := DefaultConfig()
	cfg.ApplyEnvironmentVariables()

	// Verify all env vars were applied
	if cfg.RPCUser != "envuser" {
		t.Errorf("Expected RPCUser='envuser', got '%s'", cfg.RPCUser)
	}
	if cfg.RPCPassword != "envpassword" {
		t.Errorf("Expected RPCPassword='envpassword', got '%s'", cfg.RPCPassword)
	}
	if cfg.RPCAddr != "127.0.0.1:7777" {
		t.Errorf("Expected RPCAddr='127.0.0.1:7777', got '%s'", cfg.RPCAddr)
	}
	if cfg.PrometheusAddr != "127.0.0.1:6666" {
		t.Errorf("Expected PrometheusAddr='127.0.0.1:6666', got '%s'", cfg.PrometheusAddr)
	}
	if cfg.Network != "testnet" {
		t.Errorf("Expected Network='testnet', got '%s'", cfg.Network)
	}
	if cfg.DataDir != "/tmp/envdata" {
		t.Errorf("Expected DataDir='/tmp/envdata', got '%s'", cfg.DataDir)
	}
}

// TestApplyEnvironmentVariables_NoVars tests that empty env vars don't override defaults.
func TestApplyEnvironmentVariables_NoVars(t *testing.T) {
	// Save and clear env vars
	origVars := map[string]string{
		"NMCD_RPC_USER":        os.Getenv("NMCD_RPC_USER"),
		"NMCD_RPC_PASSWORD":    os.Getenv("NMCD_RPC_PASSWORD"),
		"NMCD_RPC_ADDR":        os.Getenv("NMCD_RPC_ADDR"),
		"NMCD_PROMETHEUS_ADDR": os.Getenv("NMCD_PROMETHEUS_ADDR"),
		"NMCD_NETWORK":         os.Getenv("NMCD_NETWORK"),
		"NMCD_DATADIR":         os.Getenv("NMCD_DATADIR"),
	}

	for k := range origVars {
		os.Unsetenv(k)
	}

	// Restore env vars after test
	defer func() {
		for k, v := range origVars {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	cfg := DefaultConfig()
	originalRPCAddr := cfg.RPCAddr
	originalNetwork := cfg.Network

	cfg.ApplyEnvironmentVariables()

	// Verify defaults weren't changed
	if cfg.RPCUser != "" {
		t.Errorf("Expected RPCUser='', got '%s'", cfg.RPCUser)
	}
	if cfg.RPCPassword != "" {
		t.Errorf("Expected RPCPassword='', got '%s'", cfg.RPCPassword)
	}
	if cfg.RPCAddr != originalRPCAddr {
		t.Errorf("Expected RPCAddr='%s', got '%s'", originalRPCAddr, cfg.RPCAddr)
	}
	if cfg.Network != originalNetwork {
		t.Errorf("Expected Network='%s', got '%s'", originalNetwork, cfg.Network)
	}
}

// TestConfigPath tests the config file path generation.
func TestConfigPath(t *testing.T) {
	path := ConfigPath("/home/user/.nmcd")
	expected := filepath.Join("/home/user/.nmcd", "nmcd.conf")
	if path != expected {
		t.Errorf("Expected path='%s', got '%s'", expected, path)
	}
}
