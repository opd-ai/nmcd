package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Level != LevelInfo {
		t.Errorf("Expected default level to be INFO, got %s", cfg.Level)
	}
	if cfg.Format != "text" {
		t.Errorf("Expected default format to be text, got %s", cfg.Format)
	}
	if cfg.Output != "stdout" {
		t.Errorf("Expected default output to be stdout, got %s", cfg.Output)
	}
	if cfg.Component != "nmcd" {
		t.Errorf("Expected default component to be nmcd, got %s", cfg.Component)
	}
}

func TestLogLevels(t *testing.T) {
	tests := []struct {
		name           string
		level          LogLevel
		expectedSlog   slog.Level
		logDebug       bool
		logInfo        bool
		logWarn        bool
		logError       bool
	}{
		{
			name:         "DEBUG level",
			level:        LevelDebug,
			expectedSlog: slog.LevelDebug,
			logDebug:     true,
			logInfo:      true,
			logWarn:      true,
			logError:     true,
		},
		{
			name:         "INFO level",
			level:        LevelInfo,
			expectedSlog: slog.LevelInfo,
			logDebug:     false,
			logInfo:      true,
			logWarn:      true,
			logError:     true,
		},
		{
			name:         "WARN level",
			level:        LevelWarn,
			expectedSlog: slog.LevelWarn,
			logDebug:     false,
			logInfo:      false,
			logWarn:      true,
			logError:     true,
		},
		{
			name:         "ERROR level",
			level:        LevelError,
			expectedSlog: slog.LevelError,
			logDebug:     false,
			logInfo:      false,
			logWarn:      false,
			logError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			// Create logger with custom writer to capture output
			cfg := &Config{
				Level:     tt.level,
				Format:    "text",
				Output:    "stdout",
				Component: "test",
			}

			logger, err := Init(cfg)
			if err != nil {
				t.Fatalf("Failed to init logger: %v", err)
			}

			// Override the logger's handler to write to our buffer
			opts := &slog.HandlerOptions{Level: tt.expectedSlog}
			handler := slog.NewTextHandler(&buf, opts)
			logger.Logger = slog.New(handler)

			// Try logging at different levels
			logger.Debug("debug message")
			debugLogged := strings.Contains(buf.String(), "debug message")

			buf.Reset()
			logger.Info("info message")
			infoLogged := strings.Contains(buf.String(), "info message")

			buf.Reset()
			logger.Warn("warn message")
			warnLogged := strings.Contains(buf.String(), "warn message")

			buf.Reset()
			logger.Error("error message")
			errorLogged := strings.Contains(buf.String(), "error message")

			// Verify expectations
			if debugLogged != tt.logDebug {
				t.Errorf("DEBUG: expected logged=%v, got %v", tt.logDebug, debugLogged)
			}
			if infoLogged != tt.logInfo {
				t.Errorf("INFO: expected logged=%v, got %v", tt.logInfo, infoLogged)
			}
			if warnLogged != tt.logWarn {
				t.Errorf("WARN: expected logged=%v, got %v", tt.logWarn, warnLogged)
			}
			if errorLogged != tt.logError {
				t.Errorf("ERROR: expected logged=%v, got %v", tt.logError, errorLogged)
			}
		})
	}
}

func TestJSONFormat(t *testing.T) {
	var buf bytes.Buffer

	cfg := &Config{
		Level:     LevelInfo,
		Format:    "json",
		Output:    "stdout",
		Component: "test",
	}

	// Override stdout temporarily
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	handler := slog.NewJSONHandler(&buf, opts)
	compHandler := &componentHandler{Handler: handler, component: "test"}

	logger := &Logger{
		Logger: slog.New(compHandler),
		config: cfg,
	}

	logger.Info("test message", "key", "value")

	// Parse JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, buf.String())
	}

	// Verify JSON fields
	if logEntry["msg"] != "test message" {
		t.Errorf("Expected msg='test message', got '%v'", logEntry["msg"])
	}
	if logEntry["level"] != "INFO" {
		t.Errorf("Expected level='INFO', got '%v'", logEntry["level"])
	}
	if logEntry["component"] != "test" {
		t.Errorf("Expected component='test', got '%v'", logEntry["component"])
	}
	if logEntry["key"] != "value" {
		t.Errorf("Expected key='value', got '%v'", logEntry["key"])
	}
}

func TestFileOutput(t *testing.T) {
	// Create temporary directory for test logs
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	cfg := &Config{
		Level:     LevelInfo,
		Format:    "text",
		Output:    logFile,
		Component: "test",
	}

	logger, err := Init(cfg)
	if err != nil {
		t.Fatalf("Failed to init logger: %v", err)
	}
	defer logger.Close()

	// Write a log message
	logger.Info("test message to file")

	// Close to ensure flush
	if err := logger.Close(); err != nil {
		t.Fatalf("Failed to close logger: %v", err)
	}

	// Read the log file
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Verify content
	if !strings.Contains(string(content), "test message to file") {
		t.Errorf("Log file doesn't contain expected message. Content: %s", content)
	}
}

func TestWithContext(t *testing.T) {
	var buf bytes.Buffer

	cfg := &Config{
		Level:     LevelInfo,
		Format:    "json",
		Output:    "stdout",
		Component: "test",
	}

	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	handler := slog.NewJSONHandler(&buf, opts)

	logger := &Logger{
		Logger: slog.New(handler),
		config: cfg,
	}

	// Test WithComponent
	compLogger := logger.WithComponent("subcomponent")
	compLogger.Info("component test")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if logEntry["component"] != "subcomponent" {
		t.Errorf("Expected component='subcomponent', got '%v'", logEntry["component"])
	}

	// Test WithOperation
	buf.Reset()
	opLogger := logger.WithOperation("test_op")
	opLogger.Info("operation test")

	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if logEntry["operation"] != "test_op" {
		t.Errorf("Expected operation='test_op', got '%v'", logEntry["operation"])
	}

	// Test WithBlockHeight
	buf.Reset()
	heightLogger := logger.WithBlockHeight(12345)
	heightLogger.Info("block test")

	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if logEntry["block_height"] != float64(12345) {
		t.Errorf("Expected block_height=12345, got '%v'", logEntry["block_height"])
	}

	// Test WithPeerID
	buf.Reset()
	peerLogger := logger.WithPeerID("peer123")
	peerLogger.Info("peer test")

	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if logEntry["peer_id"] != "peer123" {
		t.Errorf("Expected peer_id='peer123', got '%v'", logEntry["peer_id"])
	}

	// Test WithTxHash
	buf.Reset()
	txLogger := logger.WithTxHash("abc123")
	txLogger.Info("tx test")

	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if logEntry["tx_hash"] != "abc123" {
		t.Errorf("Expected tx_hash='abc123', got '%v'", logEntry["tx_hash"])
	}
}

func TestGetDefault(t *testing.T) {
	// This test verifies GetDefault behavior but doesn't modify global state
	// We just verify the function works correctly
	logger := GetDefault()
	if logger == nil {
		t.Fatal("GetDefault returned nil")
	}

	// Verify it returns the same instance on subsequent calls
	logger2 := GetDefault()
	if logger != logger2 {
		t.Error("GetDefault should return the same instance")
	}

	// Verify the logger is functional
	logger.Info("test message")
}

func TestSetDefault(t *testing.T) {
	cfg := &Config{
		Level:     LevelDebug,
		Format:    "json",
		Output:    "stdout",
		Component: "custom",
	}

	logger, err := Init(cfg)
	if err != nil {
		t.Fatalf("Failed to init logger: %v", err)
	}

	SetDefault(logger)

	retrieved := GetDefault()
	if retrieved != logger {
		t.Error("SetDefault didn't set the correct logger")
	}
}

func TestHelperFunctions(t *testing.T) {
	var buf bytes.Buffer

	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	handler := slog.NewJSONHandler(&buf, opts)

	logger := &Logger{
		Logger: slog.New(handler),
		config: DefaultConfig(),
	}

	// Test LogBlockProcessed
	LogBlockProcessed(logger, 100, "abc123", 50*time.Millisecond)
	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if entry["msg"] != "block processed" {
		t.Errorf("Expected msg='block processed', got '%v'", entry["msg"])
	}
	if entry["block_height"] != float64(100) {
		t.Errorf("Expected block_height=100, got '%v'", entry["block_height"])
	}

	// Test LogNameOperation
	buf.Reset()
	LogNameOperation(logger, "NAME_UPDATE", "d/example", "txhash123")
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if entry["msg"] != "name operation" {
		t.Errorf("Expected msg='name operation', got '%v'", entry["msg"])
	}

	// Test LogPeerConnected
	buf.Reset()
	LogPeerConnected(logger, "peer1", "192.168.1.1:8334")
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if entry["msg"] != "peer connected" {
		t.Errorf("Expected msg='peer connected', got '%v'", entry["msg"])
	}

	// Test LogRPCRequest
	buf.Reset()
	LogRPCRequest(logger, "getinfo", "127.0.0.1")
	// RPC request is DEBUG level, so it won't appear with INFO level logger
	if buf.Len() > 0 {
		t.Error("LogRPCRequest should not log at INFO level")
	}

	// Test LogRPCResponse with error
	buf.Reset()
	testErr := &testError{msg: "test error"}
	LogRPCResponse(logger, "getinfo", 10*time.Millisecond, testErr)
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if entry["level"] != "WARN" {
		t.Errorf("Expected level='WARN' for error response, got '%v'", entry["level"])
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestComponentHandler(t *testing.T) {
	var buf bytes.Buffer

	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	baseHandler := slog.NewJSONHandler(&buf, opts)
	handler := &componentHandler{
		Handler:   baseHandler,
		component: "test-component",
	}

	logger := slog.New(handler)
	logger.Info("test message", "key", "value")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if entry["component"] != "test-component" {
		t.Errorf("Expected component='test-component', got '%v'", entry["component"])
	}
	if entry["msg"] != "test message" {
		t.Errorf("Expected msg='test message', got '%v'", entry["msg"])
	}
	if entry["key"] != "value" {
		t.Errorf("Expected key='value', got '%v'", entry["key"])
	}
}

func TestLogFileCreation(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Use a nested path to test directory creation
	logFile := filepath.Join(tmpDir, "logs", "subdir", "test.log")

	cfg := &Config{
		Level:     LevelInfo,
		Format:    "text",
		Output:    logFile,
		Component: "test",
	}

	logger, err := Init(cfg)
	if err != nil {
		t.Fatalf("Failed to init logger: %v", err)
	}
	defer logger.Close()

	// Verify the log file was created
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Errorf("Log file was not created: %s", logFile)
	}

	// Verify the directory structure was created
	logDir := filepath.Dir(logFile)
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		t.Errorf("Log directory was not created: %s", logDir)
	}
}
