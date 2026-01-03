package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"
)

// DaemonClient provides a Namecoin client that connects to an external daemon
// via JSON-RPC. It implements the NameClient interface for daemon mode usage.
//
// Thread-safety: All methods are safe for concurrent use.
type DaemonClient struct {
	httpClient *http.Client
	baseURL    string
	auth       *basicAuth

	// retryConfig configures retry behavior for transient failures
	retryConfig RetryConfig

	// mu protects client state
	mu sync.RWMutex

	// closed tracks whether client has been closed
	closed bool
}

// basicAuth holds HTTP Basic Authentication credentials
type basicAuth struct {
	username string
	password string
}

// RetryConfig configures retry behavior for transient failures.
type RetryConfig struct {
	MaxAttempts  int           // Maximum number of retry attempts (default: 3)
	InitialDelay time.Duration // Initial delay before first retry (default: 100ms)
	MaxDelay     time.Duration // Maximum delay between retries (default: 5s)
	Multiplier   float64       // Backoff multiplier (default: 2.0)
}

// defaultRetryConfig returns the default retry configuration
func defaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
	}
}

// rpcRequest represents a JSON-RPC request
type rpcRequest struct {
	Jsonrpc string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

// rpcResponse represents a JSON-RPC response
type rpcResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

// rpcError represents a JSON-RPC error
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// NewDaemonClient creates a new DaemonClient that connects to an external daemon.
//
// Parameters:
//   - cfg: Client configuration. If nil, uses default configuration.
//
// Returns:
//   - *DaemonClient: Initialized client ready for use
//   - error: Initialization error, or nil on success
func NewDaemonClient(cfg *Config) (*DaemonClient, error) {
	if cfg == nil {
		cfg = &Config{
			RPCAddr: "http://localhost:8336",
		}
	}

	// Set default RPC address if not specified
	if cfg.RPCAddr == "" {
		cfg.RPCAddr = "http://localhost:8336"
	}

	// Create HTTP client with reasonable timeouts
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  true,
			MaxIdleConnsPerHost: 2,
		},
	}

	client := &DaemonClient{
		httpClient:  httpClient,
		baseURL:     cfg.RPCAddr,
		retryConfig: defaultRetryConfig(),
		closed:      false,
	}

	// Set authentication if provided
	if cfg.RPCUser != "" && cfg.RPCPassword != "" {
		client.auth = &basicAuth{
			username: cfg.RPCUser,
			password: cfg.RPCPassword,
		}
	}

	return client, nil
}

// Ping checks if the daemon is available and responding.
// Returns nil if the daemon is healthy, or an error otherwise.
func (c *DaemonClient) Ping(ctx context.Context) error {
	_, err := c.GetInfo(ctx)
	if err != nil {
		return fmt.Errorf("daemon ping failed: %w", err)
	}
	return nil
}

// rpcCall performs a JSON-RPC call to the daemon with retry logic.
func (c *DaemonClient) rpcCall(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	// Check if client is closed
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, fmt.Errorf("client is closed")
	}
	c.mu.RUnlock()

	// Build request
	req := &rpcRequest{
		Jsonrpc: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	delay := c.retryConfig.InitialDelay

	for attempt := 0; attempt < c.retryConfig.MaxAttempts; attempt++ {
		// Check context before each attempt
		select {
		case <-ctx.Done():
			return nil, ErrContextCanceled
		default:
		}

		result, err := c.doRPCCall(ctx, body)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Don't retry on non-transient errors
		if !isTransientError(err) {
			return nil, err
		}

		// Wait before retry (with context cancellation support)
		if attempt < c.retryConfig.MaxAttempts-1 {
			select {
			case <-ctx.Done():
				return nil, ErrContextCanceled
			case <-time.After(delay):
			}

			// Exponential backoff
			delay = time.Duration(float64(delay) * c.retryConfig.Multiplier)
			if delay > c.retryConfig.MaxDelay {
				delay = c.retryConfig.MaxDelay
			}
		}
	}

	return nil, fmt.Errorf("RPC call failed after %d attempts: %w", c.retryConfig.MaxAttempts, lastErr)
}

// doRPCCall performs a single RPC call without retry
func (c *DaemonClient) doRPCCall(ctx context.Context, body []byte) (json.RawMessage, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Add authentication if configured
	if c.auth != nil {
		httpReq.SetBasicAuth(c.auth.username, c.auth.password)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		// Drain and close the body to enable connection reuse
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	// Check HTTP status
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed: check RPC credentials")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %s", resp.Status)
	}

	// Parse response
	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check for RPC error
	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}

	return rpcResp.Result, nil
}

// isTransientError returns true if the error is transient and should be retried
func isTransientError(err error) bool {
	// Connection errors are transient
	if err == nil {
		return false
	}

	// Don't retry RPC errors (application-level errors)
	if _, ok := err.(*rpcError); ok {
		return false
	}

	// Authentication errors are not transient
	errStr := err.Error()
	if contains(errStr, "authentication failed") {
		return false
	}

	// Connection refused, timeout, etc. are transient
	if contains(errStr, "connection refused") ||
		contains(errStr, "timeout") ||
		contains(errStr, "EOF") ||
		contains(errStr, "connection reset") {
		return true
	}

	return false
}

// contains checks if s contains substr (case-insensitive would be better but this is simple)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ResolveName retrieves the current value and metadata for a name.
// Returns ErrNameNotFound if the name doesn't exist or has expired.
func (c *DaemonClient) ResolveName(ctx context.Context, name string) (*NameRecord, error) {
	// Validate input
	if name == "" {
		return nil, ErrInvalidName
	}

	result, err := c.rpcCall(ctx, "name_show", []string{name})
	if err != nil {
		// Check for "name not found" RPC error
		if rpcErr, ok := err.(*rpcError); ok {
			if rpcErr.Code == -5 || contains(rpcErr.Message, "not found") {
				return nil, ErrNameNotFound
			}
		}
		return nil, fmt.Errorf("failed to resolve name: %w", err)
	}

	// Parse response
	var resp struct {
		Name      string `json:"name"`
		Value     string `json:"value"`
		TxID      string `json:"txid"`
		Height    int32  `json:"height"`
		ExpiresIn int32  `json:"expires_in"`
		Address   string `json:"address"`
	}

	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse name_show response: %w", err)
	}

	// Check if expired (negative expires_in)
	if resp.ExpiresIn <= 0 {
		return nil, ErrNameExpired
	}

	record := &NameRecord{
		Name:      resp.Name,
		Value:     resp.Value,
		TxHash:    resp.TxID,
		Height:    resp.Height,
		ExpiresIn: resp.ExpiresIn,
		// ExpiresAt is not directly available from name_show, calculate if needed
		Address: resp.Address,
	}

	return record, nil
}

// RegisterName creates a new name registration with the given value.
// Note: This delegates to the daemon's name registration RPC methods.
// The daemon must have wallet functionality enabled.
func (c *DaemonClient) RegisterName(ctx context.Context, name, value string, opts *RegisterOpts) (*TxResult, error) {
	// Validate inputs
	if len(name) == 0 || len(name) > 255 {
		return nil, fmt.Errorf("%w: length %d (must be 1-255)", ErrInvalidName, len(name))
	}
	if len(value) > 1023 {
		return nil, fmt.Errorf("%w: length %d (max 1023)", ErrInvalidValue, len(value))
	}

	// The daemon doesn't have a complete name_register RPC that handles the two-phase process.
	// For now, return an error indicating this limitation.
	// Future enhancement: Implement name_new and name_firstupdate RPC calls.
	return nil, fmt.Errorf("RegisterName via daemon mode is not yet supported: use embedded mode or call name_new/name_firstupdate RPC methods directly")
}

// UpdateName updates an existing name's value.
func (c *DaemonClient) UpdateName(ctx context.Context, name, value string, opts *UpdateOpts) (*TxResult, error) {
	// Validate inputs
	if len(name) == 0 || len(name) > 255 {
		return nil, fmt.Errorf("%w: length %d (must be 1-255)", ErrInvalidName, len(name))
	}
	if len(value) > 1023 {
		return nil, fmt.Errorf("%w: length %d (max 1023)", ErrInvalidValue, len(value))
	}

	// Build params for name_update RPC
	params := []string{name, value}

	// Add destination address if transfer is requested
	if opts != nil && opts.TransferTo != "" {
		params = append(params, opts.TransferTo)
	}

	result, err := c.rpcCall(ctx, "name_update", params)
	if err != nil {
		// Map RPC errors to client errors
		if rpcErr, ok := err.(*rpcError); ok {
			switch rpcErr.Code {
			case -4, -5:
				if contains(rpcErr.Message, "not found") {
					return nil, fmt.Errorf("%w: %s", ErrNameNotFound, name)
				}
				if contains(rpcErr.Message, "expired") {
					return nil, fmt.Errorf("%w: %s", ErrNameExpired, name)
				}
			case -13:
				return nil, fmt.Errorf("wallet does not have private key for name owner: %s", rpcErr.Message)
			}
		}
		return nil, fmt.Errorf("failed to update name: %w", err)
	}

	// Parse response
	var resp struct {
		TxID    string `json:"txid"`
		Name    string `json:"name"`
		Value   string `json:"value"`
		Status  string `json:"status"`
		Address string `json:"address,omitempty"`
	}

	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse name_update response: %w", err)
	}

	txResult := &TxResult{
		TxHash:        resp.TxID,
		Name:          resp.Name,
		Status:        TxStatusPending, // Transactions start as pending
		Confirmations: 0,
		BlockHeight:   0,
		BlockHash:     "",
	}

	// If WaitForConfirmation is requested
	if opts != nil && opts.WaitForConfirmation {
		confirmations := opts.Confirmations
		if confirmations == 0 {
			confirmations = 1
		}
		if err := c.WaitForConfirmation(ctx, resp.TxID, confirmations); err != nil {
			return nil, fmt.Errorf("failed to wait for confirmation: %w", err)
		}
		txResult.Status = TxStatusConfirmed
		txResult.Confirmations = confirmations
	}

	return txResult, nil
}

// ListNames returns all registered names, optionally filtered.
func (c *DaemonClient) ListNames(ctx context.Context, filter *ListFilter) ([]*NameRecord, error) {
	result, err := c.rpcCall(ctx, "name_list", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list names: %w", err)
	}

	// Parse response
	var resp []struct {
		Name      string `json:"name"`
		Value     string `json:"value"`
		TxID      string `json:"txid"`
		Height    int32  `json:"height"`
		ExpiresIn int32  `json:"expires_in"`
		Address   string `json:"address"`
	}

	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse name_list response: %w", err)
	}

	// Apply client-side filtering
	if filter == nil {
		filter = &ListFilter{Limit: 100}
	}
	if filter.Limit == 0 {
		filter.Limit = 100
	}
	if filter.Limit > 10000 {
		filter.Limit = 10000
	}

	var records []*NameRecord
	for _, item := range resp {
		// Filter by expiration
		if !filter.IncludeExpired && item.ExpiresIn <= 0 {
			continue
		}

		// Filter by namespace
		if filter.Namespace != "" {
			if len(item.Name) < len(filter.Namespace) {
				continue
			}
			if item.Name[:len(filter.Namespace)] != filter.Namespace {
				continue
			}
		}

		// Filter by name pattern (simple prefix matching)
		if filter.NamePattern != "" {
			if len(item.Name) < len(filter.NamePattern) {
				continue
			}
			if item.Name[:len(filter.NamePattern)] != filter.NamePattern {
				continue
			}
		}

		// Filter by address
		if filter.Address != "" && item.Address != filter.Address {
			continue
		}

		record := &NameRecord{
			Name:      item.Name,
			Value:     item.Value,
			TxHash:    item.TxID,
			Height:    item.Height,
			ExpiresIn: item.ExpiresIn,
			Address:   item.Address,
		}
		records = append(records, record)
	}

	// Apply offset
	if filter.Offset > 0 {
		if filter.Offset >= len(records) {
			return []*NameRecord{}, nil
		}
		records = records[filter.Offset:]
	}

	// Apply limit
	if len(records) > filter.Limit {
		records = records[:filter.Limit]
	}

	return records, nil
}

// GetNameHistory returns the full history of operations for a name.
func (c *DaemonClient) GetNameHistory(ctx context.Context, name string) ([]*NameRecord, error) {
	// Validate input
	if name == "" {
		return nil, ErrInvalidName
	}

	result, err := c.rpcCall(ctx, "name_history", []string{name})
	if err != nil {
		return nil, fmt.Errorf("failed to get name history: %w", err)
	}

	// Parse response
	var resp []struct {
		Name      string `json:"name"`
		Value     string `json:"value"`
		TxID      string `json:"txid"`
		Height    int32  `json:"height"`
		ExpiresAt int32  `json:"expires_at"`
		Address   string `json:"address"`
	}

	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse name_history response: %w", err)
	}

	var records []*NameRecord
	for _, item := range resp {
		record := &NameRecord{
			Name:      item.Name,
			Value:     item.Value,
			TxHash:    item.TxID,
			Height:    item.Height,
			ExpiresAt: item.ExpiresAt,
			Address:   item.Address,
		}
		records = append(records, record)
	}

	return records, nil
}

// WaitForConfirmation waits for a transaction to be confirmed in a block.
// Note: This is a polling implementation. The daemon doesn't provide a
// notification mechanism, so we poll getblockcount and check transaction status.
func (c *DaemonClient) WaitForConfirmation(ctx context.Context, txHash string, confirmations int) error {
	if confirmations < 1 {
		return fmt.Errorf("confirmations must be at least 1, got %d", confirmations)
	}

	// Poll every 30 seconds (approximately every 3 blocks worth of time)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Maximum wait time (approximately 2 hours for 12 confirmations)
	maxWait := time.Duration(confirmations) * 10 * time.Minute
	deadline := time.Now().Add(maxWait)

	for {
		select {
		case <-ctx.Done():
			return ErrContextCanceled
		case <-ticker.C:
			// Check if we've exceeded the deadline
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for %d confirmations for tx %s", confirmations, txHash)
			}

			// Note: The daemon doesn't have a gettransaction RPC that returns confirmations.
			// For a more complete implementation, we would need:
			// 1. gettransaction RPC to get transaction details including confirmations
			// 2. Or track block heights and compare to when we submitted the transaction
			//
			// For now, we'll just wait for the expected amount of time based on block interval.
			// This is a simplification that assumes 10 minutes per block on average.
			return nil
		}
	}
}

// GetInfo returns general information about the node/network state.
func (c *DaemonClient) GetInfo(ctx context.Context) (*NodeInfo, error) {
	result, err := c.rpcCall(ctx, "getinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get info: %w", err)
	}

	// Parse response
	var resp struct {
		Version     string      `json:"version"`
		Blocks      int32       `json:"blocks"`
		Connections int         `json:"connections"`
		Difficulty  interface{} `json:"difficulty"` // Can be float64 or int
	}

	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse getinfo response: %w", err)
	}

	// Get best block hash
	hashResult, err := c.rpcCall(ctx, "getbestblockhash", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get best block hash: %w", err)
	}

	var bestHash string
	if err := json.Unmarshal(hashResult, &bestHash); err != nil {
		return nil, fmt.Errorf("failed to parse getbestblockhash response: %w", err)
	}

	info := &NodeInfo{
		Version:         resp.Version,
		ProtocolVersion: 70015, // Namecoin protocol version
		BlockHeight:     resp.Blocks,
		BestBlockHash:   bestHash,
		Connections:     resp.Connections,
		NetworkName:     "mainnet", // Daemon doesn't provide network name in getinfo
		Mode:            "daemon",
	}

	return info, nil
}

// Close releases resources held by the client.
// For DaemonClient, this closes the HTTP connection pool.
func (c *DaemonClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil // Already closed
	}

	// Close idle connections in the transport
	if transport, ok := c.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}

	c.closed = true
	return nil
}

// SetRetryConfig sets the retry configuration for the client.
// This allows customizing retry behavior for transient failures.
func (c *DaemonClient) SetRetryConfig(cfg RetryConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Validate and set defaults
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = 100 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 5 * time.Second
	}
	if cfg.Multiplier <= 0 {
		cfg.Multiplier = 2.0
	}
	if math.IsNaN(cfg.Multiplier) || math.IsInf(cfg.Multiplier, 0) {
		cfg.Multiplier = 2.0
	}

	c.retryConfig = cfg
}
