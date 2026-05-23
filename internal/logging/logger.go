package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel string

const (
	LevelDebug LogLevel = "DEBUG"
	LevelInfo  LogLevel = "INFO"
	LevelWarn  LogLevel = "WARN"
	LevelError LogLevel = "ERROR"
)

// Config holds logging configuration
type Config struct {
	// Level is the minimum log level to output (DEBUG, INFO, WARN, ERROR)
	Level LogLevel

	// Format determines the output format ("json" or "text")
	Format string

	// Output specifies where logs should be written ("stdout", "stderr", or a file path)
	Output string

	// Component is the name of the component (used as a field in all logs)
	Component string

	// EnableRotation enables log file rotation (only applies when Output is a file)
	EnableRotation bool

	// MaxSizeMB is the maximum size in megabytes before rotation (default: 100)
	MaxSizeMB int

	// MaxBackups is the maximum number of old log files to retain (default: 10)
	MaxBackups int

	// MaxAgeDays is the maximum number of days to retain old log files (default: 30)
	MaxAgeDays int

	// CompressBackups determines if rotated files should be compressed (default: true)
	CompressBackups bool
}

// DefaultConfig returns default logging configuration
func DefaultConfig() *Config {
	return &Config{
		Level:           LevelInfo,
		Format:          "text",
		Output:          "stdout",
		Component:       "nmcd",
		EnableRotation:  false,
		MaxSizeMB:       100,
		MaxBackups:      10,
		MaxAgeDays:      30,
		CompressBackups: true,
	}
}

// Logger wraps slog.Logger with additional context
type Logger struct {
	*slog.Logger
	config *Config
	mu     sync.RWMutex
	closer io.Closer // for log rotation
}

var (
	defaultLogger *Logger
	defaultLoggerMu sync.RWMutex
	once          sync.Once
)

// Init initializes the global logger with the given configuration
func Init(cfg *Config) (*Logger, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Determine the log level
	var level slog.Level
	switch cfg.Level {
	case LevelDebug:
		level = slog.LevelDebug
	case LevelInfo:
		level = slog.LevelInfo
	case LevelWarn:
		level = slog.LevelWarn
	case LevelError:
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// Determine the output writer
	var writer io.Writer
	var closer io.Closer

	switch cfg.Output {
	case "stdout":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	default:
		// File output
		// Note: Log rotation can be enabled in a future enhancement using external tools
		// or a third-party library. For now, we use simple file appending.

		// Create log directory if it doesn't exist
		logDir := filepath.Dir(cfg.Output)
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return nil, err
		}

		// Open log file
		f, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		writer = f
		closer = f
	}

	// Create the handler based on format
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
	}

	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	// Wrap handler to add component field to all logs
	handler = &componentHandler{
		Handler:   handler,
		component: cfg.Component,
	}

	logger := &Logger{
		Logger: slog.New(handler),
		config: cfg,
		closer: closer,
	}

	return logger, nil
}

// GetDefault returns the default global logger
// If not initialized, it creates one with default config
func GetDefault() *Logger {
	once.Do(func() {
		var err error
		defaultLogger, err = Init(DefaultConfig())
		if err != nil {
			// Fallback to stdout if initialization fails
			defaultLogger = &Logger{
				Logger: slog.Default(),
				config: DefaultConfig(),
			}
		}
	})
	defaultLoggerMu.RLock()
	defer defaultLoggerMu.RUnlock()
	return defaultLogger
}

// SetDefault sets the default global logger
func SetDefault(logger *Logger) {
	defaultLoggerMu.Lock()
	defer defaultLoggerMu.Unlock()
	defaultLogger = logger
}

// Close closes the logger and any underlying resources (like log files)
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closer != nil {
		return l.closer.Close()
	}
	return nil
}

// WithComponent creates a new logger with an additional component context
// Child loggers do not manage the closer resource - only the root logger should close files
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger: l.Logger.With("component", component),
		config: l.config,
		closer: nil, // Child loggers don't own the closer
	}
}

// WithOperation creates a child logger with operation context
// Child loggers do not manage the closer resource - only the root logger should close files
func (l *Logger) WithOperation(operation string) *Logger {
	return &Logger{
		Logger: l.Logger.With("operation", operation),
		config: l.config,
		closer: nil, // Child loggers don't own the closer
	}
}

// WithBlockHeight creates a child logger with block height context
// Child loggers do not manage the closer resource - only the root logger should close files
func (l *Logger) WithBlockHeight(height int32) *Logger {
	return &Logger{
		Logger: l.Logger.With("block_height", height),
		config: l.config,
		closer: nil, // Child loggers don't own the closer
	}
}

// WithPeerID creates a child logger with peer ID context
// Child loggers do not manage the closer resource - only the root logger should close files
func (l *Logger) WithPeerID(peerID string) *Logger {
	return &Logger{
		Logger: l.Logger.With("peer_id", peerID),
		config: l.config,
		closer: nil, // Child loggers don't own the closer
	}
}

// WithTxHash creates a child logger with transaction hash context
// Child loggers do not manage the closer resource - only the root logger should close files
func (l *Logger) WithTxHash(txHash string) *Logger {
	return &Logger{
		Logger: l.Logger.With("tx_hash", txHash),
		config: l.config,
		closer: nil, // Child loggers don't own the closer
	}
}

// WithError creates a child logger with error context
// Child loggers do not manage the closer resource - only the root logger should close files
func (l *Logger) WithError(err error) *Logger {
	return &Logger{
		Logger: l.Logger.With("error", err),
		config: l.config,
		closer: nil, // Child loggers don't own the closer
	}
}

// componentHandler wraps an slog.Handler to add a component field to all log records
type componentHandler struct {
	slog.Handler
	component string
}

func (h *componentHandler) Handle(ctx context.Context, r slog.Record) error {
	// Create a copy of the record to avoid modifying the original
	rec := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)

	// Add component field
	rec.AddAttrs(slog.String("component", h.component))

	// Add original attributes
	r.Attrs(func(a slog.Attr) bool {
		rec.AddAttrs(a)
		return true
	})

	return h.Handler.Handle(ctx, rec)
}

func (h *componentHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &componentHandler{
		Handler:   h.Handler.WithAttrs(attrs),
		component: h.component,
	}
}

func (h *componentHandler) WithGroup(name string) slog.Handler {
	return &componentHandler{
		Handler:   h.Handler.WithGroup(name),
		component: h.component,
	}
}

// Helper functions for common log patterns

// LogBlockProcessed logs a block processing event with relevant context
func LogBlockProcessed(logger *Logger, height int32, hash string, duration time.Duration) {
	logger.Info("block processed",
		"block_height", height,
		"block_hash", hash,
		"duration_ms", duration.Milliseconds(),
	)
}

// LogNameOperation logs a name operation event
func LogNameOperation(logger *Logger, operation, name, txHash string) {
	logger.Info("name operation",
		"operation", operation,
		"name", name,
		"tx_hash", txHash,
	)
}

// LogPeerConnected logs a peer connection event
func LogPeerConnected(logger *Logger, peerID, addr string) {
	logger.Info("peer connected",
		"peer_id", peerID,
		"address", addr,
	)
}

// LogPeerDisconnected logs a peer disconnection event
func LogPeerDisconnected(logger *Logger, peerID, reason string) {
	logger.Info("peer disconnected",
		"peer_id", peerID,
		"reason", reason,
	)
}

// LogRPCRequest logs an RPC request
func LogRPCRequest(logger *Logger, method, clientIP string) {
	logger.Debug("rpc request",
		"method", method,
		"client_ip", clientIP,
	)
}

// LogRPCResponse logs an RPC response
func LogRPCResponse(logger *Logger, method string, duration time.Duration, err error) {
	if err != nil {
		logger.Warn("rpc error",
			"method", method,
			"duration_ms", duration.Milliseconds(),
			"error", err,
		)
	} else {
		logger.Debug("rpc response",
			"method", method,
			"duration_ms", duration.Milliseconds(),
		)
	}
}
