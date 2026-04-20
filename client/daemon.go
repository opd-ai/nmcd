package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/internal/logging"
)

// DaemonClient provides a Namecoin client that connects to an external daemon
// via JSON-RPC. It implements the NameClient interface for daemon mode usage.
//
// Thread-safety: All methods are safe for concurrent use.
type DaemonClient struct {
	httpClient *http.Client
	baseURL    string
	auth       *basicAuth
	logger     *logging.Logger

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

	// Create HTTP client with optimized connection pooling for production use.
	// Connection pooling reduces latency and resource usage by reusing TCP connections.
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			// MaxIdleConns: Total idle connections across all hosts (100 for high-throughput)
			MaxIdleConns: 100,
			// IdleConnTimeout: How long idle connections stay open (90s balances reuse vs resources)
			IdleConnTimeout: 90 * time.Second,
			// DisableCompression: Disabled for RPC (JSON is already compact, compression adds CPU overhead)
			DisableCompression: true,
			// MaxIdleConnsPerHost: Idle connections per daemon (10 for concurrent requests, was 2)
			MaxIdleConnsPerHost: 10,
			// MaxConnsPerHost: Total connections per daemon (20 to prevent resource exhaustion)
			MaxConnsPerHost: 20,
			// WriteBufferSize/ReadBufferSize: We intentionally rely on the default 4KB values
			// provided by net/http for JSON-RPC payload sizes, so these fields are left unset.
		},
	}

	client := &DaemonClient{
		httpClient:  httpClient,
		baseURL:     cfg.RPCAddr,
		retryConfig: defaultRetryConfig(),
		closed:      false,
	}

	// Validate authentication configuration: either both credentials must be provided, or neither
	hasUser := cfg.RPCUser != ""
	hasPassword := cfg.RPCPassword != ""
	if hasUser != hasPassword {
		return nil, fmt.Errorf("incomplete RPC authentication: both RPCUser and RPCPassword must be provided together, or neither")
	}

	// Set authentication if both credentials are provided
	if hasUser && hasPassword {
		client.auth = &basicAuth{
			username: cfg.RPCUser,
			password: cfg.RPCPassword,
		}
	}

	// Initialize logger (use provided or default)
	var logger *logging.Logger
	if cfg.Logger != nil {
		logger = cfg.Logger
	} else {
		logger = logging.GetDefault()
	}
	client.logger = logger.WithComponent("daemon-client")

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
	retryCfg, err := c.getRetryConfig()
	if err != nil {
		return nil, err
	}

	body, err := marshalRPCRequest(method, params)
	if err != nil {
		return nil, err
	}

	return c.executeWithRetry(ctx, body, retryCfg)
}

// getRetryConfig returns the retry configuration after checking client state.
func (c *DaemonClient) getRetryConfig() (RetryConfig, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return RetryConfig{}, fmt.Errorf("client is closed")
	}
	return c.retryConfig, nil
}

// marshalRPCRequest creates and marshals a JSON-RPC request body.
func marshalRPCRequest(method string, params interface{}) ([]byte, error) {
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
	return body, nil
}

// executeWithRetry performs the RPC call with exponential backoff retry logic.
func (c *DaemonClient) executeWithRetry(ctx context.Context, body []byte, retryCfg RetryConfig) (json.RawMessage, error) {
	var lastErr error
	delay := retryCfg.InitialDelay

	for attempt := 0; attempt < retryCfg.MaxAttempts; attempt++ {
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
		if !isTransientError(err) {
			return nil, err
		}

		if attempt < retryCfg.MaxAttempts-1 {
			delay = c.waitAndBackoff(ctx, delay, retryCfg)
			if delay < 0 {
				return nil, ErrContextCanceled
			}
		}
	}

	return nil, fmt.Errorf("RPC call failed after %d attempts: %w", retryCfg.MaxAttempts, lastErr)
}

// waitAndBackoff waits for the backoff delay and returns the next delay.
// Returns -1 if context is cancelled.
func (c *DaemonClient) waitAndBackoff(ctx context.Context, delay time.Duration, retryCfg RetryConfig) time.Duration {
	select {
	case <-ctx.Done():
		return -1
	case <-time.After(delay):
	}
	nextDelay := time.Duration(float64(delay) * retryCfg.Multiplier)
	if nextDelay > retryCfg.MaxDelay {
		nextDelay = retryCfg.MaxDelay
	}
	return nextDelay
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
	if strings.Contains(errStr, "authentication failed") {
		return false
	}

	// Connection refused, timeout, etc. are transient
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "EOF") ||
		strings.Contains(errStr, "connection reset") {
		return true
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
			if rpcErr.Code == -5 || strings.Contains(rpcErr.Message, "not found") {
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

	// Check if expired (negative expires_in means already expired)
	// ExpiresIn of 0 means expires at current block, which is still valid
	if resp.ExpiresIn < 0 {
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
//
// RegisterName implements the two-phase Namecoin registration workflow:
//  1. Calls name_new RPC to create a commitment transaction
//  2. If WaitForConfirmation is false, returns immediately with the NAME_NEW txid
//  3. If WaitForConfirmation is true, waits for 12 block confirmations, then
//     calls name_firstupdate RPC to complete the registration
//
// The rand value returned by name_new is required for name_firstupdate.
// This method uses that value internally only when WaitForConfirmation is true.
// When WaitForConfirmation is false, RegisterName returns the NAME_NEW txid but
// does not expose rand to the caller. Callers that need to complete
// name_firstupdate later must either set WaitForConfirmation to true or call
// name_new directly and persist the returned rand themselves.
func (c *DaemonClient) RegisterName(ctx context.Context, name, value string, opts *RegisterOpts) (*TxResult, error) {
	// Validate inputs - use UI limit (520 bytes) matching Namecoin Core
	if len(name) == 0 || len(name) > 255 {
		return nil, fmt.Errorf("%w: length %d (must be 1-255)", ErrInvalidName, len(name))
	}
	if len(value) > config.MaxValueLengthUI {
		return nil, fmt.Errorf("%w: length %d (max %d)", ErrInvalidValue, len(value), config.MaxValueLengthUI)
	}

	// Phase 1: Call name_new to create commitment
	nameNewResult, err := c.rpcCall(ctx, "name_new", []string{name})
	if err != nil {
		return nil, fmt.Errorf("name_new failed: %w", err)
	}

	txid, rand, err := parseNameNewResponse(nameNewResult)
	if err != nil {
		return nil, err
	}

	result := &TxResult{
		TxHash: txid,
		Name:   name,
		Status: TxStatusPending,
	}

	if opts == nil || !opts.WaitForConfirmation {
		return result, nil
	}

	// Phase 2: Wait for 12 confirmations, then call name_firstupdate
	return c.completeNameRegistration(ctx, name, value, txid, rand, result)
}

// nameNewMinConfirmations is the minimum number of block confirmations
// required before a NAME_FIRSTUPDATE can reference a NAME_NEW commitment.
const nameNewMinConfirmations = 12

// parseNameNewResponse extracts the txid and rand from a name_new RPC response.
func parseNameNewResponse(result json.RawMessage) (txid, rand string, err error) {
	var resp struct {
		TxID string `json:"txid"`
		Rand string `json:"rand"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return "", "", fmt.Errorf("failed to parse name_new response: %w", err)
	}
	if resp.TxID == "" {
		return "", "", fmt.Errorf("name_new response missing txid")
	}
	if resp.Rand == "" {
		return "", "", fmt.Errorf("name_new response missing rand")
	}
	return resp.TxID, resp.Rand, nil
}

// completeNameRegistration waits for NAME_NEW to be confirmed and then
// calls name_firstupdate to complete the two-phase registration.
func (c *DaemonClient) completeNameRegistration(ctx context.Context, name, value, nameNewTxID, rand string, result *TxResult) (*TxResult, error) {
	if err := c.WaitForConfirmation(ctx, nameNewTxID, nameNewMinConfirmations); err != nil {
		return nil, fmt.Errorf("failed waiting for NAME_NEW confirmation: %w", err)
	}

	firstUpdateResult, err := c.rpcCall(ctx, "name_firstupdate", []string{name, rand, value})
	if err != nil {
		return nil, fmt.Errorf("name_firstupdate failed: %w", err)
	}

	var resp struct {
		TxID string `json:"txid"`
	}
	if err := json.Unmarshal(firstUpdateResult, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse name_firstupdate response: %w", err)
	}
	if resp.TxID == "" {
		return nil, fmt.Errorf("name_firstupdate response missing txid")
	}

	result.TxHash = resp.TxID
	result.Status = TxStatusPending
	result.Confirmations = 0
	return result, nil
}

// UpdateName updates an existing name's value.
func (c *DaemonClient) UpdateName(ctx context.Context, name, value string, opts *UpdateOpts) (*TxResult, error) {
	if len(name) == 0 || len(name) > 255 {
		return nil, fmt.Errorf("%w: length %d (must be 1-255)", ErrInvalidName, len(name))
	}
	if len(value) > config.MaxValueLengthUI {
		return nil, fmt.Errorf("%w: length %d (max %d)", ErrInvalidValue, len(value), config.MaxValueLengthUI)
	}

	params := []string{name, value}
	if opts != nil && opts.TransferTo != "" {
		params = append(params, opts.TransferTo)
	}

	result, err := c.rpcCall(ctx, "name_update", params)
	if err != nil {
		return nil, mapUpdateNameError(err, name)
	}

	return c.parseUpdateResponse(ctx, result, opts)
}

// mapUpdateNameError converts RPC errors to client-specific errors for name_update.
func mapUpdateNameError(err error, name string) error {
	if rpcErr, ok := err.(*rpcError); ok {
		switch rpcErr.Code {
		case -4, -5:
			if strings.Contains(rpcErr.Message, "not found") {
				return fmt.Errorf("%w: %s", ErrNameNotFound, name)
			}
			if strings.Contains(rpcErr.Message, "expired") {
				return fmt.Errorf("%w: %s", ErrNameExpired, name)
			}
		case -13:
			return fmt.Errorf("wallet does not have private key for name owner: %s", rpcErr.Message)
		}
	}
	return fmt.Errorf("failed to update name: %w", err)
}

// parseUpdateResponse parses the name_update RPC response and optionally waits for confirmation.
func (c *DaemonClient) parseUpdateResponse(ctx context.Context, result json.RawMessage, opts *UpdateOpts) (*TxResult, error) {
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
		TxHash: resp.TxID,
		Name:   resp.Name,
		Status: TxStatusPending,
	}

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

	filter = normalizeListFilter(filter)

	var records []*NameRecord
	for _, item := range resp {
		if !matchesListFilter(item.Name, item.Address, item.ExpiresIn, filter) {
			continue
		}
		records = append(records, &NameRecord{
			Name:      item.Name,
			Value:     item.Value,
			TxHash:    item.TxID,
			Height:    item.Height,
			ExpiresIn: item.ExpiresIn,
			Address:   item.Address,
		})
	}

	return applyPagination(records, filter), nil
}

// normalizeListFilter applies default values to a list filter.
func normalizeListFilter(filter *ListFilter) *ListFilter {
	if filter == nil {
		return &ListFilter{Limit: 100}
	}
	if filter.Limit == 0 {
		filter.Limit = 100
	}
	if filter.Limit > 10000 {
		filter.Limit = 10000
	}
	return filter
}

// matchesListFilter checks whether a name record passes the filter criteria.
func matchesListFilter(name, address string, expiresIn int32, filter *ListFilter) bool {
	if !filter.IncludeExpired && expiresIn < 0 {
		return false
	}
	if filter.Namespace != "" && !strings.HasPrefix(name, filter.Namespace) {
		return false
	}
	if filter.NamePattern != "" && !strings.HasPrefix(name, filter.NamePattern) {
		return false
	}
	if filter.Address != "" && address != filter.Address {
		return false
	}
	return true
}

// applyPagination applies offset and limit to a slice of name records.
func applyPagination(records []*NameRecord, filter *ListFilter) []*NameRecord {
	if filter.Offset > 0 {
		if filter.Offset >= len(records) {
			return []*NameRecord{}
		}
		records = records[filter.Offset:]
	}
	if len(records) > filter.Limit {
		records = records[:filter.Limit]
	}
	return records
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

// txInfo holds transaction information from getrawtransaction RPC
type txInfo struct {
	TxID          string `json:"txid"`
	Confirmations int    `json:"confirmations"`
	BlockHash     string `json:"blockhash,omitempty"`
	BlockHeight   int32  `json:"blockheight,omitempty"`
}

// getRawTransaction calls the getrawtransaction RPC method to get transaction info.
// Returns transaction information including confirmation count, or an error if the
// transaction is not found or the RPC call fails.
func (c *DaemonClient) getRawTransaction(ctx context.Context, txHash string) (*txInfo, error) {
	// Call getrawtransaction with verbose=true (second parameter) to get JSON response with confirmations
	// When verbose=false, it returns hex-encoded transaction data instead
	params := []interface{}{txHash, true}
	result, err := c.rpcCall(ctx, "getrawtransaction", params)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	var info txInfo
	if err := json.Unmarshal(result, &info); err != nil {
		return nil, fmt.Errorf("failed to parse transaction info: %w", err)
	}

	return &info, nil
}

// WaitForConfirmation waits for a transaction to be confirmed in a block.
//
// This method polls the daemon using the getrawtransaction RPC to check actual
// blockchain confirmations. It polls every second to allow for timely context
// cancellation and responsive feedback without excessive RPC load.
//
// Note: DaemonClient uses a 1-second polling interval (vs EmbeddedClient's 10-second
// interval) because RPC calls to an external daemon are typically more tolerant of
// frequent requests than direct blockchain access, and faster polling provides better
// responsiveness for remote operations.
//
// The function returns when the transaction has reached the requested number of
// confirmations, or when the context is canceled. If the transaction is not found
// in the blockchain, it continues polling until the context deadline.
func (c *DaemonClient) WaitForConfirmation(ctx context.Context, txHash string, confirmations int) error {
	if confirmations < 1 {
		return fmt.Errorf("confirmations must be at least 1, got %d", confirmations)
	}

	select {
	case <-ctx.Done():
		return ErrContextCanceled
	default:
	}

	if c.txHasConfirmations(ctx, txHash, confirmations) {
		return nil
	}

	return c.pollDaemonConfirmation(ctx, txHash, confirmations)
}

// txHasConfirmations checks if a transaction has at least the required confirmations.
func (c *DaemonClient) txHasConfirmations(ctx context.Context, txHash string, required int) bool {
	info, err := c.getRawTransaction(ctx, txHash)
	return err == nil && info.Confirmations >= required
}

// pollDaemonConfirmation polls the daemon until the transaction has enough confirmations.
func (c *DaemonClient) pollDaemonConfirmation(ctx context.Context, txHash string, required int) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ErrContextCanceled
		case <-ticker.C:
			if c.txHasConfirmations(ctx, txHash, required) {
				return nil
			}
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
	// Multiplier must be >= 1.0 (no backoff reduction) and finite
	if cfg.Multiplier < 1.0 || math.IsNaN(cfg.Multiplier) || math.IsInf(cfg.Multiplier, 0) {
		cfg.Multiplier = 2.0
	}

	c.retryConfig = cfg
}

// GetBlockHash retrieves the hash of a block at the specified height.
// This is primarily used for network detection via genesis block hash.
func (c *DaemonClient) GetBlockHash(ctx context.Context, height int32) (string, error) {
	params := []interface{}{height}
	result, err := c.rpcCall(ctx, "getblockhash", params)
	if err != nil {
		return "", fmt.Errorf("failed to get block hash: %w", err)
	}

	var blockHash string
	if err := json.Unmarshal(result, &blockHash); err != nil {
		return "", fmt.Errorf("failed to parse block hash: %w", err)
	}

	return blockHash, nil
}

// DetectNetwork queries the daemon to detect which network it's running on.
// It does this by retrieving the genesis block hash and comparing it against
// known Namecoin network genesis hashes.
//
// Returns:
//   - "mainnet", "testnet", or "regtest" on successful detection
//   - error if network cannot be determined or RPC call fails
func (c *DaemonClient) DetectNetwork(ctx context.Context) (string, error) {
	// Get genesis block hash (block 0)
	genesisHash, err := c.GetBlockHash(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get genesis hash: %w", err)
	}

	// Compare against known Namecoin network genesis hashes
	// Mainnet: 000000000062b72c5e2ceb45fbc8c80c7b157c0da7e635483dfba2a9f0a9c770
	// Testnet: 00000007199508e34a9ff81e6ec0c477a4cccff2a4767a8eee39c11db367b008
	// These are reversed from the internal representation in config/namecoin_params.go
	switch genesisHash {
	case "000000000062b72c5e2ceb45fbc8c80c7b157c0da7e635483dfba2a9f0a9c770":
		return "mainnet", nil
	case "00000007199508e34a9ff81e6ec0c477a4cccff2a4767a8eee39c11db367b008":
		return "testnet", nil
	default:
		// Regtest can have any genesis hash, but since we can't reliably detect it,
		// we'll assume anything not mainnet/testnet is regtest
		return "regtest", nil
	}
}
