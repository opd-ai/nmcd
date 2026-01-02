package rpc

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/metrics"
	"github.com/opd-ai/nmcd/network"
	"github.com/opd-ai/nmcd/wallet"
)

// Server provides RPC interface using standard library
type Server struct {
	blockchain  *chain.BlockChain
	peerMgr     *network.PeerManager
	wallet      *wallet.Wallet
	listener    net.Listener
	server      *http.Server
	rpcUser     string
	rpcPassword string
	mu          sync.RWMutex
}

// Config holds RPC server configuration
type Config struct {
	Blockchain  *chain.BlockChain
	PeerMgr     *network.PeerManager
	Wallet      *wallet.Wallet
	ListenAddr  string
	RPCUser     string
	RPCPassword string
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

	s := &Server{
		blockchain:  cfg.Blockchain,
		peerMgr:     cfg.PeerMgr,
		wallet:      cfg.Wallet,
		listener:    listener,
		rpcUser:     cfg.RPCUser,
		rpcPassword: cfg.RPCPassword,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

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

// handleRequest handles incoming RPC requests
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

	// TODO: Support changing the destination address (third parameter)
	// For now, the name remains at the same address

	// Check if wallet has the key for the current owner
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
	tx, err := s.wallet.CreateNameUpdateTx(name, newValue, utxos, nameUtxoIndex, feeRate)
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
	// Add to mempool
	mempool := s.peerMgr.GetMempool()
	err = mempool.AddTx(tx)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to add transaction to mempool: %v", err),
			},
			ID: req.ID,
		}
	}

	// Note: Transaction relay to peers is not yet implemented
	// The transaction is now in the mempool and will be:
	// 1. Available for inclusion in blocks we mine
	// 2. Returned in mempool queries
	// Future enhancement: Add peer.QueueMessage to broadcast to network

	// Return success with transaction details
	txHash := tx.TxHash()
	result := map[string]interface{}{
		"txid":   txHash.String(),
		"name":   name,
		"value":  newValue,
		"status": "mempool",
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
