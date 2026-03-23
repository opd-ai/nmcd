// Package rpc provides a JSON-RPC 2.0 server for nmcd.
//
// The rpc package implements a standard library-based JSON-RPC server that
// provides remote access to nmcd functionality. It supports both standard
// Bitcoin/Namecoin RPC methods and Namecoin-specific name operation methods.
//
// # JSON-RPC 2.0 Compliance
//
// The server implements JSON-RPC 2.0 specification:
//
//   - Request format: {"jsonrpc":"2.0","method":"...","params":[...],"id":1}
//   - Response format: {"jsonrpc":"2.0","result":...,"id":1}
//   - Error format: {"jsonrpc":"2.0","error":{"code":...,"message":"..."},"id":1}
//
// Standard error codes:
//
//   - -32700: Parse error
//   - -32600: Invalid request
//   - -32601: Method not found
//   - -32602: Invalid params
//   - -32603: Internal error
//
// # Supported Methods
//
// Standard blockchain methods:
//
//   - getinfo: Returns node information
//   - getblockcount: Returns current block height
//   - getbestblockhash: Returns tip block hash
//   - getblock: Returns block data by hash
//   - getblockhash: Returns block hash by height
//   - getrawtransaction: Returns raw transaction data
//   - sendrawtransaction: Broadcasts a signed transaction
//
// Peer methods:
//
//   - getconnectioncount: Returns connected peer count
//   - getpeerinfo: Returns detailed peer information
//
// Name methods (Namecoin-specific):
//
//   - name_show: Returns name record by name
//   - name_list: Lists all registered names
//   - name_history: Returns name operation history
//   - name_scan: Scans names by prefix
//   - name_filter: Filters names by pattern
//   - name_pending: Returns pending name operations
//   - name_update: Creates a name update transaction
//
// Wallet methods:
//
//   - getnewaddress: Generates a new address
//   - listaddresses: Lists wallet addresses
//   - getbalance: Returns wallet balance
//   - dumpprivkey: Exports private key (requires unlocked wallet)
//
// # Authentication
//
// The server supports HTTP Basic Authentication when configured:
//
//	server, err := rpc.NewServer(&rpc.Config{
//	    RPCUser:     "user",
//	    RPCPassword: "password",
//	    ListenAddr:  "127.0.0.1:8336",
//	})
//
// When credentials are configured, all requests must include the Authorization
// header. The server uses constant-time comparison to prevent timing attacks.
//
// # Rate Limiting
//
// Built-in rate limiting protects against abuse:
//
//   - Default: 100 requests/minute per IP
//   - Configurable via Config.RateLimit
//   - Uses token bucket algorithm
//   - Returns HTTP 429 when exceeded
//
// # Security Considerations
//
//   - Bind to localhost only unless behind a firewall/VPN
//   - Credentials are transmitted as base64 (use HTTPS in production)
//   - Wallet operations require explicit unlock for sensitive methods
//   - Request size is limited to prevent resource exhaustion
//
// # Thread Safety
//
// The Server is safe for concurrent use. Multiple goroutines can handle
// requests simultaneously, and all shared state is protected with sync.RWMutex.
//
// # Example Usage
//
// Creating and starting an RPC server:
//
//	cfg := &rpc.Config{
//	    Blockchain:  blockchain,
//	    PeerMgr:     peerManager,
//	    Wallet:      wallet,
//	    ListenAddr:  "127.0.0.1:8336",
//	    RPCUser:     "rpcuser",
//	    RPCPassword: "rpcpassword",
//	    RateLimit:   100,
//	}
//	server, err := rpc.NewServer(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Start serving (blocks)
//	go func() {
//	    if err := server.Start(); err != nil {
//	        log.Printf("RPC server error: %v", err)
//	    }
//	}()
//	defer server.Stop()
//
// Making RPC calls (using curl):
//
//	curl -u user:password --data-binary \
//	  '{"jsonrpc":"2.0","method":"name_show","params":["d/example"],"id":1}' \
//	  http://127.0.0.1:8336/
//
// # Default Ports
//
//   - Mainnet: 8336
//   - Testnet: 18336
//   - Regtest: 18443
package rpc
