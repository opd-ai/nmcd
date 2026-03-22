package rpc

import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/internal/logging"
	"github.com/opd-ai/nmcd/metrics"
	"github.com/opd-ai/nmcd/namedb"
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
	rateLimiter    *rateLimiter
	maxRequestSize int64
	logger         *logging.Logger
	mu             sync.RWMutex
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
func successResponse(id interface{}, result interface{}) *Response {
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

// validateValueSize validates that a name value does not exceed 1023 bytes.
func validateValueSize(value string, reqID interface{}) *Response {
	if len(value) > 1023 {
		return errorResponse(reqID, -5, fmt.Sprintf("Value too large: %d bytes (max 1023)", len(value)))
	}
	return nil
}

// broadcastAndRespond broadcasts a transaction and returns a success or error response.
func (s *Server) broadcastAndRespond(tx *wire.MsgTx, reqID interface{}, result map[string]interface{}) *Response {
	if err := s.peerMgr.BroadcastTx(tx); err != nil {
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

	s := &Server{
		blockchain:     cfg.Blockchain,
		peerMgr:        cfg.PeerMgr,
		wallet:         cfg.Wallet,
		listener:       listener,
		rpcUser:        cfg.RPCUser,
		rpcPassword:    cfg.RPCPassword,
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

// Stop stops the RPC server
func (s *Server) Stop() error {
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

	return serverErr
}

// Close closes the RPC server (alias for Stop for compatibility)
func (s *Server) Close() error {
	return s.Stop()
}

// checkAuth validates HTTP Basic Authentication credentials.
// Returns true if the request contains valid credentials matching
// the configured rpcUser and rpcPassword.
// Uses constant-time comparison to prevent timing attacks.
func (s *Server) checkAuth(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(s.rpcUser))
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(s.rpcPassword))
	return userMatch == 1 && passMatch == 1
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
				// Log the panic with full context
				s.logger.Error("panic recovered in HTTP handler",
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
	// Set security headers
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")

	// Only allow POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Require a known, positive Content-Length and enforce the maximum size
	if r.ContentLength <= 0 {
		http.Error(w, "Content-Length required", http.StatusLengthRequired)
		return
	}
	if r.ContentLength > s.maxRequestSize {
		http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Extract IP and apply rate limiting
	ip := extractIP(r.RemoteAddr)
	if !s.rateLimiter.allow(ip) {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Check authentication if both credentials are configured.
	// Both rpcUser and rpcPassword must be set for authentication to be enforced.
	if s.rpcUser != "" && s.rpcPassword != "" {
		if !s.checkAuth(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="nmcd RPC"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Limit reader to maxRequestSize to prevent memory exhaustion
	limitedReader := http.MaxBytesReader(w, r.Body, s.maxRequestSize)
	defer limitedReader.Close()

	var req Request
	if err := json.NewDecoder(limitedReader).Decode(&req); err != nil {
		s.writeError(w, &req, -32700, "Parse error")
		return
	}

	resp := s.processRequest(&req)

	// Log errors for internal tracking (don't expose details to clients)
	if resp.Error != nil && s.logger != nil {
		s.logger.Warn("rpc request error",
			"method", req.Method,
			"error_code", resp.Error.Code,
			"error_message", resp.Error.Message,
		)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		// Log encoding error with structured logging
		if s.logger != nil {
			s.logger.Error("failed to encode JSON-RPC response",
				"error", err,
				"method", req.Method,
			)
		}
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
	if s.blockchain == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32603,
				Message: "Blockchain not initialized",
			},
			ID: req.ID,
		}
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
		"difficulty":  best.Bits,
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  info,
		ID:      req.ID,
	}
}

// getBlockCount returns the current block count
func (s *Server) getBlockCount(req *Request) *Response {
	if s.blockchain == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32603,
				Message: "Blockchain not initialized",
			},
			ID: req.ID,
		}
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
	if s.blockchain == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32603,
				Message: "Blockchain not initialized",
			},
			ID: req.ID,
		}
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

// nameShow returns information about a name
func (s *Server) nameShow(req *Request) *Response {
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) == 0 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params",
			},
			ID: req.ID,
		}
	}

	name := params[0]
	record, err := s.blockchain.GetName(name)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Name not found: %s", name),
			},
			ID: req.ID,
		}
	}

	result := map[string]interface{}{
		"name":       record.Name,
		"value":      record.Value,
		"txid":       record.TxHash.String(),
		"height":     record.Height,
		"expires_in": record.ExpiresAt - s.blockchain.BestSnapshot().Height,
		"address":    record.Address,
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// nameUpdate updates a name's value. This creates a NAME_UPDATE transaction.
// Parameters: ["name", "value"] or ["name", "value", "address"]
// The name must exist and not be expired. If no address is specified, the
// name will be updated to remain at its current address.
//
// Returns the transaction hex that can be broadcast to the network.
// Note: This creates an unsigned transaction template. For full transaction
// signing and broadcasting, the wallet must have the private key for the
// address that owns the name.
func (s *Server) nameUpdate(req *Request) *Response {
	if errResp := s.requireWallet(req.ID); errResp != nil {
		return errResp
	}

	params, errResp := parseStringParams(req.Params, req.ID, 2, "[\"name\", \"value\"] or [\"name\", \"value\", \"address\"]")
	if errResp != nil {
		return errResp
	}

	name := params[0]
	newValue := params[1]

	destAddress, errResp := s.parseOptionalDestAddress(params, req.ID)
	if errResp != nil {
		return errResp
	}

	if errResp := validateNameLength(name, req.ID); errResp != nil {
		return errResp
	}
	if errResp := validateValueSize(newValue, req.ID); errResp != nil {
		return errResp
	}

	record, errResp := s.lookupActiveNameRecord(name, req.ID)
	if errResp != nil {
		return errResp
	}

	if errResp := s.verifyNameOwnership(record, req.ID); errResp != nil {
		return errResp
	}

	utxos, nameUtxoIndex, errResp := s.collectNameUpdateUTXOs(name, record, req.ID)
	if errResp != nil {
		return errResp
	}

	feeRate := int64(1)
	tx, err := s.wallet.CreateNameUpdateTx(name, newValue, utxos, nameUtxoIndex, feeRate, destAddress)
	if err != nil {
		return errorResponse(req.ID, -1, fmt.Sprintf("Failed to create transaction: %v", err))
	}

	result := map[string]interface{}{
		"txid":   tx.TxHash().String(),
		"name":   name,
		"value":  newValue,
		"status": "broadcasted",
	}
	if destAddress != nil {
		result["address"] = destAddress.EncodeAddress()
	}

	return s.broadcastAndRespond(tx, req.ID, result)
}

// parseOptionalDestAddress parses an optional P2PKH destination address from the third parameter.
func (s *Server) parseOptionalDestAddress(params []string, reqID interface{}) (btcutil.Address, *Response) {
	if len(params) < 3 || params[2] == "" {
		return nil, nil
	}
	addr, err := btcutil.DecodeAddress(params[2], s.blockchain.ChainParams())
	if err != nil {
		return nil, errorResponse(reqID, -5, fmt.Sprintf("Invalid destination address: %v", err))
	}
	if _, ok := addr.(*btcutil.AddressPubKeyHash); !ok {
		return nil, errorResponse(reqID, -5, fmt.Sprintf("Destination address must be P2PKH, got: %T", addr))
	}
	return addr, nil
}

// lookupActiveNameRecord retrieves a name record and verifies it is not expired.
func (s *Server) lookupActiveNameRecord(name string, reqID interface{}) (*namedb.NameRecord, *Response) {
	record, err := s.blockchain.GetName(name)
	if err != nil {
		return nil, errorResponse(reqID, -4, fmt.Sprintf("Name not found: %s", name))
	}
	bestHeight := s.blockchain.BestSnapshot().Height
	if record.ExpiresAt <= bestHeight {
		return nil, errorResponse(reqID, -4, fmt.Sprintf("Name expired at block %d (current: %d)", record.ExpiresAt, bestHeight))
	}
	return record, nil
}

// verifyNameOwnership checks that the wallet has the private key for the name's current owner.
func (s *Server) verifyNameOwnership(record *namedb.NameRecord, reqID interface{}) *Response {
	if !s.wallet.HasKey(record.Address) {
		return errorResponse(reqID, -13, fmt.Sprintf("Wallet does not have the private key for address: %s", record.Address))
	}
	return nil
}

// collectNameUpdateUTXOs retrieves and converts the UTXOs needed for a NAME_UPDATE transaction.
func (s *Server) collectNameUpdateUTXOs(name string, record *namedb.NameRecord, reqID interface{}) ([]wallet.UTXO, int, *Response) {
	nameUTXO, err := s.blockchain.GetNameUTXO(name)
	if err != nil {
		return nil, 0, errorResponse(reqID, -1, fmt.Sprintf("Failed to get name UTXO: %v", err))
	}

	walletUTXOs, err := s.blockchain.GetUTXOsForAddress(record.Address)
	if err != nil {
		return nil, 0, errorResponse(reqID, -1, fmt.Sprintf("Failed to get wallet UTXOs: %v", err))
	}

	utxos, nameUtxoIndex := convertAndFindNameUTXO(walletUTXOs, &nameUTXO.TxHash, nameUTXO.OutIndex)
	if nameUtxoIndex == -1 {
		return nil, 0, errorResponse(reqID, -1, "Name UTXO not found in wallet UTXOs")
	}
	return utxos, nameUtxoIndex, nil
}

// convertAndFindNameUTXO converts namedb UTXOs to wallet UTXOs and locates the name UTXO index.
func convertAndFindNameUTXO(dbUTXOs []*namedb.UTXO, nameHash *chainhash.Hash, nameOutIndex uint32) ([]wallet.UTXO, int) {
	var utxos []wallet.UTXO
	nameUtxoIndex := -1
	for _, dbUTXO := range dbUTXOs {
		wUtxo := wallet.UTXO{
			TxHash:   dbUTXO.TxHash,
			Vout:     dbUTXO.OutIndex,
			Value:    dbUTXO.Value,
			PkScript: dbUTXO.PkScript,
			Address:  dbUTXO.Address,
		}
		if dbUTXO.TxHash.IsEqual(nameHash) && dbUTXO.OutIndex == nameOutIndex {
			nameUtxoIndex = len(utxos)
		}
		utxos = append(utxos, wUtxo)
	}
	return utxos, nameUtxoIndex
}

// getWalletAddressAndUTXOs is a helper function that retrieves the wallet address and UTXOs
// for funding name operation transactions. This reduces code duplication between nameNew
// and nameFirstUpdate methods.
//
// Returns:
//   - btcutil.Address: The decoded wallet address
//   - []wallet.UTXO: The converted wallet UTXOs ready for transaction creation
//   - *Response: Error response if any step fails, nil on success
func (s *Server) getWalletAddressAndUTXOs(reqID interface{}) (btcutil.Address, []wallet.UTXO, *Response) {
	// Get a wallet address to own the name
	addresses := s.wallet.GetAddresses()
	if len(addresses) == 0 {
		return nil, nil, &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "No addresses in wallet. Create an address first using getnewaddress.",
			},
			ID: reqID,
		}
	}
	ownerAddress := addresses[0] // Use the first address

	// Decode the address
	addr, err := btcutil.DecodeAddress(ownerAddress, s.blockchain.ChainParams())
	if err != nil {
		return nil, nil, &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Invalid address: %v", err),
			},
			ID: reqID,
		}
	}

	// Get wallet UTXOs for funding
	walletUTXOs, err := s.blockchain.GetUTXOsForAddress(ownerAddress)
	if err != nil {
		return nil, nil, &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to get wallet UTXOs: %v", err),
			},
			ID: reqID,
		}
	}

	if len(walletUTXOs) == 0 {
		return nil, nil, &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -6,
				Message: "Insufficient funds. No UTXOs available in wallet.",
			},
			ID: reqID,
		}
	}

	// Convert namedb UTXOs to wallet UTXOs
	var utxos []wallet.UTXO
	for _, dbUTXO := range walletUTXOs {
		wUtxo := wallet.UTXO{
			TxHash:   dbUTXO.TxHash,
			Vout:     dbUTXO.OutIndex,
			Value:    dbUTXO.Value,
			PkScript: dbUTXO.PkScript,
			Address:  dbUTXO.Address,
		}
		utxos = append(utxos, wUtxo)
	}

	return addr, utxos, nil
}

// nameNew creates a NAME_NEW transaction for pre-registering a name commitment.
// This is the first step in the two-phase name registration process to prevent front-running.
//
// Parameters:
//   - name: Name to be registered (e.g., "d/example")
//
// Returns a JSON object with:
//   - txid: Transaction ID of the NAME_NEW transaction
//   - name: The name being registered
//   - rand: Hex-encoded random salt (MUST be saved for NAME_FIRSTUPDATE)
//   - status: "broadcasted" indicating transaction is in mempool
func (s *Server) nameNew(req *Request) *Response {
	if errResp := s.requireWallet(req.ID); errResp != nil {
		return errResp
	}

	params, errResp := parseStringParams(req.Params, req.ID, 1, "[\"name\"]")
	if errResp != nil {
		return errResp
	}

	name := params[0]
	if errResp := validateNameLength(name, req.ID); errResp != nil {
		return errResp
	}

	if errResp := s.checkNameNotActive(name, req.ID); errResp != nil {
		return errResp
	}

	addr, utxos, errResp := s.getWalletAddressAndUTXOs(req.ID)
	if errResp != nil {
		return errResp
	}

	randBytes, err := wallet.GenerateRand()
	if err != nil {
		return errorResponse(req.ID, -1, fmt.Sprintf("Failed to generate random salt for NAME_NEW: %v", err))
	}

	feeRate := int64(1)
	tx, randBytesReturned, err := s.wallet.CreateNameNewTx(randBytes, name, utxos, feeRate, addr)
	if err != nil {
		return errorResponse(req.ID, -1, fmt.Sprintf("Failed to create NAME_NEW transaction: %v", err))
	}

	result := map[string]interface{}{
		"txid":   tx.TxHash().String(),
		"name":   name,
		"rand":   fmt.Sprintf("%x", randBytesReturned),
		"status": "broadcasted",
	}
	return s.broadcastAndRespond(tx, req.ID, result)
}

// checkNameNotActive verifies a name doesn't already exist as an unexpired registration.
func (s *Server) checkNameNotActive(name string, reqID interface{}) *Response {
	existingRecord, err := s.blockchain.GetName(name)
	if err != nil {
		return nil // Name doesn't exist - that's what we want
	}
	bestHeight := s.blockchain.BestSnapshot().Height
	if existingRecord.ExpiresAt > bestHeight {
		return errorResponse(reqID, -25, fmt.Sprintf("Name already exists and is not expired (expires at block %d, current: %d)", existingRecord.ExpiresAt, bestHeight))
	}
	return nil
}

// nameFirstUpdate creates a NAME_FIRSTUPDATE transaction to complete name registration.
// This is the second step in the two-phase registration process. Must be called at least
// 12 blocks after the NAME_NEW transaction.
//
// Parameters:
//   - name: Name being registered (must match the NAME_NEW commitment)
//   - rand: Hex-encoded random bytes from the NAME_NEW transaction
//   - value: Initial value for the name (max 1023 bytes)
//
// Returns a JSON object with:
//   - txid: Transaction ID of the NAME_FIRSTUPDATE transaction
//   - name: The name being registered
//   - value: The initial value
//   - status: "broadcasted" indicating transaction is in mempool
func (s *Server) nameFirstUpdate(req *Request) *Response {
	if errResp := s.requireWallet(req.ID); errResp != nil {
		return errResp
	}

	params, errResp := parseStringParams(req.Params, req.ID, 3, "[\"name\", \"rand\", \"value\"]")
	if errResp != nil {
		return errResp
	}

	name, randHex, value := params[0], params[1], params[2]

	if errResp := validateNameLength(name, req.ID); errResp != nil {
		return errResp
	}
	if errResp := validateValueSize(value, req.ID); errResp != nil {
		return errResp
	}

	randBytes, err := hex.DecodeString(randHex)
	if err != nil {
		return errorResponse(req.ID, -5, fmt.Sprintf("Invalid rand hex: %v", err))
	}

	if errResp := s.validateNameNewCommitment(randBytes, name, req.ID); errResp != nil {
		return errResp
	}

	addr, utxos, errResp := s.getWalletAddressAndUTXOs(req.ID)
	if errResp != nil {
		return errResp
	}

	nameNewUtxoIndex := findNameNewUTXOIndex(utxos)

	feeRate := int64(1)
	tx, err := s.wallet.CreateNameFirstUpdateTx(name, randHex, value, utxos, nameNewUtxoIndex, feeRate, addr)
	if err != nil {
		return errorResponse(req.ID, -1, fmt.Sprintf("Failed to create NAME_FIRSTUPDATE transaction: %v", err))
	}

	result := map[string]interface{}{
		"txid":   tx.TxHash().String(),
		"name":   name,
		"value":  value,
		"status": "broadcasted",
	}
	return s.broadcastAndRespond(tx, req.ID, result)
}

// validateNameNewCommitment validates that a NAME_NEW commitment exists and is within the valid window.
func (s *Server) validateNameNewCommitment(randBytes []byte, name string, reqID interface{}) *Response {
	commitHash := wallet.ComputeNameNewHash(randBytes, name, s.blockchain.ChainParams())
	nameNewRecord, err := s.blockchain.GetNameDB().GetNameNew(commitHash)
	if err != nil {
		return errorResponse(reqID, -25, "NAME_NEW commitment not found. You must call name_new first and wait for confirmation.")
	}

	bestHeight := s.blockchain.BestSnapshot().Height
	blocksSinceNameNew := bestHeight - nameNewRecord.Height
	if blocksSinceNameNew < 12 {
		return errorResponse(reqID, -25, fmt.Sprintf("NAME_NEW not confirmed enough. Need 12 blocks, only %d blocks have passed.", blocksSinceNameNew))
	}
	if blocksSinceNameNew > 36000 {
		return errorResponse(reqID, -25, fmt.Sprintf("NAME_NEW commitment expired. Maximum window is 36,000 blocks, but %d blocks have passed.", blocksSinceNameNew))
	}
	return nil
}

// findNameNewUTXOIndex locates the NAME_NEW UTXO index by checking for OP_NAME_NEW opcode in scripts.
// Falls back to index 0 if no NAME_NEW UTXO is found.
func findNameNewUTXOIndex(utxos []wallet.UTXO) int {
	for i, utxo := range utxos {
		if len(utxo.PkScript) > 22 && utxo.PkScript[0] == opNameNew {
			return i
		}
	}
	return 0
}

// nameList returns all names in the database
func (s *Server) nameList(req *Request) *Response {
	names, err := s.blockchain.ListNames()
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to list names: %v", err),
			},
			ID: req.ID,
		}
	}

	// Format names for response
	result := make([]map[string]interface{}, len(names))
	bestHeight := s.blockchain.BestSnapshot().Height
	for i, record := range names {
		result[i] = map[string]interface{}{
			"name":       record.Name,
			"value":      record.Value,
			"txid":       record.TxHash.String(),
			"height":     record.Height,
			"expires_in": record.ExpiresAt - bestHeight,
			"address":    record.Address,
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// nameHistory returns the history of a name, including all past operations.
// Each entry in the history represents a NAME_FIRSTUPDATE or NAME_UPDATE operation.
func (s *Server) nameHistory(req *Request) *Response {
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) == 0 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params: expected ['name']",
			},
			ID: req.ID,
		}
	}

	name := params[0]
	history, err := s.blockchain.GetNameHistory(name)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to get name history: %v", err),
			},
			ID: req.ID,
		}
	}

	// Format history for response.
	// Historical records use 'expires_at' (absolute block height) instead of 'expires_in'
	// because these are past snapshots where calculating blocks remaining would be misleading.
	result := make([]map[string]interface{}, len(history))
	for i, record := range history {
		result[i] = map[string]interface{}{
			"name":       record.Name,
			"value":      record.Value,
			"txid":       record.TxHash.String(),
			"height":     record.Height,
			"expires_at": record.ExpiresAt,
			"address":    record.Address,
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// nameScan scans names with prefix matching and pagination.
// Matches Namecoin Core's name_scan RPC.
// Parameters: [start] [count] where start is the prefix and count is max results (default 500)
func (s *Server) nameScan(req *Request) *Response {
	if errResp := s.requireBlockchain(req.ID); errResp != nil {
		return errResp
	}

	start, count, errResp := parseNameScanParams(req.Params, req.ID)
	if errResp != nil {
		return errResp
	}

	names, err := s.blockchain.ScanNames(start, count)
	if err != nil {
		return errorResponse(req.ID, -32603, fmt.Sprintf("Failed to scan names: %v", err))
	}

	return successResponse(req.ID, formatNameRecords(names, s.blockchain.BestSnapshot().Height))
}

// parseNameScanParams extracts start prefix and count from name_scan parameters.
func parseNameScanParams(rawParams json.RawMessage, reqID interface{}) (string, int, *Response) {
	start := ""
	count := 500

	var params []interface{}
	if err := json.Unmarshal(rawParams, &params); err != nil || len(params) == 0 {
		return start, count, nil
	}

	if len(params) > 0 {
		startStr, ok := params[0].(string)
		if !ok {
			return "", 0, errorResponse(reqID, -32602, "start must be a string")
		}
		start = startStr
	}

	if len(params) > 1 {
		countFloat, ok := params[1].(float64)
		if !ok {
			return "", 0, errorResponse(reqID, -32602, "count must be a number")
		}
		count = int(countFloat)
		if count <= 0 || count > 10000 {
			return "", 0, errorResponse(reqID, -32602, "count must be between 1 and 10000")
		}
	}

	return start, count, nil
}

// formatNameRecords converts name records to JSON-RPC result format.
func formatNameRecords(names []*namedb.NameRecord, currentHeight int32) []map[string]interface{} {
	result := make([]map[string]interface{}, len(names))
	for i, record := range names {
		result[i] = map[string]interface{}{
			"name":       record.Name,
			"value":      record.Value,
			"txid":       record.TxHash.String(),
			"height":     record.Height,
			"expires_in": record.ExpiresAt - currentHeight,
			"address":    record.Address,
		}
	}
	return result
}

// namePending returns pending name operations from the mempool.
// Matches Namecoin Core's name_pending RPC.
// Parameters: [] or ["name"] where name is an optional filter
func (s *Server) namePending(req *Request) *Response {
	result := []map[string]interface{}{}

	// Get mempool from peer manager
	if s.peerMgr == nil {
		// No peer manager means no mempool - return empty list
		return &Response{
			Jsonrpc: "2.0",
			Result:  result,
			ID:      req.ID,
		}
	}

	mempool := s.peerMgr.GetMempool()
	if mempool == nil {
		return &Response{
			Jsonrpc: "2.0",
			Result:  result,
			ID:      req.ID,
		}
	}

	// Parse optional name filter from params
	var nameFilter string
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err == nil && len(params) > 0 {
		if name, ok := params[0].(string); ok {
			nameFilter = name
		}
	}

	// Get all transactions from mempool and parse name operations
	mempoolTxs := mempool.GetAll()
	for _, tx := range mempoolTxs {
		nameOps := chain.ParseNameOperationsFromTx(tx)
		for _, op := range nameOps {
			// Apply name filter if specified
			if nameFilter != "" && op.Name != nameFilter {
				continue
			}

			// Build result object matching Namecoin Core format
			opResult := map[string]interface{}{
				"name":   op.Name,
				"txid":   op.TxHash.String(),
				"vout":   op.OutputIndex,
				"op":     op.OpType.String(),
				"ismine": false, // Would require wallet lookup
			}

			// Add value for NAME_FIRSTUPDATE and NAME_UPDATE
			// NAME_NEW operations only contain a hash commitment, not a value
			if op.OpType != namedb.NameNew {
				opResult["value"] = op.Value
			}

			result = append(result, opResult)
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// getNewAddress generates a new address in the wallet and returns it.
// This method creates a new key pair and persists it to the wallet file.
func (s *Server) getNewAddress(req *Request) *Response {
	if s.wallet == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "Wallet not initialized. Start the node with wallet enabled.",
			},
			ID: req.ID,
		}
	}

	address, err := s.wallet.GenerateKey()
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to generate address: %v", err),
			},
			ID: req.ID,
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  address,
		ID:      req.ID,
	}
}

// listAddresses returns all addresses in the wallet.
// Returns an array of address strings currently stored in the wallet.
func (s *Server) listAddresses(req *Request) *Response {
	if s.wallet == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "Wallet not initialized. Start the node with wallet enabled.",
			},
			ID: req.ID,
		}
	}

	addresses := s.wallet.GetAddresses()

	return &Response{
		Jsonrpc: "2.0",
		Result:  addresses,
		ID:      req.ID,
	}
}

// walletPassphrase unlocks the wallet with a password for a specified time.
// Parameters: [password, timeout]
//   - password (string, required): The wallet password
//   - timeout (int, optional): Time in seconds to keep wallet unlocked (default: 60)
//
// Returns: null on success
// Errors:
//   - -1: Wallet not initialized
//   - -13: Wallet is not encrypted
//   - -14: Incorrect password
func (s *Server) walletPassphrase(req *Request) *Response {
	if errResp := s.requireWallet(req.ID); errResp != nil {
		return errResp
	}

	params, errResp := parseInterfaceParams(req.Params, req.ID, 1, "[password] or [password, timeout]")
	if errResp != nil {
		return errResp
	}

	password, ok := params[0].(string)
	if !ok {
		return errorResponse(req.ID, -32602, "Invalid password parameter: expected string")
	}

	timeout, errResp := parsePassphraseTimeout(params, req.ID)
	if errResp != nil {
		return errResp
	}

	if !s.wallet.IsEncrypted() {
		return errorResponse(req.ID, -13, "Wallet is not encrypted")
	}

	if err := s.wallet.Unlock(password); err != nil {
		return errorResponse(req.ID, -14, fmt.Sprintf("Failed to unlock wallet: %v", err))
	}

	time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		s.wallet.Lock()
	})

	return successResponse(req.ID, nil)
}

// parsePassphraseTimeout extracts the optional timeout parameter (default 60 seconds).
func parsePassphraseTimeout(params []interface{}, reqID interface{}) (int, *Response) {
	if len(params) <= 1 {
		return 60, nil
	}
	timeoutFloat, ok := params[1].(float64)
	if !ok {
		return 0, errorResponse(reqID, -32602, "Invalid timeout parameter: expected integer")
	}
	timeout := int(timeoutFloat)
	if timeout <= 0 {
		return 0, errorResponse(reqID, -32602, "Invalid timeout: must be positive")
	}
	return timeout, nil
}

// walletLock locks the wallet, removing keys from memory.
// Parameters: none
// Returns: null on success
// Errors:
//   - -1: Wallet not initialized
//   - -13: Wallet is not encrypted
func (s *Server) walletLock(req *Request) *Response {
	if s.wallet == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "Wallet not initialized. Start the node with wallet enabled.",
			},
			ID: req.ID,
		}
	}

	if !s.wallet.IsEncrypted() {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -13,
				Message: "Wallet is not encrypted",
			},
			ID: req.ID,
		}
	}

	if err := s.wallet.Lock(); err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to lock wallet: %v", err),
			},
			ID: req.ID,
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  nil,
		ID:      req.ID,
	}
}

// encryptWallet encrypts the wallet with a password.
// Parameters: [password]
//   - password (string, required): The password to encrypt the wallet with
//
// Returns: null on success
// Errors:
//   - -1: Wallet not initialized or already encrypted
//   - -8: Invalid password (too weak)
func (s *Server) encryptWallet(req *Request) *Response {
	if s.wallet == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "Wallet not initialized. Start the node with wallet enabled.",
			},
			ID: req.ID,
		}
	}

	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) == 0 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params: expected [password]",
			},
			ID: req.ID,
		}
	}

	password, ok := params[0].(string)
	if !ok {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid password parameter: expected string",
			},
			ID: req.ID,
		}
	}

	// Encrypt wallet
	if err := s.wallet.EncryptWallet(password); err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -8,
				Message: fmt.Sprintf("Failed to encrypt wallet: %v", err),
			},
			ID: req.ID,
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  "Wallet encrypted successfully. Please backup your wallet and remember your password.",
		ID:      req.ID,
	}
}

// getBalance returns the total balance for all wallet addresses.
// Parameters: [] (no parameters required)
// Returns: balance in NMC as a float
func (s *Server) getBalance(req *Request) *Response {
	if s.wallet == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "Wallet not initialized. Start the node with wallet enabled.",
			},
			ID: req.ID,
		}
	}

	if s.blockchain == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32603,
				Message: "Blockchain not initialized",
			},
			ID: req.ID,
		}
	}

	// Get all wallet addresses
	addresses := s.wallet.GetAddresses()
	if len(addresses) == 0 {
		return &Response{
			Jsonrpc: "2.0",
			Result:  0.0,
			ID:      req.ID,
		}
	}

	// Sum up UTXOs for all addresses
	var totalSatoshis int64
	for _, addr := range addresses {
		utxos, err := s.blockchain.GetUTXOsForAddress(addr)
		if err != nil {
			continue // Skip addresses with errors
		}
		for _, utxo := range utxos {
			totalSatoshis += utxo.Value
		}
	}

	// Convert satoshis to NMC (1 NMC = 100,000,000 satoshis)
	balance := float64(totalSatoshis) / 1e8

	return &Response{
		Jsonrpc: "2.0",
		Result:  balance,
		ID:      req.ID,
	}
}

// listUnspent returns all unspent transaction outputs for wallet addresses.
// Parameters: [] or [minconf] or [minconf, maxconf] or [minconf, maxconf, [addresses]]
//   - minconf (int, optional): Minimum confirmations (default: 1)
//   - maxconf (int, optional): Maximum confirmations (default: 9999999)
//   - addresses (array, optional): Filter by addresses (default: all wallet addresses)
//
// Returns array of UTXO objects with txid, vout, address, amount, confirmations, etc.
func (s *Server) listUnspent(req *Request) *Response {
	if err := s.validateListUnspentState(); err != nil {
		return err
	}

	minConf, maxConf, filterAddrs := s.parseListUnspentParams(req.Params)
	addresses := s.resolveTargetAddresses(filterAddrs)
	utxos := s.collectFilteredUTXOs(addresses, minConf, maxConf)

	return &Response{
		Jsonrpc: "2.0",
		Result:  utxos,
		ID:      req.ID,
	}
}

// validateListUnspentState checks wallet and blockchain availability.
func (s *Server) validateListUnspentState() *Response {
	if s.wallet == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "Wallet not initialized. Start the node with wallet enabled.",
			},
		}
	}
	if s.blockchain == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32603,
				Message: "Blockchain not initialized",
			},
		}
	}
	return nil
}

// parseListUnspentParams extracts minconf, maxconf, and address filters from request params.
func (s *Server) parseListUnspentParams(rawParams json.RawMessage) (minConf, maxConf int, filterAddresses []string) {
	minConf = 1
	maxConf = 9999999

	var params []interface{}
	if err := json.Unmarshal(rawParams, &params); err != nil || len(params) == 0 {
		return minConf, maxConf, filterAddresses
	}

	if minC, ok := params[0].(float64); ok {
		minConf = int(minC)
	}
	if len(params) > 1 {
		if maxC, ok := params[1].(float64); ok {
			maxConf = int(maxC)
		}
	}
	if len(params) > 2 {
		filterAddresses = s.extractAddressFilter(params[2])
	}
	return minConf, maxConf, filterAddresses
}

// extractAddressFilter parses the address filter parameter.
func (s *Server) extractAddressFilter(param interface{}) []string {
	var addresses []string
	if addrs, ok := param.([]interface{}); ok {
		for _, a := range addrs {
			if addr, ok := a.(string); ok {
				addresses = append(addresses, addr)
			}
		}
	}
	return addresses
}

// resolveTargetAddresses returns the addresses to query (filter or all wallet addresses).
func (s *Server) resolveTargetAddresses(filterAddresses []string) []string {
	if len(filterAddresses) > 0 {
		return filterAddresses
	}
	return s.wallet.GetAddresses()
}

// collectFilteredUTXOs gathers UTXOs for addresses, applying confirmation filters.
func (s *Server) collectFilteredUTXOs(addresses []string, minConf, maxConf int) []map[string]interface{} {
	bestHeight := s.blockchain.BestSnapshot().Height
	var result []map[string]interface{}

	for _, addr := range addresses {
		utxos, err := s.blockchain.GetUTXOsForAddress(addr)
		if err != nil {
			continue
		}

		for _, utxo := range utxos {
			if utxoObj := s.buildUTXOResult(utxo, bestHeight, minConf, maxConf); utxoObj != nil {
				result = append(result, utxoObj)
			}
		}
	}

	if result == nil {
		result = []map[string]interface{}{}
	}
	return result
}

// buildUTXOResult creates a UTXO result object if it passes confirmation filters.
func (s *Server) buildUTXOResult(utxo *namedb.UTXO, bestHeight int32, minConf, maxConf int) map[string]interface{} {
	confirmations := s.calculateConfirmations(utxo.Height, bestHeight)
	if confirmations < minConf || confirmations > maxConf {
		return nil
	}

	result := map[string]interface{}{
		"txid":          utxo.TxHash.String(),
		"vout":          utxo.OutIndex,
		"address":       utxo.Address,
		"amount":        float64(utxo.Value) / 1e8,
		"confirmations": confirmations,
		"spendable":     true,
		"solvable":      true,
		"safe":          true,
	}

	if len(utxo.PkScript) > 0 {
		result["scriptPubKey"] = hex.EncodeToString(utxo.PkScript)
	}

	return result
}

// calculateConfirmations computes confirmations for a UTXO at a given height.
func (s *Server) calculateConfirmations(utxoHeight, bestHeight int32) int {
	if utxoHeight > 0 {
		return int(bestHeight - utxoHeight + 1)
	}
	return 0
}

// getBlock returns a block by hash with optional verbose mode.
// Parameters: [blockhash] or [blockhash, verbose]
//   - blockhash (string, required): The block hash as hex string
//   - verbose (bool, optional): If false (default), returns hex-encoded block data.
//     If true, returns JSON object with block details.
func (s *Server) getBlock(req *Request) *Response {
	params, errResp := parseInterfaceParams(req.Params, req.ID, 1, "[blockhash] or [blockhash, verbose]")
	if errResp != nil {
		return errResp
	}

	hash, errResp := parseHashParam(params, 0, req.ID, "block hash")
	if errResp != nil {
		return errResp
	}

	verbose, errResp := parseVerboseParam(params, 1, req.ID)
	if errResp != nil {
		return errResp
	}

	if errResp := s.requireBlockchain(req.ID); errResp != nil {
		return errResp
	}

	block, err := s.blockchain.GetBlockByHash(hash)
	if err != nil {
		return errorResponse(req.ID, -5, fmt.Sprintf("Block not found: %v", err))
	}

	if !verbose {
		return s.serializeBlockHex(block, req.ID)
	}

	return successResponse(req.ID, s.buildVerboseBlockResult(block, hash))
}

// serializeBlockHex returns a hex-encoded block data response.
func (s *Server) serializeBlockHex(block *btcutil.Block, reqID interface{}) *Response {
	blockBytes, err := block.Bytes()
	if err != nil {
		return errorResponse(reqID, -1, fmt.Sprintf("Failed to serialize block: %v", err))
	}
	return successResponse(reqID, fmt.Sprintf("%x", blockBytes))
}

// buildVerboseBlockResult builds a verbose JSON result for a block.
func (s *Server) buildVerboseBlockResult(block *btcutil.Block, hash *chainhash.Hash) map[string]interface{} {
	msgBlock := block.MsgBlock()
	header := msgBlock.Header
	bestSnapshot := s.blockchain.BestSnapshot()

	height, err := s.blockchain.BlockHeightByHash(hash)
	if err != nil {
		height = -1
	}

	var confirmations int32
	if height >= 0 {
		confirmations = bestSnapshot.Height - height + 1
	}

	txs := make([]string, len(msgBlock.Transactions))
	for i, tx := range msgBlock.Transactions {
		txs[i] = tx.TxHash().String()
	}

	result := map[string]interface{}{
		"hash":              hash.String(),
		"confirmations":     confirmations,
		"height":            height,
		"version":           header.Version,
		"merkleroot":        header.MerkleRoot.String(),
		"time":              header.Timestamp.Unix(),
		"nonce":             header.Nonce,
		"bits":              fmt.Sprintf("%08x", header.Bits),
		"difficulty":        getDifficultyRatio(header.Bits, s.blockchain.ChainParams()),
		"previousblockhash": header.PrevBlock.String(),
		"tx":                txs,
	}

	if height >= 0 && height < bestSnapshot.Height {
		nextHash, err := s.blockchain.BlockHashByHeight(height + 1)
		if err == nil {
			result["nextblockhash"] = nextHash.String()
		}
	}

	return result
}

// getBlockHash returns the block hash for a given height.
// Parameters: [height]
// - height (int): The block height
func (s *Server) getBlockHash(req *Request) *Response {
	params, errResp := parseInterfaceParams(req.Params, req.ID, 1, "[height]")
	if errResp != nil {
		return errResp
	}

	height, errResp := parseBlockHeightParam(params[0], req.ID)
	if errResp != nil {
		return errResp
	}

	if errResp := s.requireBlockchain(req.ID); errResp != nil {
		return errResp
	}

	hash, err := s.blockchain.BlockHashByHeight(height)
	if err != nil {
		return errorResponse(req.ID, -8, fmt.Sprintf("Block height out of range: %v", err))
	}

	return successResponse(req.ID, hash.String())
}

// parseBlockHeightParam parses and validates a block height from a JSON parameter.
func parseBlockHeightParam(param interface{}, reqID interface{}) (int32, *Response) {
	var height int32
	switch v := param.(type) {
	case float64:
		if v > 2147483647 || v < -2147483648 {
			return 0, errorResponse(reqID, -32602, fmt.Sprintf("Invalid params: height out of int32 range: %v", v))
		}
		height = int32(v)
	case int:
		height = int32(v)
	case int32:
		height = v
	default:
		return 0, errorResponse(reqID, -32602, fmt.Sprintf("Invalid params: height must be a number, got %T", param))
	}
	if height < 0 {
		return 0, errorResponse(reqID, -8, "Block height out of range")
	}
	return height, nil
}

// getRawTransaction returns the raw transaction data.
// Parameters: [txid] or [txid, verbose]
//   - txid (string, required): The transaction ID
//   - verbose (bool, optional): If false (default), returns hex-encoded transaction.
//     If true, returns JSON object with transaction details.
//
// Note: This implementation searches through recent blocks to find transactions.
// It does not currently support mempool transactions or a full transaction index.
func (s *Server) getRawTransaction(req *Request) *Response {
	params, errResp := parseInterfaceParams(req.Params, req.ID, 1, "[txid] or [txid, verbose]")
	if errResp != nil {
		return errResp
	}

	txid, errResp := parseHashParam(params, 0, req.ID, "transaction ID")
	if errResp != nil {
		return errResp
	}

	verbose, errResp := parseVerboseParam(params, 1, req.ID)
	if errResp != nil {
		return errResp
	}

	if errResp := s.requireBlockchain(req.ID); errResp != nil {
		return errResp
	}

	foundTx, foundBlockHash, foundHeight, bestHeight, errResp := s.searchTransaction(txid, req.ID)
	if errResp != nil {
		return errResp
	}

	if !verbose {
		return s.serializeTransactionHex(foundTx, req.ID)
	}

	return successResponse(req.ID, buildVerboseTransactionResult(foundTx, foundBlockHash, foundHeight, bestHeight))
}

// searchTransaction searches recent blocks for a transaction by its hash.
// Returns the transaction, block hash, block height, best height, and an error response if not found.
func (s *Server) searchTransaction(txid *chainhash.Hash, reqID interface{}) (*wire.MsgTx, *chainhash.Hash, int32, int32, *Response) {
	bestHeight := s.blockchain.BestSnapshot().Height

	startHeight := bestHeight - 1000
	if startHeight < 0 {
		startHeight = 0
	}

	for height := bestHeight; height >= startHeight; height-- {
		hash, err := s.blockchain.BlockHashByHeight(height)
		if err != nil {
			continue
		}
		block, err := s.blockchain.GetBlockByHash(hash)
		if err != nil {
			continue
		}
		for _, tx := range block.MsgBlock().Transactions {
			txHash := tx.TxHash()
			if txHash.IsEqual(txid) {
				return tx, hash, height, bestHeight, nil
			}
		}
	}

	return nil, nil, 0, 0, errorResponse(reqID, -5, fmt.Sprintf("Transaction not found: %s", txid.String()))
}

// serializeTransactionHex serializes a transaction to hex-encoded string response.
func (s *Server) serializeTransactionHex(tx *wire.MsgTx, reqID interface{}) *Response {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return errorResponse(reqID, -1, fmt.Sprintf("Failed to serialize transaction: %v", err))
	}
	return successResponse(reqID, fmt.Sprintf("%x", buf.Bytes()))
}

// buildVerboseTransactionResult builds a verbose JSON result for a transaction.
func buildVerboseTransactionResult(tx *wire.MsgTx, blockHash *chainhash.Hash, height, bestHeight int32) map[string]interface{} {
	result := map[string]interface{}{
		"txid":          tx.TxHash().String(),
		"version":       tx.Version,
		"locktime":      tx.LockTime,
		"blockhash":     blockHash.String(),
		"blockheight":   height,
		"confirmations": bestHeight - height + 1,
	}

	vin := make([]map[string]interface{}, len(tx.TxIn))
	for i, txIn := range tx.TxIn {
		vin[i] = map[string]interface{}{
			"txid":     txIn.PreviousOutPoint.Hash.String(),
			"vout":     txIn.PreviousOutPoint.Index,
			"sequence": txIn.Sequence,
		}
	}
	result["vin"] = vin

	vout := make([]map[string]interface{}, len(tx.TxOut))
	for i, txOut := range tx.TxOut {
		vout[i] = map[string]interface{}{
			"value": float64(txOut.Value) / 1e8,
			"n":     i,
			"scriptPubKey": map[string]interface{}{
				"hex": fmt.Sprintf("%x", txOut.PkScript),
			},
		}
	}
	result["vout"] = vout

	return result
}

// sendRawTransaction broadcasts a raw transaction to the network.
// Parameters: [hexstring]
//   - hexstring (string, required): The hex-encoded raw transaction
//
// Returns: transaction hash (txid) if broadcast was successful
func (s *Server) sendRawTransaction(req *Request) *Response {
	params, errResp := parseInterfaceParams(req.Params, req.ID, 1, "[hexstring]")
	if errResp != nil {
		return errResp
	}

	hexStr, ok := params[0].(string)
	if !ok {
		return errorResponse(req.ID, -32602, "Invalid params: hexstring must be a string")
	}

	txBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return errorResponse(req.ID, -22, fmt.Sprintf("TX decode failed: %v", err))
	}

	var tx wire.MsgTx
	if err := tx.Deserialize(bytes.NewReader(txBytes)); err != nil {
		return errorResponse(req.ID, -22, fmt.Sprintf("TX decode failed: %v", err))
	}

	if s.peerMgr == nil {
		return errorResponse(req.ID, -1, "Network not available: peer manager not initialized")
	}

	if err := s.peerMgr.BroadcastTx(&tx); err != nil {
		return errorResponse(req.ID, -25, fmt.Sprintf("Transaction rejected: %v", err))
	}

	txHash := tx.TxHash()
	return successResponse(req.ID, txHash.String())
}

// writeError writes an error response
func (s *Server) writeError(w http.ResponseWriter, req *Request, code int, message string) {
	// Log error with structured logging
	if s.logger != nil {
		s.logger.Warn("rpc error response",
			"method", req.Method,
			"error_code", code,
			"error_message", message,
		)
	}

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
		if s.logger != nil {
			s.logger.Error("failed to encode JSON-RPC error response",
				"error", err,
				"method", req.Method,
			)
		}
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
