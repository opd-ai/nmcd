package mail

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

// RelayConfig contains configuration for the SMTP relay server.
type RelayConfig struct {
	// ListenAddr is the address to listen on (e.g., ":2525" or "localhost:2525")
	ListenAddr string

	// UpstreamHost is the SMTP server to forward mail to (e.g., "smtp.gmail.com")
	UpstreamHost string

	// UpstreamPort is the port of the upstream SMTP server (typically 587 for TLS)
	UpstreamPort int

	// UpstreamAuth contains SMTP authentication credentials for the upstream server
	// If nil, no authentication is performed
	UpstreamAuth smtp.Auth

	// ReadTimeout is the maximum duration for reading from client connections
	ReadTimeout time.Duration

	// WriteTimeout is the maximum duration for writing to client connections
	WriteTimeout time.Duration

	// MaxMessageSize is the maximum size in bytes for incoming messages
	MaxMessageSize int64
}

// DefaultRelayConfig returns a RelayConfig with sensible defaults.
func DefaultRelayConfig() RelayConfig {
	return RelayConfig{
		ListenAddr:     ":2525",
		UpstreamPort:   587,
		ReadTimeout:    5 * time.Minute,
		WriteTimeout:   5 * time.Minute,
		MaxMessageSize: 10 * 1024 * 1024, // 10 MB
	}
}

// Relay is a minimal SMTP relay server that accepts mail to .bit addresses
// and forwards it to real email addresses via an upstream SMTP server.
//
// The relay uses the Router to resolve .bit addresses to real email addresses,
// then forwards the message to the upstream SMTP server for delivery.
//
// Thread-safety: Relay is safe for concurrent use by multiple goroutines.
type Relay struct {
	router   *Router
	config   RelayConfig
	listener net.Listener
	wg       sync.WaitGroup
	mu       sync.Mutex
	started  bool
	logger   *log.Logger
}

// NewRelay creates a new SMTP relay with the given router and configuration.
//
// Parameters:
//   - router: Mail router for resolving .bit addresses
//   - config: Relay configuration (use DefaultRelayConfig() for defaults)
//
// Returns:
//   - *Relay: Initialized relay ready to start accepting connections
//
// Example:
//
//	config := DefaultRelayConfig()
//	config.UpstreamHost = "smtp.gmail.com"
//	config.UpstreamAuth = smtp.PlainAuth("", "user@gmail.com", "password", "smtp.gmail.com")
//
//	relay := NewRelay(router, config)
//	if err := relay.Start(); err != nil {
//	    log.Fatal(err)
//	}
//	defer relay.Stop()
func NewRelay(router *Router, config RelayConfig) *Relay {
	return &Relay{
		router: router,
		config: config,
		logger: log.New(log.Writer(), "[smtp-relay] ", log.LstdFlags),
	}
}

// Start begins accepting SMTP connections on the configured listen address.
// This method returns immediately after starting the listener; connections
// are handled in background goroutines.
//
// Returns an error if the relay is already started or if binding to the
// listen address fails.
func (r *Relay) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return errors.New("relay already started")
	}

	// Create TCP listener
	listener, err := net.Listen("tcp", r.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", r.config.ListenAddr, err)
	}

	r.listener = listener
	r.started = true

	r.logger.Printf("SMTP relay started on %s", r.config.ListenAddr)

	// Start accepting connections in background
	r.wg.Add(1)
	go r.acceptLoop()

	return nil
}

// Stop gracefully shuts down the relay server.
// It stops accepting new connections and waits for existing connections to complete.
//
// This method blocks until all connection handlers have finished.
func (r *Relay) Stop() error {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return errors.New("relay not started")
	}
	r.started = false
	r.mu.Unlock()

	r.logger.Println("Stopping SMTP relay...")

	// Close listener to stop accepting new connections
	if err := r.listener.Close(); err != nil {
		return fmt.Errorf("failed to close listener: %w", err)
	}

	// Wait for all handlers to finish
	r.wg.Wait()

	r.logger.Println("SMTP relay stopped")
	return nil
}

// acceptLoop continuously accepts incoming connections until the listener is closed.
func (r *Relay) acceptLoop() {
	defer r.wg.Done()

	for {
		conn, err := r.listener.Accept()
		if err != nil {
			// Check if listener was closed (expected during shutdown)
			if errors.Is(err, net.ErrClosed) {
				return
			}
			r.logger.Printf("Accept error: %v", err)
			continue
		}

		// Handle connection in background
		r.wg.Add(1)
		go r.handleConnection(conn)
	}
}

// handleConnection processes a single SMTP connection.
func (r *Relay) handleConnection(conn net.Conn) {
	defer r.wg.Done()
	defer conn.Close()

	// Set connection timeouts
	if r.config.ReadTimeout > 0 {
		conn.SetReadDeadline(time.Now().Add(r.config.ReadTimeout))
	}
	if r.config.WriteTimeout > 0 {
		conn.SetWriteDeadline(time.Now().Add(r.config.WriteTimeout))
	}

	r.logger.Printf("New connection from %s", conn.RemoteAddr())

	// Implement minimal SMTP protocol
	session := &smtpSession{
		conn:   conn,
		relay:  r,
		logger: r.logger,
		reader: bufio.NewReader(conn),
	}

	if err := session.handle(); err != nil {
		r.logger.Printf("Session error from %s: %v", conn.RemoteAddr(), err)
	}
}

// smtpSession represents a single SMTP session.
type smtpSession struct {
	conn   net.Conn
	relay  *Relay
	logger *log.Logger
	from   string
	to     []string
	reader *bufio.Reader
}

// handle processes the SMTP protocol for this session.
func (s *smtpSession) handle() error {
	if err := s.sendGreeting(); err != nil {
		return err
	}
	return s.processCommands()
}

// sendGreeting sends the initial SMTP greeting to the client.
func (s *smtpSession) sendGreeting() error {
	return s.writeLine("220 nmcd SMTP Relay Service Ready")
}

// processCommands reads and dispatches SMTP commands until the session ends.
func (s *smtpSession) processCommands() error {
	for {
		line, err := s.readLine()
		if err != nil {
			return err
		}

		cmd := strings.ToUpper(strings.TrimSpace(line))
		if err := s.dispatchCommand(cmd); err != nil {
			return err
		}

		if s.shouldQuit(cmd) {
			return nil
		}
	}
}

// shouldQuit checks if the session should terminate after processing the command.
func (s *smtpSession) shouldQuit(cmd string) bool {
	return cmd == "QUIT"
}

// dispatchCommand routes a command to its handler.
func (s *smtpSession) dispatchCommand(cmd string) error {
	switch {
	case strings.HasPrefix(cmd, "HELO "), strings.HasPrefix(cmd, "EHLO "):
		return s.handleHelo(cmd)
	case strings.HasPrefix(cmd, "MAIL FROM:"):
		return s.handleMailFrom(cmd)
	case strings.HasPrefix(cmd, "RCPT TO:"):
		return s.handleRcptTo(cmd)
	case cmd == "DATA":
		return s.handleData()
	case cmd == "QUIT":
		return s.handleQuit()
	case cmd == "RSET":
		return s.handleReset()
	case cmd == "NOOP":
		return s.handleNoop()
	default:
		return s.handleUnknown()
	}
}

// handleQuit handles the QUIT command.
func (s *smtpSession) handleQuit() error {
	return s.writeLine("221 Goodbye")
}

// handleReset handles the RSET command.
func (s *smtpSession) handleReset() error {
	s.from = ""
	s.to = nil
	return s.writeLine("250 OK")
}

// handleNoop handles the NOOP command.
func (s *smtpSession) handleNoop() error {
	return s.writeLine("250 OK")
}

// handleUnknown handles unrecognized commands.
func (s *smtpSession) handleUnknown() error {
	return s.writeLine("502 Command not implemented")
}

// handleHelo handles HELO/EHLO commands.
func (s *smtpSession) handleHelo(cmd string) error {
	if strings.HasPrefix(cmd, "EHLO ") {
		// For EHLO, we can advertise extensions
		if err := s.writeLine("250-nmcd.local"); err != nil {
			return err
		}
		if err := s.writeLine(fmt.Sprintf("250 SIZE %d", s.relay.config.MaxMessageSize)); err != nil {
			return err
		}
	} else {
		// Simple HELO response
		if err := s.writeLine("250 nmcd.local"); err != nil {
			return err
		}
	}
	return nil
}

// handleMailFrom handles MAIL FROM command.
func (s *smtpSession) handleMailFrom(cmd string) error {
	// Extract email address from "MAIL FROM:<address>"
	addr := extractAddress(cmd[10:]) // Skip "MAIL FROM:"
	if addr == "" {
		return s.writeLine("501 Syntax error in MAIL FROM")
	}

	// Store the address in lowercase for consistency
	s.from = strings.ToLower(addr)
	return s.writeLine("250 OK")
}

// handleRcptTo handles RCPT TO command.
func (s *smtpSession) handleRcptTo(cmd string) error {
	// Extract email address from "RCPT TO:<address>"
	addr := extractAddress(cmd[8:]) // Skip "RCPT TO:"
	if addr == "" {
		return s.writeLine("501 Syntax error in RCPT TO")
	}

	// Validate it's a .bit address by checking the domain part
	// Email format is localpart@domain, so we split and check domain
	atIndex := strings.LastIndex(addr, "@")
	if atIndex == -1 {
		return s.writeLine("501 Invalid email address format")
	}
	domain := strings.ToLower(addr[atIndex+1:])
	if !strings.HasSuffix(domain, ".bit") {
		return s.writeLine("550 Only .bit addresses accepted")
	}

	// Store the address in lowercase for consistency
	s.to = append(s.to, strings.ToLower(addr))
	return s.writeLine("250 OK")
}

// handleData handles DATA command and message body.
func (s *smtpSession) handleData() error {
	if s.from == "" || len(s.to) == 0 {
		return s.writeLine("503 Bad sequence of commands")
	}

	// Send intermediate response
	if err := s.writeLine("354 Start mail input; end with <CRLF>.<CRLF>"); err != nil {
		return err
	}

	// Read message body until "." on a line by itself
	body, err := s.readDataBody()
	if err != nil {
		return err
	}

	// Check message size
	if int64(len(body)) > s.relay.config.MaxMessageSize {
		return s.writeLine("552 Message exceeds maximum size")
	}

	// Forward to each recipient
	ctx := context.Background()
	var successCount, failCount int
	var lastErr error

	for _, to := range s.to {
		if err := s.forwardMessage(ctx, s.from, to, body); err != nil {
			s.logger.Printf("Failed to forward to %s: %v", to, err)
			failCount++
			lastErr = err
		} else {
			successCount++
		}
	}

	// Reset for next message
	s.from = ""
	s.to = nil

	// Return appropriate status based on results
	if failCount == 0 {
		return s.writeLine("250 OK: Message accepted for delivery")
	} else if successCount == 0 {
		// All recipients failed
		return s.writeLine(fmt.Sprintf("554 Transaction failed: %v", lastErr))
	} else {
		// Partial success
		return s.writeLine(fmt.Sprintf("250 OK: Delivered to %d of %d recipients", successCount, successCount+failCount))
	}
}

// forwardMessage resolves the .bit address and forwards the message to the upstream SMTP server.
func (s *smtpSession) forwardMessage(ctx context.Context, from, to string, body []byte) error {
	realAddr, err := s.relay.router.Route(ctx, to)
	if err != nil {
		return fmt.Errorf("routing failed: %w", err)
	}

	s.logger.Printf("Forwarding message from %s to %s (resolved from %s)", from, realAddr, to)

	client, err := s.connectUpstream()
	if err != nil {
		return err
	}
	defer client.Close()

	if err := s.sendEnvelope(client, from, realAddr); err != nil {
		return err
	}

	return s.sendBody(client, body)
}

// connectUpstream connects to the upstream SMTP server, optionally upgrading to TLS and authenticating.
func (s *smtpSession) connectUpstream() (*smtp.Client, error) {
	addr := fmt.Sprintf("%s:%d", s.relay.config.UpstreamHost, s.relay.config.UpstreamPort)
	client, err := smtp.Dial(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to upstream SMTP: %w", err)
	}

	if s.relay.config.UpstreamPort == 587 {
		tlsConfig := &tls.Config{
			ServerName: s.relay.config.UpstreamHost,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			client.Close()
			return nil, fmt.Errorf("STARTTLS failed: %w", err)
		}
	}

	if s.relay.config.UpstreamAuth != nil {
		if err := client.Auth(s.relay.config.UpstreamAuth); err != nil {
			client.Close()
			return nil, fmt.Errorf("upstream authentication failed: %w", err)
		}
	}

	return client, nil
}

// sendEnvelope sends the SMTP envelope (MAIL FROM and RCPT TO).
func (s *smtpSession) sendEnvelope(client *smtp.Client, from, to string) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM failed: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT TO failed: %w", err)
	}
	return nil
}

// sendBody sends the message body and issues QUIT.
func (s *smtpSession) sendBody(client *smtp.Client, body []byte) error {
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA command failed: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write message failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close message failed: %w", err)
	}
	return client.Quit()
}

// readLine reads a single line from the connection.
func (s *smtpSession) readLine() (string, error) {
	line, err := s.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	// Trim CRLF or just LF
	line = strings.TrimSuffix(line, "\r\n")
	line = strings.TrimSuffix(line, "\n")

	// Prevent excessive line length
	if len(line) > 1024 {
		return "", errors.New("line too long")
	}

	return line, nil
}

// writeLine writes a line to the connection with CRLF terminator.
func (s *smtpSession) writeLine(line string) error {
	_, err := s.conn.Write([]byte(line + "\r\n"))
	return err
}

// readDataBody reads the message body until end-of-data marker.
func (s *smtpSession) readDataBody() ([]byte, error) {
	var body []byte

	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		// Check for end-of-data marker (single "." on a line)
		trimmed := strings.TrimSuffix(line, "\r\n")
		trimmed = strings.TrimSuffix(trimmed, "\n")
		if trimmed == "." {
			return body, nil
		}

		// Add line to body (keep CRLF for proper email format)
		body = append(body, []byte(line)...)
	}

	return body, nil
}

// extractAddress extracts an email address from a parameter string.
// Handles both <address> and plain address formats.
func extractAddress(param string) string {
	param = strings.TrimSpace(param)

	// Handle <address> format
	if strings.HasPrefix(param, "<") && strings.HasSuffix(param, ">") {
		return param[1 : len(param)-1]
	}

	// Handle plain address
	return param
}
