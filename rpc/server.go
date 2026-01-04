package rpc

import (
	"bytes"
	"crypto/subtle"
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
