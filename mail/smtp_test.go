package mail

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/opd-ai/nmcd/bridge"
)

// mockResolver implements Resolver for testing
type mockSMTPResolver struct {
	lookups map[string]bridge.MailConfig
	err     error
}

func (m *mockSMTPResolver) LookupMail(ctx context.Context, name string) (bridge.MailConfig, error) {
	if m.err != nil {
		return bridge.MailConfig{}, m.err
	}
	if config, ok := m.lookups[name]; ok {
		return config, nil
	}
	return bridge.MailConfig{}, fmt.Errorf("name not found: %s", name)
}

// setupTestRelay creates and starts a test relay with default configuration
func setupTestRelay(t *testing.T, lookups map[string]bridge.MailConfig) (*Relay, string) {
	t.Helper()
	resolver := &mockSMTPResolver{lookups: lookups}
	router := NewRouter(resolver, 0)
	config := DefaultRelayConfig()
	config.ListenAddr = "localhost:0"

	relay := NewRelay(router, config)
	if err := relay.Start(); err != nil {
		t.Fatalf("Failed to start relay: %v", err)
	}
	return relay, relay.listener.Addr().String()
}

// setupTestConnection creates a connection to the test relay and returns conn and reader
func setupTestConnection(t *testing.T, addr string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	reader := bufio.NewReader(conn)
	// Read greeting
	reader.ReadString('\n')
	return conn, reader
}

// sendEHLO sends EHLO command and skips the multi-line response
func sendEHLO(t *testing.T, conn net.Conn, reader *bufio.Reader) {
	t.Helper()
	fmt.Fprintf(conn, "EHLO client.example.com\r\n")
	for {
		line, _ := reader.ReadString('\n')
		if !strings.Contains(line, "250-") {
			break
		}
	}
}

// TestNewRelay tests relay creation.
func TestNewRelay(t *testing.T) {
	resolver := &mockSMTPResolver{
		lookups: map[string]bridge.MailConfig{
			"alice": {ForwardTo: "alice@gmail.com"},
		},
	}
	router := NewRouter(resolver, time.Hour)
	config := DefaultRelayConfig()

	relay := NewRelay(router, config)

	if relay == nil {
		t.Fatal("NewRelay returned nil")
	}
	if relay.router != router {
		t.Error("Router not set correctly")
	}
	if relay.config.ListenAddr != config.ListenAddr {
		t.Error("Config not set correctly")
	}
}

// TestDefaultRelayConfig tests default configuration.
func TestDefaultRelayConfig(t *testing.T) {
	config := DefaultRelayConfig()

	if config.ListenAddr != ":2525" {
		t.Errorf("Expected ListenAddr :2525, got %s", config.ListenAddr)
	}
	if config.UpstreamPort != 587 {
		t.Errorf("Expected UpstreamPort 587, got %d", config.UpstreamPort)
	}
	if config.ReadTimeout != 5*time.Minute {
		t.Errorf("Expected ReadTimeout 5m, got %v", config.ReadTimeout)
	}
	if config.WriteTimeout != 5*time.Minute {
		t.Errorf("Expected WriteTimeout 5m, got %v", config.WriteTimeout)
	}
	if config.MaxMessageSize != 10*1024*1024 {
		t.Errorf("Expected MaxMessageSize 10MB, got %d", config.MaxMessageSize)
	}
}

// TestRelayStartStop tests starting and stopping the relay.
func TestRelayStartStop(t *testing.T) {
	resolver := &mockSMTPResolver{
		lookups: map[string]bridge.MailConfig{
			"alice": {ForwardTo: "alice@gmail.com"},
		},
	}
	router := NewRouter(resolver, time.Hour)
	config := DefaultRelayConfig()
	config.ListenAddr = "localhost:0" // Use random port

	relay := NewRelay(router, config)

	// Start relay
	if err := relay.Start(); err != nil {
		t.Fatalf("Failed to start relay: %v", err)
	}

	// Verify listener is active
	if relay.listener == nil {
		t.Error("Listener not set after Start()")
	}

	// Try to start again (should fail)
	if err := relay.Start(); err == nil {
		t.Error("Expected error when starting already-started relay")
	}

	// Stop relay
	if err := relay.Stop(); err != nil {
		t.Fatalf("Failed to stop relay: %v", err)
	}

	// Try to stop again (should fail)
	if err := relay.Stop(); err == nil {
		t.Error("Expected error when stopping already-stopped relay")
	}
}

// TestRelayStartBindError tests error handling when bind fails.
func TestRelayStartBindError(t *testing.T) {
	resolver := &mockSMTPResolver{
		lookups: map[string]bridge.MailConfig{
			"alice": {ForwardTo: "alice@gmail.com"},
		},
	}
	router := NewRouter(resolver, time.Hour)
	config := DefaultRelayConfig()
	config.ListenAddr = "invalid:address:format"

	relay := NewRelay(router, config)

	// Start should fail with invalid address
	if err := relay.Start(); err == nil {
		t.Error("Expected error when starting with invalid listen address")
		relay.Stop()
	}
}

// TestSMTPProtocol tests basic SMTP protocol handling.
func TestSMTPProtocol(t *testing.T) {
	lookups := map[string]bridge.MailConfig{
		"alice": {ForwardTo: "alice@gmail.com"},
	}
	relay, addr := setupTestRelay(t, lookups)
	defer relay.Stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Read greeting
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read greeting: %v", err)
	}
	if !strings.HasPrefix(line, "220 ") {
		t.Errorf("Expected 220 greeting, got: %s", line)
	}

	// Send EHLO
	fmt.Fprintf(conn, "EHLO client.example.com\r\n")
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read EHLO response: %v", err)
	}
	if !strings.HasPrefix(line, "250") {
		t.Errorf("Expected 250 response to EHLO, got: %s", line)
	}

	// Read additional EHLO responses (SIZE extension)
	for {
		line, err = reader.ReadString('\n')
		if err != nil {
			break
		}
		if !strings.HasPrefix(line, "250") {
			break
		}
		// Continue reading 250- responses
		if !strings.Contains(line, "250-") {
			break
		}
	}

	// Send MAIL FROM
	fmt.Fprintf(conn, "MAIL FROM:<sender@example.com>\r\n")
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read MAIL FROM response: %v", err)
	}
	if !strings.HasPrefix(line, "250 ") {
		t.Errorf("Expected 250 response to MAIL FROM, got: %s", line)
	}

	// Send RCPT TO with .bit address
	fmt.Fprintf(conn, "RCPT TO:<alice@mail.bit>\r\n")
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read RCPT TO response: %v", err)
	}
	if !strings.HasPrefix(line, "250 ") {
		t.Errorf("Expected 250 response to RCPT TO, got: %s", line)
	}

	// Send QUIT
	fmt.Fprintf(conn, "QUIT\r\n")
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read QUIT response: %v", err)
	}
	if !strings.HasPrefix(line, "221 ") {
		t.Errorf("Expected 221 response to QUIT, got: %s", line)
	}
}

// TestSMTPNonBitAddress tests rejection of non-.bit addresses.
func TestSMTPNonBitAddress(t *testing.T) {
	lookups := map[string]bridge.MailConfig{
		"alice": {ForwardTo: "alice@gmail.com"},
	}
	relay, addr := setupTestRelay(t, lookups)
	defer relay.Stop()

	conn, reader := setupTestConnection(t, addr)
	defer conn.Close()

	sendEHLO(t, conn, reader)

	// Send MAIL FROM
	fmt.Fprintf(conn, "MAIL FROM:<sender@example.com>\r\n")
	reader.ReadString('\n')

	// Send RCPT TO with non-.bit address (should be rejected)
	fmt.Fprintf(conn, "RCPT TO:<user@gmail.com>\r\n")
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read RCPT TO response: %v", err)
	}
	if !strings.HasPrefix(line, "550 ") {
		t.Errorf("Expected 550 rejection for non-.bit address, got: %s", line)
	}

	// Send QUIT
	fmt.Fprintf(conn, "QUIT\r\n")
	reader.ReadString('\n')
}

// TestSMTPRSET tests RSET command.
func TestSMTPRSET(t *testing.T) {
	lookups := map[string]bridge.MailConfig{
		"alice": {ForwardTo: "alice@gmail.com"},
	}
	relay, addr := setupTestRelay(t, lookups)
	defer relay.Stop()

	conn, reader := setupTestConnection(t, addr)
	defer conn.Close()

	sendEHLO(t, conn, reader)

	// Send MAIL FROM
	fmt.Fprintf(conn, "MAIL FROM:<sender@example.com>\r\n")
	reader.ReadString('\n')

	// Send RSET
	fmt.Fprintf(conn, "RSET\r\n")
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read RSET response: %v", err)
	}
	if !strings.HasPrefix(line, "250 ") {
		t.Errorf("Expected 250 response to RSET, got: %s", line)
	}

	// Send QUIT
	fmt.Fprintf(conn, "QUIT\r\n")
	reader.ReadString('\n')
}

// TestSMTPNOOP tests NOOP command.
func TestSMTPNOOP(t *testing.T) {
	lookups := map[string]bridge.MailConfig{
		"alice": {ForwardTo: "alice@gmail.com"},
	}
	relay, addr := setupTestRelay(t, lookups)
	defer relay.Stop()

	conn, reader := setupTestConnection(t, addr)
	defer conn.Close()

	// Send NOOP
	fmt.Fprintf(conn, "NOOP\r\n")
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read NOOP response: %v", err)
	}
	if !strings.HasPrefix(line, "250 ") {
		t.Errorf("Expected 250 response to NOOP, got: %s", line)
	}

	// Send QUIT
	fmt.Fprintf(conn, "QUIT\r\n")
	reader.ReadString('\n')
}

// TestExtractAddress tests email address extraction.
func TestExtractAddress(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "angle brackets",
			input:    "<user@example.com>",
			expected: "user@example.com",
		},
		{
			name:     "plain address",
			input:    "user@example.com",
			expected: "user@example.com",
		},
		{
			name:     "with spaces",
			input:    "  <user@example.com>  ",
			expected: "user@example.com",
		},
		{
			name:     "plain with spaces",
			input:    "  user@example.com  ",
			expected: "user@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAddress(tt.input)
			if result != tt.expected {
				t.Errorf("extractAddress(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestSMTPConcurrentConnections tests handling multiple concurrent connections.
func TestSMTPConcurrentConnections(t *testing.T) {
	resolver := &mockSMTPResolver{
		lookups: map[string]bridge.MailConfig{
			"alice": {ForwardTo: "alice@gmail.com"},
		},
	}
	router := NewRouter(resolver, 0)
	config := DefaultRelayConfig()
	config.ListenAddr = "localhost:0"

	relay := NewRelay(router, config)
	if err := relay.Start(); err != nil {
		t.Fatalf("Failed to start relay: %v", err)
	}
	defer relay.Stop()

	// Get actual listen address
	addr := relay.listener.Addr().String()

	// Create multiple concurrent connections
	numConns := 5
	done := make(chan error, numConns)

	for i := 0; i < numConns; i++ {
		go func() {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				done <- err
				return
			}
			defer conn.Close()

			reader := bufio.NewReader(conn)

			// Read greeting
			_, err = reader.ReadString('\n')
			if err != nil {
				done <- err
				return
			}

			// Send QUIT
			fmt.Fprintf(conn, "QUIT\r\n")
			_, err = reader.ReadString('\n')
			done <- err
		}()
	}

	// Wait for all connections to complete
	for i := 0; i < numConns; i++ {
		if err := <-done; err != nil {
			t.Errorf("Connection %d failed: %v", i, err)
		}
	}
}

// TestSMTPDataCommand tests the DATA command and message body handling.
func TestSMTPDataCommand(t *testing.T) {
	lookups := map[string]bridge.MailConfig{
		"alice": {ForwardTo: "alice@gmail.com"},
	}
	relay, addr := setupTestRelay(t, lookups)
	defer relay.Stop()

	conn, reader := setupTestConnection(t, addr)
	defer conn.Close()

	sendEHLO(t, conn, reader)

	// Send MAIL FROM
	fmt.Fprintf(conn, "MAIL FROM:<sender@example.com>\r\n")
	reader.ReadString('\n')

	// Send RCPT TO
	fmt.Fprintf(conn, "RCPT TO:<alice@mail.bit>\r\n")
	reader.ReadString('\n')

	// Send DATA command
	fmt.Fprintf(conn, "DATA\r\n")
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read DATA response: %v", err)
	}
	if !strings.HasPrefix(line, "354 ") {
		t.Errorf("Expected 354 response to DATA, got: %s", line)
	}

	// Send message body
	message := "Subject: Test Message\r\n\r\nThis is a test message body.\r\n"
	fmt.Fprintf(conn, "%s.\r\n", message)

	// Read final response (will fail to forward but should handle protocol correctly)
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read final response: %v", err)
	}
	// Should get either success or failure, but not a protocol error
	if !strings.HasPrefix(line, "250 ") && !strings.HasPrefix(line, "554 ") {
		t.Errorf("Expected 250 or 554 response after DATA, got: %s", line)
	}

	// Send QUIT
	fmt.Fprintf(conn, "QUIT\r\n")
	reader.ReadString('\n')
}

// TestSMTPDataWithoutEnvelope tests DATA command without proper envelope.
func TestSMTPDataWithoutEnvelope(t *testing.T) {
	lookups := map[string]bridge.MailConfig{
		"alice": {ForwardTo: "alice@gmail.com"},
	}
	relay, addr := setupTestRelay(t, lookups)
	defer relay.Stop()

	conn, reader := setupTestConnection(t, addr)
	defer conn.Close()

	sendEHLO(t, conn, reader)

	// Send DATA without MAIL FROM or RCPT TO
	fmt.Fprintf(conn, "DATA\r\n")
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read DATA response: %v", err)
	}
	if !strings.HasPrefix(line, "503 ") {
		t.Errorf("Expected 503 error for DATA without envelope, got: %s", line)
	}

	// Send QUIT
	fmt.Fprintf(conn, "QUIT\r\n")
	reader.ReadString('\n')
}

// TestSMTPMessageSizeLimit tests that messages exceeding size limit are rejected.
func TestSMTPMessageSizeLimit(t *testing.T) {
	resolver := &mockSMTPResolver{
		lookups: map[string]bridge.MailConfig{
			"alice": {ForwardTo: "alice@gmail.com"},
		},
	}
	router := NewRouter(resolver, 0)
	config := DefaultRelayConfig()
	config.ListenAddr = "localhost:0"
	config.MaxMessageSize = 100 // Small limit for testing

	relay := NewRelay(router, config)
	if err := relay.Start(); err != nil {
		t.Fatalf("Failed to start relay: %v", err)
	}
	defer relay.Stop()

	addr := relay.listener.Addr().String()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Read greeting
	reader.ReadString('\n')

	// Send EHLO
	fmt.Fprintf(conn, "EHLO client.example.com\r\n")
	for {
		line, _ := reader.ReadString('\n')
		if !strings.Contains(line, "250-") {
			break
		}
	}

	// Send MAIL FROM
	fmt.Fprintf(conn, "MAIL FROM:<sender@example.com>\r\n")
	reader.ReadString('\n')

	// Send RCPT TO
	fmt.Fprintf(conn, "RCPT TO:<alice@mail.bit>\r\n")
	reader.ReadString('\n')

	// Send DATA command
	fmt.Fprintf(conn, "DATA\r\n")
	reader.ReadString('\n')

	// Send message body that exceeds size limit
	largeMessage := strings.Repeat("A", 200) + "\r\n"
	fmt.Fprintf(conn, "%s.\r\n", largeMessage)

	// Should get size exceeded error
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}
	if !strings.HasPrefix(line, "552 ") {
		t.Errorf("Expected 552 error for oversized message, got: %s", line)
	}

	// Send QUIT
	fmt.Fprintf(conn, "QUIT\r\n")
	reader.ReadString('\n')
}

// TestSMTPMessageSizeLimitDrainsData verifies oversized DATA is fully consumed
// so subsequent SMTP commands remain in sync.
func TestSMTPMessageSizeLimitDrainsData(t *testing.T) {
	resolver := &mockSMTPResolver{
		lookups: map[string]bridge.MailConfig{
			"alice": {ForwardTo: "alice@gmail.com"},
		},
	}
	router := NewRouter(resolver, 0)
	config := DefaultRelayConfig()
	config.ListenAddr = "localhost:0"
	config.MaxMessageSize = 100 // Small limit for testing

	relay := NewRelay(router, config)
	if err := relay.Start(); err != nil {
		t.Fatalf("Failed to start relay: %v", err)
	}
	defer relay.Stop()

	conn, reader := setupTestConnection(t, relay.listener.Addr().String())
	defer conn.Close()

	sendEHLO(t, conn, reader)

	fmt.Fprintf(conn, "MAIL FROM:<sender@example.com>\r\n")
	reader.ReadString('\n')
	fmt.Fprintf(conn, "RCPT TO:<alice@mail.bit>\r\n")
	reader.ReadString('\n')

	fmt.Fprintf(conn, "DATA\r\n")
	reader.ReadString('\n')

	// Exceed the limit, then include another complete DATA line before terminator.
	oversizedLine := strings.Repeat("A", 200) + "\r\n"
	fmt.Fprintf(conn, "%s", oversizedLine)
	fmt.Fprintf(conn, "This line must be drained too\r\n")
	fmt.Fprintf(conn, ".\r\n")

	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}
	if !strings.HasPrefix(line, "552 ") {
		t.Fatalf("Expected 552 error for oversized message, got: %s", line)
	}

	// If DATA wasn't drained correctly, this NOOP would be parsed from leftover body bytes.
	fmt.Fprintf(conn, "NOOP\r\n")
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read NOOP response: %v", err)
	}
	if !strings.HasPrefix(line, "250 ") {
		t.Errorf("Expected 250 response to NOOP after oversized DATA, got: %s", line)
	}

	fmt.Fprintf(conn, "QUIT\r\n")
	reader.ReadString('\n')
}
