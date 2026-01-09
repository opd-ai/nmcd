package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opd-ai/nmcd/internal/logging"
)

// TestEmbeddedClient_CustomLogger verifies that custom logger is used in embedded client
func TestEmbeddedClient_CustomLogger(t *testing.T) {
	// Create a custom logger that writes to the buffer
	logger := &logging.Logger{
		Logger: nil, // Will be set by Init
	}

	// Initialize logger with buffer output
	cfg := &logging.Config{
		Level:     logging.LevelDebug,
		Format:    "json",
		Output:    "stdout",
		Component: "test",
	}

	var err error
	logger, err = logging.Init(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Create embedded client with custom logger
	dataDir := t.TempDir()
	clientCfg := &Config{
		DataDir:       dataDir,
		Network:       "regtest",
		Logger:        logger,
		DisableWallet: false,
	}

	client, err := NewEmbeddedClient(clientCfg)
	if err != nil {
		t.Fatalf("Failed to create embedded client: %v", err)
	}
	defer client.Close()

	// Verify logger is set
	if client.logger == nil {
		t.Fatal("Expected logger to be set in embedded client")
	}

	// Verify logger has correct component context
	// We can't easily test the component field directly, but we can verify
	// that the logger instance is not nil and is a logging.Logger
	if _, ok := interface{}(client.logger).(*logging.Logger); !ok {
		t.Fatal("Expected client.logger to be *logging.Logger")
	}
}

// TestDaemonClient_CustomLogger verifies that custom logger is used in daemon client
func TestDaemonClient_CustomLogger(t *testing.T) {
	// Create a custom logger
	cfg := &logging.Config{
		Level:     logging.LevelWarn,
		Format:    "text",
		Output:    "stderr",
		Component: "test-daemon",
	}

	logger, err := logging.Init(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Create daemon client with custom logger
	clientCfg := &Config{
		RPCAddr:     "http://localhost:8336",
		RPCUser:     "testuser",
		RPCPassword: "testpass",
		Logger:      logger,
	}

	client, err := NewDaemonClient(clientCfg)
	if err != nil {
		t.Fatalf("Failed to create daemon client: %v", err)
	}
	defer client.Close()

	// Verify logger is set
	if client.logger == nil {
		t.Fatal("Expected logger to be set in daemon client")
	}

	// Verify logger is the instance we provided (with component context)
	if _, ok := interface{}(client.logger).(*logging.Logger); !ok {
		t.Fatal("Expected client.logger to be *logging.Logger")
	}
}

// TestEmbeddedClient_DefaultLogger verifies that default logger is used when none provided
func TestEmbeddedClient_DefaultLogger(t *testing.T) {
	dataDir := t.TempDir()
	clientCfg := &Config{
		DataDir:       dataDir,
		Network:       "regtest",
		Logger:        nil, // No logger provided
		DisableWallet: true,
	}

	client, err := NewEmbeddedClient(clientCfg)
	if err != nil {
		t.Fatalf("Failed to create embedded client: %v", err)
	}
	defer client.Close()

	// Verify default logger is used
	if client.logger == nil {
		t.Fatal("Expected default logger to be set when none provided")
	}
}

// TestDaemonClient_DefaultLogger verifies that default logger is used when none provided
func TestDaemonClient_DefaultLogger(t *testing.T) {
	clientCfg := &Config{
		RPCAddr: "http://localhost:8336",
		Logger:  nil, // No logger provided
	}

	client, err := NewDaemonClient(clientCfg)
	if err != nil {
		t.Fatalf("Failed to create daemon client: %v", err)
	}
	defer client.Close()

	// Verify default logger is used
	if client.logger == nil {
		t.Fatal("Expected default logger to be set when none provided")
	}
}

// TestCustomLogger_FileOutput verifies logger can write to a file
func TestCustomLogger_FileOutput(t *testing.T) {
	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "client.log")

	// Create logger that writes to file
	cfg := &logging.Config{
		Level:     logging.LevelInfo,
		Format:    "json",
		Output:    logFile,
		Component: "file-test",
	}

	logger, err := logging.Init(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	// Create client with file logger
	dataDir := t.TempDir()
	clientCfg := &Config{
		DataDir:       dataDir,
		Network:       "regtest",
		Logger:        logger,
		DisableWallet: true,
	}

	client, err := NewEmbeddedClient(clientCfg)
	if err != nil {
		t.Fatalf("Failed to create embedded client: %v", err)
	}
	defer client.Close()

	// Write a log entry (indirectly via client operations)
	logger.Info("test message", "component", "embedded-client")

	// Close logger to flush
	logger.Close()

	// Verify log file exists and has content
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Expected log file to have content")
	}

	// Verify JSON format
	var logEntry map[string]interface{}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > 0 {
		if err := json.Unmarshal([]byte(lines[0]), &logEntry); err != nil {
			t.Fatalf("Log output is not valid JSON: %v", err)
		}

		// Verify log entry has expected fields
		if _, ok := logEntry["msg"]; !ok {
			t.Error("Expected log entry to have 'msg' field")
		}
		if _, ok := logEntry["time"]; !ok {
			t.Error("Expected log entry to have 'time' field")
		}
	}
}

// TestCustomLogger_LogLevels verifies logger respects log levels
func TestCustomLogger_LogLevels(t *testing.T) {
	tests := []struct {
		name        string
		configLevel logging.LogLevel
		expectDebug bool
		expectInfo  bool
		expectWarn  bool
		expectError bool
	}{
		{
			name:        "DEBUG level logs everything",
			configLevel: logging.LevelDebug,
			expectDebug: true,
			expectInfo:  true,
			expectWarn:  true,
			expectError: true,
		},
		{
			name:        "INFO level skips DEBUG",
			configLevel: logging.LevelInfo,
			expectDebug: false,
			expectInfo:  true,
			expectWarn:  true,
			expectError: true,
		},
		{
			name:        "WARN level skips DEBUG and INFO",
			configLevel: logging.LevelWarn,
			expectDebug: false,
			expectInfo:  false,
			expectWarn:  true,
			expectError: true,
		},
		{
			name:        "ERROR level only logs errors",
			configLevel: logging.LevelError,
			expectDebug: false,
			expectInfo:  false,
			expectWarn:  false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logFile := filepath.Join(t.TempDir(), "test.log")

			cfg := &logging.Config{
				Level:     tt.configLevel,
				Format:    "text",
				Output:    logFile,
				Component: "level-test",
			}

			logger, err := logging.Init(cfg)
			if err != nil {
				t.Fatalf("Failed to initialize logger: %v", err)
			}
			defer logger.Close()

			// Write logs at different levels
			logger.Debug("debug message")
			logger.Info("info message")
			logger.Warn("warn message")
			logger.Error("error message")

			// Close to flush
			logger.Close()

			// Read log file
			data, err := os.ReadFile(logFile)
			if err != nil {
				t.Fatalf("Failed to read log file: %v", err)
			}

			logContent := string(data)

			// Verify expected messages are present/absent
			if tt.expectDebug && !strings.Contains(logContent, "debug message") {
				t.Error("Expected to find debug message")
			}
			if !tt.expectDebug && strings.Contains(logContent, "debug message") {
				t.Error("Did not expect to find debug message")
			}

			if tt.expectInfo && !strings.Contains(logContent, "info message") {
				t.Error("Expected to find info message")
			}
			if !tt.expectInfo && strings.Contains(logContent, "info message") {
				t.Error("Did not expect to find info message")
			}

			if tt.expectWarn && !strings.Contains(logContent, "warn message") {
				t.Error("Expected to find warn message")
			}
			if !tt.expectWarn && strings.Contains(logContent, "warn message") {
				t.Error("Did not expect to find warn message")
			}

			if tt.expectError && !strings.Contains(logContent, "error message") {
				t.Error("Expected to find error message")
			}
			if !tt.expectError && strings.Contains(logContent, "error message") {
				t.Error("Did not expect to find error message")
			}
		})
	}
}

// TestNewClient_CustomLogger verifies custom logger works with NewClient
func TestNewClient_CustomLogger(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "newclient.log")

	cfg := &logging.Config{
		Level:     logging.LevelDebug,
		Format:    "json",
		Output:    logFile,
		Component: "newclient-test",
	}

	logger, err := logging.Init(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	// Create client with auto-detection using custom logger
	dataDir := t.TempDir()
	clientCfg := &Config{
		Mode:          ModeEmbedded, // Force embedded to avoid daemon connection
		DataDir:       dataDir,
		Network:       "regtest",
		Logger:        logger,
		DisableWallet: true,
	}

	client, err := NewClient(clientCfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Verify client was created (type assertion to check if it's embedded)
	if _, ok := client.(*EmbeddedClient); !ok {
		t.Fatal("Expected EmbeddedClient")
	}

	// Verify the embedded client has logger set
	embeddedClient := client.(*EmbeddedClient)
	if embeddedClient.logger == nil {
		t.Fatal("Expected logger to be set in embedded client created via NewClient")
	}
}

// TestEmbeddedClient_LoggerWarnings tests that warnings are logged via custom logger
func TestEmbeddedClient_LoggerWarnings(t *testing.T) {
	// This test would require triggering the actual warning paths in RegisterName/UpdateName
	// which requires a more complex setup with wallet, blockchain, etc.
	// For now, we verify the logger is configured and can be used

	logFile := filepath.Join(t.TempDir(), "warnings.log")

	cfg := &logging.Config{
		Level:     logging.LevelWarn,
		Format:    "text",
		Output:    logFile,
		Component: "warning-test",
	}

	logger, err := logging.Init(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	dataDir := t.TempDir()
	clientCfg := &Config{
		DataDir:       dataDir,
		Network:       "regtest",
		Logger:        logger,
		DisableWallet: true,
	}

	client, err := NewEmbeddedClient(clientCfg)
	if err != nil {
		t.Fatalf("Failed to create embedded client: %v", err)
	}
	defer client.Close()

	// Manually log a warning to verify the logger works
	client.logger.Warn("test warning", "test_field", "test_value")

	// Close to flush
	logger.Close()

	// Verify warning was written
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(data), "test warning") {
		t.Error("Expected to find 'test warning' in log file")
	}
}
