package rpc

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/config"
	"github.com/opd-ai/nmcd/internal/logging"
	"github.com/opd-ai/nmcd/metrics"
	"github.com/opd-ai/nmcd/network"
	"github.com/opd-ai/nmcd/wallet"
)

const (
	// opNameNew is the opcode for NAME_NEW operations (0xd0)
	// Used to identify NAME_NEW outputs in transaction scripts
	opNameNew = 0xd0

	// defaultRateLimit is the default number of requests per minute per IP
	defaultRateLimit = 100

	// defaultMaxRequestSize is the default maximum request body size (1MB)
	defaultMaxRequestSize = 1 * 1024 * 1024
)

// Server provides RPC interface using standard library
type Server struct {
	blockchain     *chain.BlockChain
	peerMgr        *network.PeerManager
	wallet         *wallet.Wallet
	listener       net.Listener
	server         *http.Server
	rpcUser        string
	rpcPassword    string
	authKey        []byte // Per-process HMAC key for credential comparison
	rateLimiter    *rateLimiter
	maxRequestSize int64
	logger         *logging.Logger
	mu             sync.RWMutex
	autoLockTimer  *time.Timer // Auto-lock timer for walletpassphrase
	autoLockMu     sync.Mutex  // Protects autoLockTimer and autoLockGen
	autoLockGen    uint64      // Generation counter to invalidate superseded timers
	stopOnce       sync.Once
}

// Config holds RPC server configuration
type Config struct {
	Blockchain     *chain.BlockChain
	PeerMgr        *network.PeerManager
	Wallet         *wallet.Wallet
	ListenAddr     string
	RPCUser        string
	RPCPassword    string
	RateLimit      int   // Requests per minute per IP (0 = use default (100))
	MaxRequestSize int64 // Maximum request body size in bytes (0 = 1MB default)
}

// Request represents a JSON-RPC request
type Request struct {
	Jsonrpc string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

// Response represents a JSON-RPC response
type Response struct {
	Jsonrpc string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// Error represents a JSON-RPC error
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// errorResponse creates a JSON-RPC error response with the given code and message.
func errorResponse(id interface{}, code int, message string) *Response {
	return &Response{
		Jsonrpc: "2.0",
		Error: &Error{
			Code:    code,
			Message: message,
		},
		ID: id,
	}
}

// successResponse creates a JSON-RPC success response with the given result.
func successResponse(id, result interface{}) *Response {
	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      id,
	}
}

// requireWallet returns an error response if the wallet is not initialized.
func (s *Server) requireWallet(reqID interface{}) *Response {
	if s.wallet == nil {
		return errorResponse(reqID, -1, "Wallet not initialized. Start the node with wallet enabled.")
	}
	return nil
}

// requireBlockchain returns an error response if the blockchain is not initialized.
func (s *Server) requireBlockchain(reqID interface{}) *Response {
	if s.blockchain == nil {
		return errorResponse(reqID, -32603, "Blockchain not initialized")
	}
	return nil
}

// parseInterfaceParams unmarshals JSON params into []interface{} with a minimum count.
// Returns the params and nil on success, or nil and an error response on failure.
func parseInterfaceParams(params json.RawMessage, reqID interface{}, minCount int, usage string) ([]interface{}, *Response) {
	var result []interface{}
	if err := json.Unmarshal(params, &result); err != nil || len(result) < minCount {
		return nil, errorResponse(reqID, -32602, "Invalid params: expected "+usage)
	}
	return result, nil
}

// parseStringParams unmarshals JSON params into []string with a minimum count.
// Returns the params and nil on success, or nil and an error response on failure.
func parseStringParams(params json.RawMessage, reqID interface{}, minCount int, usage string) ([]string, *Response) {
	var result []string
	if err := json.Unmarshal(params, &result); err != nil || len(result) < minCount {
		return nil, errorResponse(reqID, -32602, "Invalid params: expected "+usage)
	}
	return result, nil
}

// parseHashParam parses a hash string from params and returns a chainhash.Hash.
func parseHashParam(params []interface{}, index int, reqID interface{}, paramName string) (*chainhash.Hash, *Response) {
	hashStr, ok := params[index].(string)
	if !ok {
		return nil, errorResponse(reqID, -32602, fmt.Sprintf("Invalid params: %s must be a string", paramName))
	}
	hash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		return nil, errorResponse(reqID, -5, fmt.Sprintf("Invalid %s: %v", paramName, err))
	}
	return hash, nil
}

// parseVerboseParam parses an optional verbose boolean from params.
func parseVerboseParam(params []interface{}, index int, reqID interface{}) (bool, *Response) {
	if len(params) <= index {
		return false, nil
	}
	v, ok := params[index].(bool)
	if !ok {
		return false, errorResponse(reqID, -32602, "Invalid params: verbose must be a boolean")
	}
	return v, nil
}

// validateNameLength validates that a name is between 1 and 255 characters.
func validateNameLength(name string, reqID interface{}) *Response {
	if len(name) == 0 || len(name) > 255 {
		return errorResponse(reqID, -5, fmt.Sprintf("Invalid name length: %d (max 255)", len(name)))
	}
	return nil
}

// validateValueSize validates that a name value does not exceed the UI limit (520 bytes).
// This matches Namecoin Core's MAX_VALUE_LENGTH_UI constant.
// Note: The consensus limit is 1023 bytes, but user-facing APIs enforce the 520-byte UI limit.
func validateValueSize(value string, reqID interface{}) *Response {
	if len(value) > config.MaxValueLengthUI {
		return errorResponse(reqID, -5, fmt.Sprintf("Value too large: %d bytes (max %d)",
			len(value), config.MaxValueLengthUI))
	}
	return nil
}

// broadcastAndRespond broadcasts a transaction and returns a success or error response.
func (s *Server) broadcastAndRespond(tx *wire.MsgTx, reqID interface{}, result map[string]interface{}) *Response {
	if s.peerMgr == nil {
		return errorResponse(reqID, -1, "Network not available: peer manager not initialized")
	}
	if err := s.peerMgr.BroadcastTx(tx); err != nil {
		if errors.Is(err, network.ErrNoPeers) {
			s.logWarn("transaction accepted locally but not relayed",
				"tx_hash", tx.TxHash().String())
			result["warning"] = err.Error()
			return successResponse(reqID, result)
		}
		return errorResponse(reqID, -1, fmt.Sprintf("Failed to broadcast transaction: %v", err))
	}
	return successResponse(reqID, result)
}

// NewServer creates a new RPC server
func NewServer(cfg *Config) (*Server, error) {
	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", cfg.ListenAddr, err)
	}

	// Warn when both credentials are empty and the listen address is not loopback.
	// An unprotected RPC server on a non-loopback address exposes wallet and name
	// operations to anyone with network access.
	if cfg.RPCUser == "" && cfg.RPCPassword == "" {
		if host, _, splitErr := net.SplitHostPort(cfg.ListenAddr); splitErr == nil {
			ip := net.ParseIP(host)
			if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
				log.Printf("WARNING: RPC server listening on %s with no credentials configured — "+
					"all RPC methods are accessible without authentication", cfg.ListenAddr)
			}
		}
	}

	// Set default rate limit if not configured
	rateLimit := cfg.RateLimit
	if rateLimit == 0 {
		rateLimit = defaultRateLimit
	}

	// Set default max request size if not configured
	maxRequestSize := cfg.MaxRequestSize
	if maxRequestSize == 0 {
		maxRequestSize = defaultMaxRequestSize
	}

	// Initialize logger for RPC server
	logger := logging.GetDefault().WithComponent("rpc")

	// Generate a per-process HMAC key for constant-time credential comparison.
	authKey := make([]byte, 32)
	if _, err := rand.Read(authKey); err != nil {
		listener.Close()
		return nil, fmt.Errorf("failed to generate auth key: %w", err)
	}

	s := &Server{
		blockchain:     cfg.Blockchain,
		peerMgr:        cfg.PeerMgr,
		wallet:         cfg.Wallet,
		listener:       listener,
		rpcUser:        cfg.RPCUser,
		rpcPassword:    cfg.RPCPassword,
		authKey:        authKey,
		rateLimiter:    newRateLimiter(rateLimit),
		maxRequestSize: maxRequestSize,
		logger:         logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.withPanicRecovery(s.handleRequest))
	mux.HandleFunc("/health", s.withPanicRecovery(s.handleHealth))
	mux.HandleFunc("/ready", s.withPanicRecovery(s.handleReady))

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return s, nil
}

// Start starts the RPC server and returns an error channel
func (s *Server) Start() <-chan error {
	errCh := make(chan error, 1)

	go func() {
		err := s.server.Serve(s.listener)
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	return errCh
}

// Stop stops the RPC server.
// Safe to call multiple times; only the first call has effect.
func (s *Server) Stop() error {
	var stopErr error
	s.stopOnce.Do(func() {
		// Stop rate limiter cleanup goroutine
		if s.rateLimiter != nil {
			s.rateLimiter.stop()
		}

		// Close the HTTP server
		serverErr := s.server.Close()

		// Close the listener to release the port binding
		// This is especially important for tests that create servers without starting them
		if s.listener != nil {
			if err := s.listener.Close(); err != nil && serverErr == nil {
				serverErr = err
			}
		}

		stopErr = serverErr
	})
	return stopErr
}

// Close closes the RPC server (alias for Stop for compatibility)
func (s *Server) Close() error {
	return s.Stop()
}

// checkAuth validates HTTP Basic Authentication credentials.
// Returns true if the request contains valid credentials matching
// the configured rpcUser and rpcPassword.
// Uses HMAC-SHA256 with a per-process random key before constant-time
// comparison to prevent length-based timing side-channels.
func (s *Server) checkAuth(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	mac := hmac.New(sha256.New, s.authKey)
	writeHMACField(mac, []byte(s.rpcUser))
	writeHMACField(mac, []byte(s.rpcPassword))
	expectedMAC := mac.Sum(nil)
	mac.Reset()
	writeHMACField(mac, []byte(user))
	writeHMACField(mac, []byte(pass))
	providedMAC := mac.Sum(nil)
	return subtle.ConstantTimeCompare(expectedMAC, providedMAC) == 1
}

// writeHMACField writes a length-prefixed field to the HMAC to prevent
// ambiguous credential parsing when user or pass contains separator characters.
func writeHMACField(mac hash.Hash, data []byte) {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	mac.Write(lenBuf[:])
	mac.Write(data)
}

// withPanicRecovery wraps an HTTP handler with panic recovery middleware.
// If a panic occurs, it logs the panic with full stack trace and returns a 500 error.
// This prevents the entire server from crashing due to unexpected panics.
//
// Note: If the handler already started writing the response (via w.WriteHeader or w.Write)
// before panicking, the http.Error call will not modify the response headers, but the panic
// will still be logged properly with full context.
func (s *Server) withPanicRecovery(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				s.logError("panic recovered in HTTP handler",
					"error", err,
					"method", r.Method,
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr,
					"stack", string(debug.Stack()),
				)

				// Record error metric
				metrics.Get().RecordValidationError("panic")

				// Return internal server error to client (don't expose panic details)
				// Note: This may fail silently if headers were already written
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()

		// Call the actual handler
		handler(w, r)
	}
}

// handleRequest handles incoming RPC requests with security hardening:
// - Request size validation
// - Rate limiting per IP
// - Security headers
// - Authentication
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)

	if err := s.validateHTTPRequest(w, r); err != nil {
		return
	}

	limitedReader := http.MaxBytesReader(w, r.Body, s.maxRequestSize)
	defer limitedReader.Close()

	var req Request
	if err := json.NewDecoder(limitedReader).Decode(&req); err != nil {
		s.writeError(w, &req, -32700, "Parse error")
		return
	}

	resp := s.processRequest(&req)
	s.logRPCError(resp, req.Method)
	s.writeJSONResponse(w, resp, req.Method)
}

// setSecurityHeaders sets standard security headers on the response.
func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
}

// validateHTTPRequest checks HTTP method, content length, rate limiting, and authentication.
// Returns an error (and writes the HTTP error) if any check fails.
func (s *Server) validateHTTPRequest(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return fmt.Errorf("method not allowed")
	}
	// Accept requests without Content-Length (e.g. chunked transfer encoding).
	// The body size is enforced by http.MaxBytesReader in the caller, so we only
	// reject requests that explicitly declare a size exceeding the limit.
	if r.ContentLength > s.maxRequestSize {
		http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
		return fmt.Errorf("request too large")
	}
	if !s.rateLimiter.allow(extractIP(r.RemoteAddr)) {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return fmt.Errorf("rate limit exceeded")
	}
	if (s.rpcUser != "" || s.rpcPassword != "") && !s.checkAuth(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="nmcd RPC"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return fmt.Errorf("unauthorized")
	}
	return nil
}

// logRPCError logs RPC errors for internal tracking.
func (s *Server) logRPCError(resp *Response, method string) {
	if resp.Error != nil && s.logger != nil {
		s.logger.Warn("rpc request error",
			"method", method,
			"error_code", resp.Error.Code,
			"error_message", resp.Error.Message,
		)
	}
}

// writeJSONResponse encodes and writes the JSON-RPC response.
func (s *Server) writeJSONResponse(w http.ResponseWriter, resp *Response, method string) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logError("failed to encode JSON-RPC response", "error", err, "method", method)
	}
}

// processRequest processes a JSON-RPC request
// Note: We don't need to lock s.mu here because blockchain, peerMgr, and wallet
// are set once during NewServer() and never modified. This allows parallel processing
// of concurrent RPC requests, improving throughput significantly.
func (s *Server) processRequest(req *Request) *Response {
	switch req.Method {
	case "getinfo":
		return s.getInfo(req)
	case "getblockcount":
		return s.getBlockCount(req)
	case "getbestblockhash":
		return s.getBestBlockHash(req)
	case "getconnectioncount":
		return s.getConnectionCount(req)
	case "getpeerinfo":
		return s.getPeerInfo(req)
	case "getmetrics":
		return s.getMetrics(req)
	case "name_show":
		return s.nameShow(req)
	case "name_new":
		return s.nameNew(req)
	case "name_firstupdate":
		return s.nameFirstUpdate(req)
	case "name_update":
		return s.nameUpdate(req)
	case "name_list":
		return s.nameList(req)
	case "name_history":
		return s.nameHistory(req)
	case "name_scan":
		return s.nameScan(req)
	case "name_pending":
		return s.namePending(req)
	case "getnewaddress":
		return s.getNewAddress(req)
	case "listaddresses":
		return s.listAddresses(req)
	case "walletpassphrase":
		return s.walletPassphrase(req)
	case "walletlock":
		return s.walletLock(req)
	case "encryptwallet":
		return s.encryptWallet(req)
	case "getbalance":
		return s.getBalance(req)
	case "listunspent":
		return s.listUnspent(req)
	case "getblock":
		return s.getBlock(req)
	case "getblockhash":
		return s.getBlockHash(req)
	case "getrawtransaction":
		return s.getRawTransaction(req)
	case "sendrawtransaction":
		return s.sendRawTransaction(req)
	default:
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32601,
				Message: "Method not found",
			},
			ID: req.ID,
		}
	}
}

// getDifficultyRatio returns the proof-of-work difficulty as a multiple of the
// minimum difficulty using the passed bits field from the header of a block.
// This matches Bitcoin Core's difficulty calculation.
func getDifficultyRatio(bits uint32, params *chaincfg.Params) float64 {
	// The minimum difficulty is the max target (difficulty 1)
	max := blockchain.CompactToBig(params.PowLimitBits)
	target := blockchain.CompactToBig(bits)

	difficulty := new(big.Rat).SetFrac(max, target)
	outString := difficulty.FloatString(8)
	diff, err := strconv.ParseFloat(outString, 64)
	if err != nil {
		return 0
	}
	return diff
}

// getInfo returns general information
func (s *Server) getInfo(req *Request) *Response {
	if r := s.requireBlockchain(req.ID); r != nil {
		return r
	}

	best := s.blockchain.BestSnapshot()

	connections := 0
	if s.peerMgr != nil {
		connections = s.peerMgr.GetConnectedPeers()
	}

	info := map[string]interface{}{
		"version":     "0.1.0",
		"blocks":      best.Height,
		"connections": connections,
		"difficulty":  getDifficultyRatio(best.Bits, s.blockchain.ChainParams()),
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  info,
		ID:      req.ID,
	}
}

// getBlockCount returns the current block count
func (s *Server) getBlockCount(req *Request) *Response {
	if r := s.requireBlockchain(req.ID); r != nil {
		return r
	}

	best := s.blockchain.BestSnapshot()

	return &Response{
		Jsonrpc: "2.0",
		Result:  best.Height,
		ID:      req.ID,
	}
}

// getBestBlockHash returns the best block hash
func (s *Server) getBestBlockHash(req *Request) *Response {
	if r := s.requireBlockchain(req.ID); r != nil {
		return r
	}

	best := s.blockchain.BestSnapshot()

	return &Response{
		Jsonrpc: "2.0",
		Result:  best.Hash.String(),
		ID:      req.ID,
	}
}

// getConnectionCount returns the number of connections
func (s *Server) getConnectionCount(req *Request) *Response {
	count := 0
	if s.peerMgr != nil {
		count = s.peerMgr.GetConnectedPeers()
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  count,
		ID:      req.ID,
	}
}

// getPeerInfo returns information about peers
func (s *Server) getPeerInfo(req *Request) *Response {
	if s.peerMgr == nil {
		return &Response{
			Jsonrpc: "2.0",
			Result:  []interface{}{},
			ID:      req.ID,
		}
	}

	peers := s.peerMgr.GetPeerInfo()

	return &Response{
		Jsonrpc: "2.0",
		Result:  peers,
		ID:      req.ID,
	}
}

// getMetrics returns node metrics for monitoring
func (s *Server) getMetrics(req *Request) *Response {
	snapshot := metrics.Get().Snapshot()

	return &Response{
		Jsonrpc: "2.0",
		Result:  snapshot,
		ID:      req.ID,
	}
}

// logWarn logs a warning if the server logger is initialised; otherwise it is a no-op.
func (s *Server) logWarn(msg string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Warn(msg, args...)
	}
}

// logError logs an error if the server logger is initialised; otherwise it is a no-op.
func (s *Server) logError(msg string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Error(msg, args...)
	}
}

// writeError writes an error response
func (s *Server) writeError(w http.ResponseWriter, req *Request, code int, message string) {
	s.logWarn("rpc error response",
		"method", req.Method,
		"error_code", code,
		"error_message", message,
	)

	resp := &Response{
		Jsonrpc: "2.0",
		Error: &Error{
			Code:    code,
			Message: message,
		},
		ID: req.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logError("failed to encode JSON-RPC error response",
			"error", err,
			"method", req.Method,
		)
	}
}

// HealthResponse represents the JSON response for health endpoints
type HealthResponse struct {
	Status      string `json:"status"`
	BlockHeight int32  `json:"block_height"`
	Peers       int    `json:"peers"`
	Syncing     bool   `json:"syncing,omitempty"`
}

// handleHealth handles GET requests to /health endpoint
// Returns 200 OK if the daemon is running, 503 Service Unavailable if initializing
// This endpoint is suitable for Kubernetes liveness probes
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// No lock needed - blockchain, peerMgr are immutable after initialization

	// Set Content-Type header before any writes
	w.Header().Set("Content-Type", "application/json")

	// Check if blockchain is initialized
	if s.blockchain == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(HealthResponse{
			Status: "initializing",
		}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode health response: %v\n", err)
		}
		return
	}

	// Daemon is running - get current state
	best := s.blockchain.BestSnapshot()
	peers := 0
	if s.peerMgr != nil {
		peers = s.peerMgr.GetConnectedPeers()
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(HealthResponse{
		Status:      "healthy",
		BlockHeight: best.Height,
		Peers:       peers,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode health response: %v\n", err)
	}
}

// handleReady handles GET requests to /ready endpoint
// Returns 200 OK if sync is complete, 503 Service Unavailable if still syncing
// This endpoint is suitable for Kubernetes readiness probes
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// No lock needed - blockchain, peerMgr are immutable after initialization

	// Set Content-Type header before any writes
	w.Header().Set("Content-Type", "application/json")

	// Check if blockchain is initialized
	if s.blockchain == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(HealthResponse{
			Status:  "initializing",
			Syncing: true,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode ready response: %v\n", err)
		}
		return
	}

	// Get current state
	best := s.blockchain.BestSnapshot()
	peers := 0
	syncing := false
	if s.peerMgr != nil {
		peers = s.peerMgr.GetConnectedPeers()
		syncing = s.peerMgr.IsSyncing()
	}

	// If syncing, return 503 Service Unavailable
	if syncing {
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(HealthResponse{
			Status:      "syncing",
			BlockHeight: best.Height,
			Peers:       peers,
			Syncing:     true,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode ready response: %v\n", err)
		}
		return
	}

	// Ready - sync is complete
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(HealthResponse{
		Status:      "ready",
		BlockHeight: best.Height,
		Peers:       peers,
		Syncing:     false,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode ready response: %v\n", err)
	}
}
