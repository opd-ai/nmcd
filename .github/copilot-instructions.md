# Project Overview

nmcd is a lightweight, pure Go implementation of a Namecoin daemon built using btcd libraries as dependencies. It focuses on composition over reimplementation, leveraging btcd's battle-tested blockchain, peer, and wire packages for core Bitcoin protocol functionality while extending them with Namecoin-specific name operation support. The project implements a bbolt-backed name database, blockchain integration with name validation hooks, P2P networking using btcd/peer, a JSON-RPC server using standard library net/http, and basic wallet functionality for managing name operations.

The target audience includes developers building decentralized naming systems, blockchain researchers exploring alternative implementations, and operators running lightweight Namecoin nodes. The implementation prioritizes minimal custom code (~3,000 lines excluding tests), thread-safe operations with mutex protection, and interface-based network types for enhanced testability. Key unique aspects include tight integration with btcd libraries, name expiration tracking (36,000 blocks ≈ 250 days), and support for three name operations: NAME_NEW (pre-registration), NAME_FIRSTUPDATE (first registration), and NAME_UPDATE (value updates).

## Technical Stack

- **Primary Language**: Go 1.24.11 (pure Go implementation, no C dependencies)
- **Frameworks**: 
  - **btcd v0.25.0**: Core blockchain functionality (blockchain.BlockChain, peer management, wire protocol)
  - **btcd/btcutil v1.1.5**: Bitcoin utility functions and block/transaction handling
  - **bbolt v1.4.3**: Embedded key-value database for name storage and history
  - **Standard library**: net/http for JSON-RPC server, crypto/ecdsa for wallet keys
- **Testing**: Go's built-in testing package with temporary file/directory management (t.TempDir())
- **Build/Deploy**: 
  - Makefile with standard targets (build, test, fmt, vet, clean)
  - Single binary deployment (`go build -v ./cmd/nmcd`)
  - No external runtime dependencies beyond Go standard library

## Code Assistance Guidelines

1. **Use Interface Types for Network Variables**: Always declare network variables using interface types, never concrete types. Use `net.Conn` instead of `*net.TCPConn`, `net.PacketConn` instead of `*net.UDPConn`, `net.Listener` instead of `*net.TCPListener`, and `net.Addr` instead of concrete address types like `*net.TCPAddr` or `*net.UDPAddr`. Avoid type switches or type assertions to convert from interface to concrete types; use the interface methods instead. This enhances testability and flexibility when working with different network implementations or mocks. Example: `func handleConnection(conn net.Conn)` not `func handleConnection(conn *net.TCPConn)`.

2. **Compose with btcd Libraries, Don't Reimplement**: Prefer embedding and extending btcd types rather than reimplementing blockchain logic. The chain.BlockChain embeds `*blockchain.BlockChain` and adds name validation hooks via `validateNameOperations()` and `updateNameDatabase()`. When adding blockchain features, check if btcd already provides the functionality and compose around it. Never fork or modify btcd code inline; maintain clean separation through wrapper types.

3. **Thread-Safe State Management**: All shared state must be protected with appropriate mutex types. Use `sync.RWMutex` for read-heavy operations (like namedb.NameDatabase and chain.BlockChain) and `sync.Mutex` for write-heavy operations. Follow the pattern: lock acquisition at the start of public methods, defer unlock, then perform operations. Example: `func (ndb *NameDatabase) GetName(name string) (*NameRecord, error) { ndb.mu.RLock(); defer ndb.mu.RUnlock(); ... }`. Document lock ordering in comments when multiple locks are involved to prevent deadlocks.

4. **Error Wrapping with Context**: Use `fmt.Errorf` with `%w` verb for error wrapping to maintain error chains for debugging. Each layer should add meaningful context. Example: `return fmt.Errorf("failed to update name database: %w", err)` not just `return err`. Top-level errors (RPC, network handlers) should log errors before returning to ensure visibility even if callers discard them.

5. **Name Operation Validation**: All name operations must validate against Namecoin protocol rules before blockchain processing. Check: name length (1-255 characters), value size (≤1023 bytes), name uniqueness (no duplicate unexpired names), proper operation sequence (NAME_NEW before NAME_FIRSTUPDATE), and expiration status (36,000 block validity). Validation failures should return descriptive errors, not generic "invalid name operation" messages. See `chain.validateNameOperations()` for the canonical validation pattern.

6. **JSON-RPC Standard Compliance**: RPC methods must follow JSON-RPC 2.0 specification with proper request/response structure. All responses include `jsonrpc: "2.0"`, `id` from request, and either `result` or `error` (never both). Errors use standard error codes: -32700 (Parse error), -32600 (Invalid Request), -32601 (Method not found), -32602 (Invalid params), -32603 (Internal error). Custom application errors should use codes ≥ -32099. Always validate HTTP Basic Auth when credentials are configured (see `rpc.Server.checkAuth()`).

7. **Testing with Temporary Resources**: Use `t.TempDir()` for test databases and `defer` for cleanup. Tests must be hermetic (no shared state between tests) and deterministic (same input = same output). Follow the table-driven test pattern for multiple scenarios. Mock external dependencies (peers, network connections) using interfaces. Example pattern: create temporary DB in setup, perform operations, verify state, cleanup handled automatically by Go's testing framework.

## Project Context

- **Domain**: Decentralized naming system built on Namecoin blockchain. Supports DNS-like functionality with blockchain-backed name records that map names (e.g., "d/example") to arbitrary JSON values. Primary use case is censorship-resistant domain name resolution. Key concepts: name expiration (36,000 blocks ≈ 250 days), name operations (NAME_NEW prevents front-running, NAME_FIRSTUPDATE claims name, NAME_UPDATE modifies value), and thread-safe name database with historical tracking.

- **Architecture**: Layered architecture with clear separation of concerns. Config layer (network parameters, chain params) → NameDB layer (bbolt storage, name/history/expiration buckets) → Chain layer (blockchain wrapper with name validation hooks) → Network/RPC layers (P2P peer management, JSON-RPC server) → Command layer (main binary). Dependencies flow upward; lower layers have no knowledge of higher layers. The blockchain notification system (NTBlockConnected/NTBlockDisconnected) keeps the name database consistent during chain reorganizations.

- **Key Directories**:
  - `cmd/nmcd/`: Main entry point, flag parsing, component initialization
  - `namedb/`: Name database with bbolt storage (names, history, expiration, name_new buckets)
  - `chain/`: Blockchain wrapper extending btcd's blockchain.BlockChain with name validation
  - `network/`: P2P peer management using btcd/peer, DNS seed resolution
  - `rpc/`: JSON-RPC server with standard methods (getinfo, getblockcount) and name methods (name_show, name_list, name_history, name_update)
  - `wallet/`: Basic wallet with ECDSA key management, unencrypted JSON persistence
  - `config/`: Network configuration, Namecoin chain parameters, DNS seed nodes
  - `examples/`: Usage examples demonstrating library integration

- **Configuration**: 
  - Data directory (`-datadir` flag, default: `~/.nmcd`) contains wallet.json (unencrypted private keys - secure with file permissions 0600) and names.db (bbolt database)
  - Network selection: mainnet (default), testnet (`-network=testnet`), regtest (`-network=regtest`)
  - RPC server: default localhost:8336 for mainnet, 18336 for testnet
  - Peer discovery via DNS seeds (automatic unless `-addpeer` specified): mainnet uses nmc.seed.quisquis.de, seed.nmc.markasoftware.com, etc.; testnet uses dnsseed.test.namecoin.webbtc.com
  - RPC authentication via `-rpcuser` and `-rpcpassword` flags (HTTP Basic Auth - use only over localhost or with proper network security)

## Quality Standards

- **Testing Requirements**: Maintain comprehensive test coverage using Go's built-in testing package. All public APIs must have unit tests. Tests should be hermetic (use `t.TempDir()` for temporary resources) and fast (mock external dependencies). Critical paths (name validation, blockchain processing, RPC handlers) require table-driven tests covering normal cases, edge cases (empty names, max length, expired names), and error cases (network failures, invalid JSON). Run `make test` (executes `go test -v ./...`) before committing. Integration tests should use regtest network mode for deterministic blockchain state.

- **Code Review Criteria**: 
  - All code must pass `go fmt` (gofmt -s) and `go vet` before review
  - Thread safety: verify mutex usage for all shared state access
  - Error handling: all errors must be checked and properly wrapped with context
  - Interface types: verify network variables use interfaces (net.Conn, net.Listener) not concrete types
  - Documentation: public APIs require godoc comments explaining purpose, parameters, return values, and any special considerations (e.g., thread safety, blocking behavior)
  - Simplicity: prefer stdlib over external dependencies; justify any new dependencies based on compelling need

- **Documentation Standards**: 
  - README.md must accurately reflect current functionality (update feature lists, RPC methods, and architectural descriptions when adding/removing features)
  - All public packages require package-level documentation explaining purpose and usage patterns
  - Complex algorithms (e.g., name expiration calculation, blockchain reorganization handling) require explanatory comments
  - Security-sensitive code (wallet key management, RPC authentication) must document threat model and security assumptions
  - Update examples/ directory when adding new library usage patterns

## Networking Best Practices (for Go projects)

When declaring network variables, always use interface types:
- Never use `net.UDPAddr`, `net.IPAddr`, or `net.TCPAddr`. Use `net.Addr` only instead.
- Never use `net.UDPConn`, use `net.PacketConn` instead
- Never use `net.TCPConn`, use `net.Conn` instead
- Never use `net.UDPListener` or `net.TCPListener`, use `net.Listener` instead
- Never use a type switch or type assertion to convert from an interface type to a concrete type. Use the interface methods instead.

This approach enhances testability and flexibility when working with different network implementations or mocks.

## Additional Context

### Known Limitations (from docs/development/AUDIT.md and docs/development/PROTOCOL_COMPLIANCE_AUDIT.md)

1. **Missing Features**: No mempool implementation (cannot store/relay unconfirmed transactions), no active block sync mechanism (relies on peer announcements), incomplete `name_update` RPC (creates transaction but doesn't broadcast - requires UTXO management).

2. **Protocol Compatibility**: ~35% compatible with Namecoin Core. Missing critical features for production use: AuxPow (merged mining) support, block version validation for AuxPow, and Namecoin-specific subsidy calculation. Cannot sync with mainnet past block 19,200 (AuxPow activation). Suitable for development/testing but NOT production mainnet use.

3. **Security Considerations**: Wallet stores unencrypted private keys in `wallet.json`. RPC authentication credentials visible in process listings when using command-line flags. HTTP Basic Auth transmits base64-encoded (not encrypted) credentials. Design assumes localhost-only RPC access or proper network security (firewall, VPN, reverse proxy with HTTPS).

### Design Principles

1. **Composition over Reimplementation**: Leverage btcd libraries directly; only implement Namecoin-specific name operation logic.
2. **Standard Library Preference**: Use net/http for RPC (no web frameworks), encoding/json for serialization, crypto/ecdsa for wallet keys.
3. **Interface-Based Design**: Use net.Conn not *net.TCPConn, define clear interfaces (e.g., blockchain.IndexManager) for dependency injection and testing.
4. **Thread Safety by Default**: Protect all shared state with mutexes; document any single-threaded assumptions.
5. **Minimal Custom Code**: Focus implementation on name-specific functionality; ~3,000 lines of production code (excluding tests and examples).

### Name Operations Reference

- **NAME_NEW**: Pre-register a name commitment to prevent front-running. Requires random salt. Commitment: `Hash(name || salt)`. Must be followed by NAME_FIRSTUPDATE within 12 blocks.
- **NAME_FIRSTUPDATE**: First registration of a name, reveals the name from NAME_NEW commitment. Sets initial value (JSON format, max 1023 bytes). Establishes ownership address.
- **NAME_UPDATE**: Update existing name value. Must be signed by current owner's private key. Extends expiration by 36,000 blocks from update height. Can optionally transfer to new address.

### RPC Method Categories

- **Standard Blockchain Methods**: getinfo, getblockcount, getbestblockhash, getconnectioncount, getpeerinfo
- **Name Methods**: name_show (get name info), name_list (list all names), name_history (get name operation history), name_update (update name value - currently only prepares transaction)
- **Wallet Methods**: getnewaddress (generate new address), listaddresses (list all wallet addresses)
- All methods require HTTP Basic Auth when `-rpcuser` and `-rpcpassword` are configured
