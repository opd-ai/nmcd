package rpc

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/network"
)

// Server provides RPC interface using standard library
type Server struct {
	blockchain *chain.BlockChain
	peerMgr    *network.PeerManager
	listener   net.Listener
	server     *http.Server
	mu         sync.RWMutex
}

// Config holds RPC server configuration
type Config struct {
	Blockchain *chain.BlockChain
	PeerMgr    *network.PeerManager
	ListenAddr string
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
		blockchain: cfg.Blockchain,
		peerMgr:    cfg.PeerMgr,
		listener:   listener,
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

// handleRequest handles incoming RPC requests
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
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
	case "name_show":
		return s.nameShow(req)
	case "name_update":
		return s.nameUpdate(req)
	case "name_list":
		return s.nameList(req)
	case "name_history":
		return s.nameHistory(req)
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

// nameUpdate updates a name (placeholder - requires wallet integration)
func (s *Server) nameUpdate(req *Request) *Response {
	return &Response{
		Jsonrpc: "2.0",
		Error: &Error{
			Code:    -1,
			Message: "name_update is currently unavailable because wallet functionality is not implemented in this node. " +
				"Use a wallet-enabled node or refer to the project documentation for how to update names.",
		},
		ID: req.ID,
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

// nameHistory returns the history of a name
func (s *Server) nameHistory(req *Request) *Response {
	// Method stub: name_history is not yet implemented.
	// Returning an explicit error avoids misleading clients into thinking
	// they are receiving full historical data.
	return &Response{
		Jsonrpc: "2.0",
		Error: &Error{
			Code:    -32601,
			Message: "name_history method is not yet implemented",
		},
		ID: req.ID,
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
