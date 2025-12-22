# nmcd

Pure Go Namecoin implementation using btcd as library dependencies (not forks).

## Overview

nmcd is a lightweight Namecoin daemon built using btcd libraries. It focuses on composition over reimplementation, leveraging btcd's battle-tested blockchain, peer, and wire packages.

## Features

- **Pure Go**: Built entirely in Go using standard library and btcd
- **NameDatabase**: bbolt-backed storage for name operations
- **Blockchain Integration**: Embeds btcd's blockchain.BlockChain with name validation hooks
- **Network Layer**: Uses btcd/peer for P2P networking with interface-based connections (net.Conn)
- **RPC Server**: Standard library net/http for JSON-RPC interface
- **Thread-Safe**: Mutex protection for all shared state
- **Minimal Custom Code**: ~1200 lines of focused custom code

## Architecture

### Components

1. **namedb**: Name database with bbolt storage
   - Stores name records with expiration tracking
   - Historical operation tracking
   - Thread-safe with RWMutex

2. **chain**: Blockchain wrapper
   - Embeds btcd's blockchain.BlockChain
   - Extends validation with name operation hooks
   - Manages name expiration (36000 blocks ~250 days)

3. **network**: P2P networking
   - Uses btcd/peer for peer management
   - Interface-based connections (net.Conn)
   - Handles block/tx propagation

4. **rpc**: JSON-RPC server
   - Standard library net/http
   - Name-specific RPC methods
   - Thread-safe access

5. **config**: Configuration management
   - Network selection (mainnet/testnet/regtest)
   - Data directory management

## Building

```bash
go build -v ./cmd/nmcd
```

## Running

```bash
# Run with defaults
./nmcd

# Custom data directory
./nmcd -datadir=/path/to/data

# Testnet
./nmcd -network=testnet

# Custom RPC port
./nmcd -rpcaddr=127.0.0.1:18336

# Connect to specific peer
./nmcd -addpeer=peer.example.com:8334
```

## RPC API

### Standard Methods

- `getinfo` - Get general information
- `getblockcount` - Get current block height
- `getbestblockhash` - Get best block hash
- `getconnectioncount` - Get peer connection count
- `getpeerinfo` - Get connected peer information

### Name Methods

- `name_show` - Show name information
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"name_show","params":["d/example"],"id":1}'
  ```

- `name_list` - List all registered names
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"name_list","params":[],"id":1}'
  ```

- `name_history` - Get name history
  ```bash
  curl -X POST http://127.0.0.1:8336 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"name_history","params":["d/example"],"id":1}'
  ```

- `name_update` - Update an existing name's value (requires wallet - not yet implemented)
  
  > **Note:** The `name_update` RPC method is currently unavailable because wallet functionality is not implemented in this node. This feature is planned for a future release. In the meantime, use a wallet-enabled node to perform name updates.

## Dependencies

- **github.com/btcsuite/btcd/blockchain** - Blockchain management
- **github.com/btcsuite/btcd/peer** - P2P peer management
- **github.com/btcsuite/btcd/wire** - Wire protocol
- **go.etcd.io/bbolt** - Embedded database

## Design Principles

1. **Composition over Reimplementation**: Use btcd libraries directly
2. **Standard Library**: net/http for RPC, no web frameworks
3. **Interface Types**: net.Conn not concrete *net.TCPConn
4. **Thread Safety**: Mutex protection for all shared state
5. **Minimal Code**: Focus on name-specific functionality only

## Name Operations

The implementation supports three name operations:

1. **NAME_NEW**: Pre-register a name (prevents front-running)
2. **NAME_FIRSTUPDATE**: First registration of a name
3. **NAME_UPDATE**: Update existing name value

Names expire after 36000 blocks (~250 days) and must be renewed.

## Code Structure

```
nmcd/
├── cmd/nmcd/        # Main entry point
├── namedb/          # Name database (bbolt)
├── chain/           # Blockchain wrapper
├── network/         # P2P networking
├── rpc/             # JSON-RPC server
└── config/          # Configuration
```

## Security

- All name operations are validated before blockchain processing
- Names must be unique and unexpired
- Value size limits enforced (1023 bytes)
- Name length limits enforced (1-255 characters)

## License

See LICENSE file for details.

