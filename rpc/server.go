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
	"strconv"
	"sync"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/chain"
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
	rateLimiter    *rateLimiter
	maxRequestSize int64
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

	s := &Server{
		blockchain:     cfg.Blockchain,
		peerMgr:        cfg.PeerMgr,
		wallet:         cfg.Wallet,
		listener:       listener,
		rpcUser:        cfg.RPCUser,
		rpcPassword:    cfg.RPCPassword,
		rateLimiter:    newRateLimiter(rateLimit),
		maxRequestSize: maxRequestSize,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)

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
	return s.server.Close()
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

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		// Log encoding error but can't send another response at this point
		fmt.Fprintf(os.Stderr, "failed to encode JSON-RPC response: %v\n", err)
	}
}

// processRequest processes a JSON-RPC request
func (s *Server) processRequest(req *Request) *Response {
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	case "getblock":
		return s.getBlock(req)
	case "getblockhash":
		return s.getBlockHash(req)
	case "getrawtransaction":
		return s.getRawTransaction(req)
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
	best := s.blockchain.BestSnapshot()

	info := map[string]interface{}{
		"version":     "0.1.0",
		"blocks":      best.Height,
		"connections": s.peerMgr.GetConnectedPeers(),
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
	best := s.blockchain.BestSnapshot()

	return &Response{
		Jsonrpc: "2.0",
		Result:  best.Height,
		ID:      req.ID,
	}
}

// getBestBlockHash returns the best block hash
func (s *Server) getBestBlockHash(req *Request) *Response {
	best := s.blockchain.BestSnapshot()

	return &Response{
		Jsonrpc: "2.0",
		Result:  best.Hash.String(),
		ID:      req.ID,
	}
}

// getConnectionCount returns the number of connections
func (s *Server) getConnectionCount(req *Request) *Response {
	count := s.peerMgr.GetConnectedPeers()

	return &Response{
		Jsonrpc: "2.0",
		Result:  count,
		ID:      req.ID,
	}
}

// getPeerInfo returns information about peers
func (s *Server) getPeerInfo(req *Request) *Response {
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
	// Check if wallet is available
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

	// Parse parameters
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 2 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params: expected [\"name\", \"value\"] or [\"name\", \"value\", \"address\"]",
			},
			ID: req.ID,
		}
	}

	name := params[0]
	newValue := params[1]

	// Parse optional destination address (third parameter)
	// This enables name ownership transfer. If not provided, the name stays at the current address.
	// Format: name_update "d/example" "new value" "N1Address..."
	var destAddress btcutil.Address
	if len(params) >= 3 && params[2] != "" {
		// Decode and validate the destination address
		addr, err := btcutil.DecodeAddress(params[2], s.blockchain.ChainParams())
		if err != nil {
			return &Response{
				Jsonrpc: "2.0",
				Error: &Error{
					Code:    -5,
					Message: fmt.Sprintf("Invalid destination address: %v", err),
				},
				ID: req.ID,
			}
		}
		// Ensure it's a P2PKH address (Namecoin only supports P2PKH for name operations)
		if _, ok := addr.(*btcutil.AddressPubKeyHash); !ok {
			return &Response{
				Jsonrpc: "2.0",
				Error: &Error{
					Code:    -5,
					Message: fmt.Sprintf("Destination address must be P2PKH, got: %T", addr),
				},
				ID: req.ID,
			}
		}
		destAddress = addr
	}

	// Validate name format
	if len(name) == 0 || len(name) > 255 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Invalid name length: %d (max 255)", len(name)),
			},
			ID: req.ID,
		}
	}

	// Validate value format
	if len(newValue) > 1023 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Value too large: %d bytes (max 1023)", len(newValue)),
			},
			ID: req.ID,
		}
	}

	// Look up the current name record
	record, err := s.blockchain.GetName(name)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -4,
				Message: fmt.Sprintf("Name not found: %s", name),
			},
			ID: req.ID,
		}
	}

	// Check if name is expired
	bestHeight := s.blockchain.BestSnapshot().Height
	if record.ExpiresAt <= bestHeight {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -4,
				Message: fmt.Sprintf("Name expired at block %d (current: %d)", record.ExpiresAt, bestHeight),
			},
			ID: req.ID,
		}
	}

	// Check if wallet has the key for the current owner
	// (needed to sign the transaction spending the name UTXO)
	if !s.wallet.HasKey(record.Address) {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -13,
				Message: fmt.Sprintf("Wallet does not have the private key for address: %s", record.Address),
			},
			ID: req.ID,
		}
	}

	// Get the name UTXO (the UTXO holding the current name registration)
	nameUTXO, err := s.blockchain.GetNameUTXO(name)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to get name UTXO: %v", err),
			},
			ID: req.ID,
		}
	}

	// Get wallet UTXOs for fee payment
	walletUTXOs, err := s.blockchain.GetUTXOsForAddress(record.Address)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to get wallet UTXOs: %v", err),
			},
			ID: req.ID,
		}
	}

	// Convert namedb UTXOs to wallet UTXOs
	var utxos []wallet.UTXO
	nameUtxoIndex := -1
	for _, dbUTXO := range walletUTXOs {
		wUtxo := wallet.UTXO{
			TxHash:   dbUTXO.TxHash,
			Vout:     dbUTXO.OutIndex,
			Value:    dbUTXO.Value,
			PkScript: dbUTXO.PkScript,
			Address:  dbUTXO.Address,
		}
		// Check if this is the name UTXO
		if dbUTXO.TxHash.IsEqual(&nameUTXO.TxHash) && dbUTXO.OutIndex == nameUTXO.OutIndex {
			nameUtxoIndex = len(utxos)
		}
		utxos = append(utxos, wUtxo)
	}

	if nameUtxoIndex == -1 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "Name UTXO not found in wallet UTXOs",
			},
			ID: req.ID,
		}
	}

	// Create the NAME_UPDATE transaction
	// Use a fee rate of 1 satoshi/byte (1000 satoshis/KB)
	// This is a reasonable fee for Namecoin transactions
	feeRate := int64(1) // satoshis per byte
	tx, err := s.wallet.CreateNameUpdateTx(name, newValue, utxos, nameUtxoIndex, feeRate, destAddress)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to create transaction: %v", err),
			},
			ID: req.ID,
		}
	}

	// Broadcast the transaction to the network
	// This adds it to our mempool and relays it to all connected peers
	err = s.peerMgr.BroadcastTx(tx)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to broadcast transaction: %v", err),
			},
			ID: req.ID,
		}
	}

	// Return success with transaction details
	txHash := tx.TxHash()
	result := map[string]interface{}{
		"txid":   txHash.String(),
		"name":   name,
		"value":  newValue,
		"status": "broadcasted", // Transaction is now in mempool and relayed to peers
	}

	// Include destination address in response if specified
	if destAddress != nil {
		result["address"] = destAddress.EncodeAddress()
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      req.ID,
	}
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
	// Check if wallet is available
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

	// Parse parameters
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 1 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params: expected [\"name\"]",
			},
			ID: req.ID,
		}
	}

	name := params[0]

	// Validate name format
	if len(name) == 0 || len(name) > 255 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Invalid name length: %d (max 255)", len(name)),
			},
			ID: req.ID,
		}
	}

	// Check if name already exists and is not expired
	existingRecord, err := s.blockchain.GetName(name)
	if err == nil {
		// Name exists, check if it's expired
		bestHeight := s.blockchain.BestSnapshot().Height
		if existingRecord.ExpiresAt > bestHeight {
			return &Response{
				Jsonrpc: "2.0",
				Error: &Error{
					Code:    -25,
					Message: fmt.Sprintf("Name already exists and is not expired (expires at block %d, current: %d)", existingRecord.ExpiresAt, bestHeight),
				},
				ID: req.ID,
			}
		}
	}

	// Get wallet address and UTXOs
	addr, utxos, errResp := s.getWalletAddressAndUTXOs(req.ID)
	if errResp != nil {
		return errResp
	}

	// Generate random salt (20 bytes) using wallet's crypto/rand helper
	randBytes, err := wallet.GenerateRand()
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to generate random salt for NAME_NEW: %v", err),
			},
			ID: req.ID,
		}
	}

	// Create the NAME_NEW transaction
	// Use a fee rate of 1 satoshi/byte (1000 satoshis/KB)
	feeRate := int64(1) // satoshis per byte
	tx, randBytesReturned, err := s.wallet.CreateNameNewTx(randBytes, name, utxos, feeRate, addr)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to create NAME_NEW transaction: %v", err),
			},
			ID: req.ID,
		}
	}

	// Broadcast the transaction to the network
	err = s.peerMgr.BroadcastTx(tx)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to broadcast transaction: %v", err),
			},
			ID: req.ID,
		}
	}

	// Return success with transaction details
	txHash := tx.TxHash()
	result := map[string]interface{}{
		"txid":   txHash.String(),
		"name":   name,
		"rand":   fmt.Sprintf("%x", randBytesReturned), // Hex-encode the random bytes
		"status": "broadcasted",                        // Transaction is now in mempool and relayed to peers
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      req.ID,
	}
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
	// Check if wallet is available
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

	// Parse parameters
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 3 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params: expected [\"name\", \"rand\", \"value\"]",
			},
			ID: req.ID,
		}
	}

	name := params[0]
	randHex := params[1]
	value := params[2]

	// Validate name format
	if len(name) == 0 || len(name) > 255 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Invalid name length: %d (max 255)", len(name)),
			},
			ID: req.ID,
		}
	}

	// Validate value format
	if len(value) > 1023 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Value too large: %d bytes (max 1023)", len(value)),
			},
			ID: req.ID,
		}
	}

	// Decode and validate random bytes
	randBytes, err := hex.DecodeString(randHex)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Invalid rand hex: %v", err),
			},
			ID: req.ID,
		}
	}

	// Compute the commitment hash to find the NAME_NEW UTXO
	commitHash := wallet.ComputeNameNewHash(randBytes, name, s.blockchain.ChainParams())

	// Check if NAME_NEW commitment exists
	nameNewRecord, err := s.blockchain.GetNameDB().GetNameNew(commitHash)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -25,
				Message: "NAME_NEW commitment not found. You must call name_new first and wait for confirmation.",
			},
			ID: req.ID,
		}
	}

	// Check if enough blocks have passed (minimum 12 blocks)
	bestHeight := s.blockchain.BestSnapshot().Height
	blocksSinceNameNew := bestHeight - nameNewRecord.Height
	if blocksSinceNameNew < 12 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -25,
				Message: fmt.Sprintf("NAME_NEW not confirmed enough. Need 12 blocks, only %d blocks have passed.", blocksSinceNameNew),
			},
			ID: req.ID,
		}
	}

	// Check if too many blocks have passed (maximum 36,000 blocks - name expiration period)
	if blocksSinceNameNew > 36000 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -25,
				Message: fmt.Sprintf("NAME_NEW commitment expired. Maximum window is 36,000 blocks, but %d blocks have passed.", blocksSinceNameNew),
			},
			ID: req.ID,
		}
	}

	// Get wallet address and UTXOs
	addr, utxos, errResp := s.getWalletAddressAndUTXOs(req.ID)
	if errResp != nil {
		return errResp
	}

	// Find the NAME_NEW UTXO by checking for OP_NAME_NEW opcode in scripts
	nameNewUtxoIndex := -1
	for i, utxo := range utxos {
		// Try to identify NAME_NEW UTXO by checking the script
		// NAME_NEW script format: OP_NAME_NEW <hash> OP_2DROP <P2PKH>
		if len(utxo.PkScript) > 22 && utxo.PkScript[0] == opNameNew {
			// This looks like a NAME_NEW output, mark it as a candidate
			if nameNewUtxoIndex == -1 {
				nameNewUtxoIndex = i
			}
		}
	}

	if nameNewUtxoIndex == -1 {
		// If we couldn't identify the NAME_NEW UTXO, use the first UTXO as a fallback
		// The wallet transaction creation will handle validation
		nameNewUtxoIndex = 0
	}

	// Create the NAME_FIRSTUPDATE transaction
	// Use a fee rate of 1 satoshi/byte (1000 satoshis/KB)
	feeRate := int64(1) // satoshis per byte
	tx, err := s.wallet.CreateNameFirstUpdateTx(name, randHex, value, utxos, nameNewUtxoIndex, feeRate, addr)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to create NAME_FIRSTUPDATE transaction: %v", err),
			},
			ID: req.ID,
		}
	}

	// Broadcast the transaction to the network
	err = s.peerMgr.BroadcastTx(tx)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to broadcast transaction: %v", err),
			},
			ID: req.ID,
		}
	}

	// Return success with transaction details
	txHash := tx.TxHash()
	result := map[string]interface{}{
		"txid":   txHash.String(),
		"name":   name,
		"value":  value,
		"status": "broadcasted", // Transaction is now in mempool and relayed to peers
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      req.ID,
	}
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
				Message: "Invalid params: expected [password] or [password, timeout]",
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

	// Get timeout (default: 60 seconds)
	timeout := 60
	if len(params) > 1 {
		timeoutFloat, ok := params[1].(float64)
		if !ok {
			return &Response{
				Jsonrpc: "2.0",
				Error: &Error{
					Code:    -32602,
					Message: "Invalid timeout parameter: expected integer",
				},
				ID: req.ID,
			}
		}
		timeout = int(timeoutFloat)
		if timeout <= 0 {
			return &Response{
				Jsonrpc: "2.0",
				Error: &Error{
					Code:    -32602,
					Message: "Invalid timeout: must be positive",
				},
				ID: req.ID,
			}
		}
	}

	// Check if wallet is encrypted
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

	// Unlock wallet
	if err := s.wallet.Unlock(password); err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -14,
				Message: fmt.Sprintf("Failed to unlock wallet: %v", err),
			},
			ID: req.ID,
		}
	}

	// Schedule auto-lock after timeout
	time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		s.wallet.Lock()
	})

	return &Response{
		Jsonrpc: "2.0",
		Result:  nil,
		ID:      req.ID,
	}
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

// getBlock returns a block by hash with optional verbose mode.
// Parameters: [blockhash] or [blockhash, verbose]
//   - blockhash (string, required): The block hash as hex string
//   - verbose (bool, optional): If false (default), returns hex-encoded block data.
//     If true, returns JSON object with block details.
func (s *Server) getBlock(req *Request) *Response {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) == 0 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params: expected [blockhash] or [blockhash, verbose]",
			},
			ID: req.ID,
		}
	}

	// Parse block hash
	hashStr, ok := params[0].(string)
	if !ok {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params: blockhash must be a string",
			},
			ID: req.ID,
		}
	}

	hash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Invalid block hash: %v", err),
			},
			ID: req.ID,
		}
	}

	// Parse optional verbose parameter (default is false for hex output)
	verbose := false
	if len(params) >= 2 {
		v, ok := params[1].(bool)
		if !ok {
			return &Response{
				Jsonrpc: "2.0",
				Error: &Error{
					Code:    -32602,
					Message: "Invalid params: verbose must be a boolean",
				},
				ID: req.ID,
			}
		}
		verbose = v
	}

	// Get the block from blockchain
	block, err := s.blockchain.GetBlockByHash(hash)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Block not found: %v", err),
			},
			ID: req.ID,
		}
	}

	if !verbose {
		// Return hex-encoded block data
		blockBytes, err := block.Bytes()
		if err != nil {
			return &Response{
				Jsonrpc: "2.0",
				Error: &Error{
					Code:    -1,
					Message: fmt.Sprintf("Failed to serialize block: %v", err),
				},
				ID: req.ID,
			}
		}

		return &Response{
			Jsonrpc: "2.0",
			Result:  fmt.Sprintf("%x", blockBytes),
			ID:      req.ID,
		}
	}

	// Return verbose JSON object
	msgBlock := block.MsgBlock()
	header := msgBlock.Header

	// Capture best snapshot once to avoid race conditions
	bestSnapshot := s.blockchain.BestSnapshot()

	// Get block height
	height, err := s.blockchain.BlockHeightByHash(hash)
	if err != nil {
		// If we can't get height, return -1 (for orphan blocks)
		height = -1
	}

	// Calculate confirmations
	var confirmations int32
	if height == -1 {
		// Orphan block - no confirmations
		confirmations = 0
	} else {
		confirmations = bestSnapshot.Height - height + 1
	}

	// Build transaction list
	txs := make([]string, len(msgBlock.Transactions))
	for i, tx := range msgBlock.Transactions {
		txs[i] = tx.TxHash().String()
	}

	// Calculate actual difficulty from bits
	// Bitcoin difficulty = max_target / current_target
	// where max_target is the difficulty 1 target
	difficulty := getDifficultyRatio(header.Bits, s.blockchain.ChainParams())

	result := map[string]interface{}{
		"hash":              hash.String(),
		"confirmations":     confirmations,
		"height":            height,
		"version":           header.Version,
		"merkleroot":        header.MerkleRoot.String(),
		"time":              header.Timestamp.Unix(),
		"nonce":             header.Nonce,
		"bits":              fmt.Sprintf("%08x", header.Bits),
		"difficulty":        difficulty,
		"previousblockhash": header.PrevBlock.String(),
		"tx":                txs,
	}

	// Add next block hash if not the best block
	if height >= 0 && height < bestSnapshot.Height {
		nextHash, err := s.blockchain.BlockHashByHeight(height + 1)
		if err == nil {
			result["nextblockhash"] = nextHash.String()
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// getBlockHash returns the block hash for a given height.
// Parameters: [height]
// - height (int): The block height
func (s *Server) getBlockHash(req *Request) *Response {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) == 0 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params: expected [height]",
			},
			ID: req.ID,
		}
	}

	// Parse height - handle both int and float64 from JSON
	var height int32
	switch v := params[0].(type) {
	case float64:
		// Check for overflow before conversion
		if v > 2147483647 || v < -2147483648 {
			return &Response{
				Jsonrpc: "2.0",
				Error: &Error{
					Code:    -32602,
					Message: fmt.Sprintf("Invalid params: height out of int32 range: %v", v),
				},
				ID: req.ID,
			}
		}
		height = int32(v)
	case int:
		height = int32(v)
	case int32:
		height = v
	default:
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: fmt.Sprintf("Invalid params: height must be a number, got %T", params[0]),
			},
			ID: req.ID,
		}
	}

	// Validate height is non-negative
	if height < 0 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -8,
				Message: "Block height out of range",
			},
			ID: req.ID,
		}
	}

	// Get hash by height
	hash, err := s.blockchain.BlockHashByHeight(height)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -8,
				Message: fmt.Sprintf("Block height out of range: %v", err),
			},
			ID: req.ID,
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  hash.String(),
		ID:      req.ID,
	}
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
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) == 0 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params: expected [txid] or [txid, verbose]",
			},
			ID: req.ID,
		}
	}

	// Parse transaction ID
	txidStr, ok := params[0].(string)
	if !ok {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params: txid must be a string",
			},
			ID: req.ID,
		}
	}

	txid, err := chainhash.NewHashFromStr(txidStr)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Invalid transaction ID: %v", err),
			},
			ID: req.ID,
		}
	}

	// Parse optional verbose parameter
	verbose := false
	if len(params) >= 2 {
		v, ok := params[1].(bool)
		if !ok {
			return &Response{
				Jsonrpc: "2.0",
				Error: &Error{
					Code:    -32602,
					Message: "Invalid params: verbose must be a boolean",
				},
				ID: req.ID,
			}
		}
		verbose = v
	}

	// Search for transaction in recent blocks
	// We search backwards from the current best block
	// Capture best snapshot once to avoid race conditions
	bestSnapshot := s.blockchain.BestSnapshot()
	bestHeight := bestSnapshot.Height

	// Limit search to last 1000 blocks to prevent excessive lookups
	// For a full transaction index, use btcd's txindex
	startHeight := bestHeight - 1000
	if startHeight < 0 {
		startHeight = 0
	}

	var foundTx *wire.MsgTx
	var foundBlockHash *chainhash.Hash
	var foundHeight int32

	for height := bestHeight; height >= startHeight; height-- {
		hash, err := s.blockchain.BlockHashByHeight(height)
		if err != nil {
			continue
		}

		block, err := s.blockchain.GetBlockByHash(hash)
		if err != nil {
			continue
		}

		// Search transactions in this block
		for _, tx := range block.MsgBlock().Transactions {
			txHash := tx.TxHash()
			if txHash.IsEqual(txid) {
				foundTx = tx
				foundBlockHash = hash
				foundHeight = height
				break
			}
		}

		if foundTx != nil {
			break
		}
	}

	if foundTx == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Transaction not found: %s", txidStr),
			},
			ID: req.ID,
		}
	}

	if !verbose {
		// Return hex-encoded transaction
		var buf bytes.Buffer
		if err := foundTx.Serialize(&buf); err != nil {
			return &Response{
				Jsonrpc: "2.0",
				Error: &Error{
					Code:    -1,
					Message: fmt.Sprintf("Failed to serialize transaction: %v", err),
				},
				ID: req.ID,
			}
		}

		return &Response{
			Jsonrpc: "2.0",
			Result:  fmt.Sprintf("%x", buf.Bytes()),
			ID:      req.ID,
		}
	}

	// Build verbose JSON response
	result := map[string]interface{}{
		"txid":          foundTx.TxHash().String(),
		"version":       foundTx.Version,
		"locktime":      foundTx.LockTime,
		"blockhash":     foundBlockHash.String(),
		"blockheight":   foundHeight,
		"confirmations": bestHeight - foundHeight + 1,
	}

	// Add inputs
	vin := make([]map[string]interface{}, len(foundTx.TxIn))
	for i, txIn := range foundTx.TxIn {
		vin[i] = map[string]interface{}{
			"txid":     txIn.PreviousOutPoint.Hash.String(),
			"vout":     txIn.PreviousOutPoint.Index,
			"sequence": txIn.Sequence,
		}
	}
	result["vin"] = vin

	// Add outputs
	vout := make([]map[string]interface{}, len(foundTx.TxOut))
	for i, txOut := range foundTx.TxOut {
		vout[i] = map[string]interface{}{
			"value": float64(txOut.Value) / 1e8, // Convert satoshis to NMC
			"n":     i,
			"scriptPubKey": map[string]interface{}{
				"hex": fmt.Sprintf("%x", txOut.PkScript),
			},
		}
	}
	result["vout"] = vout

	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// writeError writes an error response
func (s *Server) writeError(w http.ResponseWriter, req *Request, code int, message string) {
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
		fmt.Fprintf(os.Stderr, "failed to encode JSON-RPC error response: %v\n", err)
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

	s.mu.RLock()
	defer s.mu.RUnlock()

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

	s.mu.RLock()
	defer s.mu.RUnlock()

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
