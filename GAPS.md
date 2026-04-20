# Implementation Gaps — 2026-04-20

## H-1: Block Serving Not Implemented in onGetData

- **Intended Behavior**: When a remote peer sends a `getdata` message requesting specific blocks (identified by hash), the node should retrieve those blocks from its blockchain database and send them back via `wire.MsgBlock` messages. This is a fundamental part of the Bitcoin/Namecoin P2P protocol that enables block propagation and allows peers to sync from this node.
- **Current State**: The `onGetData` handler at `network/peermgr.go:465-470` logs a debug message ("received block request (not implemented)") and takes no further action. Transaction serving in the same handler (`case wire.InvTypeTx`, lines 472-489) works correctly — it retrieves transactions from the mempool and sends them to the requesting peer.
- **Blocked Goal**: Block synchronization (stated goal #6). This node cannot serve blocks to peers that request them, breaking the P2P protocol contract. Peers attempting to download blocks from this node during Initial Block Download will timeout waiting for block data.
- **Implementation Path**:
  1. In `network/peermgr.go`, in the `case wire.InvTypeBlock` branch of `onGetData`:
  2. Call `pm.blockchain.GetBlockByHash(&inv.Hash)` to retrieve the block from the blockchain database
  3. If found, serialize it as `block.MsgBlock()` and queue it to the peer with `p.QueueMessage(block.MsgBlock(), nil)`
  4. If not found, log a debug message (block may have been pruned or not yet received)
  5. Add a unit test that creates a PeerManager with a mock blockchain, calls `onGetData` with a block inventory, and verifies the block message is queued to the peer
- **Dependencies**: Requires `chain.BlockChain.GetBlockByHash()` which already exists at `chain/blockchain.go:1671`
- **Effort**: Small

---

## H-2: DaemonClient.RegisterName Returns Stub Error

- **Intended Behavior**: The `NameClient` interface defines `RegisterName(ctx, name, value, opts)` which should perform the two-phase Namecoin registration: (1) send NAME_NEW to commit, (2) wait 12 blocks, (3) send NAME_FIRSTUPDATE to reveal and register. The `EmbeddedClient` implements this fully at `client/embedded.go:413-451`.
- **Current State**: `client/daemon.go:402-413` validates inputs but then unconditionally returns: `"RegisterName via daemon mode is not yet supported: use embedded mode (ModeEmbedded) or call name_new/name_firstupdate RPC methods directly on Namecoin Core"`. The documentation (lines 388-401) explicitly acknowledges this gap and provides workarounds.
- **Blocked Goal**: Library-first design (stated goal #1). The `NameClient` interface contract is broken for `DaemonClient` — callers cannot use `RegisterName` polymorphically across client modes. The README Quick Start shows `RegisterName` as a primary use case.
- **Implementation Path**:
  1. Add a `pendingRegistrations` map to `DaemonClient` to track NAME_NEW → NAME_FIRSTUPDATE workflows
  2. In `RegisterName`: call `c.rpcCall(ctx, "name_new", [name])` to submit NAME_NEW
  3. If `opts.WaitForConfirmation` is true, poll for 12 block confirmations, then call `c.rpcCall(ctx, "name_firstupdate", [name, rand, value])` 
  4. Parse the response and return a `TxResult` with the transaction hash
  5. Handle the case where the daemon is Namecoin Core (slightly different RPC response format)
  6. Add tests using a mock RPC server
- **Dependencies**: Requires the daemon (nmcd or Namecoin Core) to have `name_new` and `name_firstupdate` RPC methods implemented. nmcd's RPC server already has both (registered at `rpc/server.go:443-446`).
- **Effort**: Medium

---

## M-1: onGetBlocks Sends Empty Inventory Response

- **Intended Behavior**: When a peer sends `getblocks` with block locator hashes, the node should respond with an inventory message containing block hashes from the common ancestor forward. This is the legacy block synchronization protocol (predecessor to headers-first sync).
- **Current State**: `network/peermgr.go:524-545` logs the request but creates an empty `MsgInv` and sends it (lines 543-544). The comment on lines 534-536 explicitly states this is intentional: "For now, just log that we received the request." The `onGetHeaders` handler at lines 500-521 is fully implemented using `pm.blockchain.LocateHeaders()`.
- **Blocked Goal**: Block synchronization (stated goal #6) for peers using the legacy `getblocks` protocol. Modern peers use `getheaders` (which works correctly), so this primarily affects older clients.
- **Implementation Path**:
  1. Use `pm.blockchain.LocateBlocks(msg.BlockLocatorHashes, &msg.HashStop)` to find block hashes after the common ancestor (similar to `LocateHeaders` used in `onGetHeaders`)
  2. Note: btcd's `blockchain.BlockChain` exposes `LocateBlocks()` which returns `[]chainhash.Hash`
  3. Populate the inventory message with `wire.NewInvVect(wire.InvTypeBlock, &hash)` for each returned hash
  4. Limit to 500 hashes per message (standard protocol limit)
  5. Add a test verifying the handler returns correct block inventory for known chain state
- **Dependencies**: Requires `blockchain.BlockChain.LocateBlocks()` from btcd — verify this method is available on the embedded type
- **Effort**: Small

---

## M-2: Transaction Search Limited to Last 1000 Blocks

- **Intended Behavior**: The `getrawtransaction` RPC method and `WaitForConfirmation` should be able to find any confirmed transaction by its hash. This is a standard blockchain RPC capability.
- **Current State**: Two separate implementations share the same limitation:
  - `rpc/server.go:1891-1917` (`searchTransaction`): scans blocks from `bestHeight` down to `bestHeight - 1000`
  - `client/embedded.go:1082-1096` (`getTransactionConfirmationStatus`): same 1000-block window
  
  Both implementations use linear block scanning with O(blocks × transactions_per_block) complexity. The RPC documentation at line 1855-1856 acknowledges: "It does not currently support mempool transactions or a full transaction index."
- **Blocked Goal**: JSON-RPC completeness (stated goal #8). Historical transaction queries fail silently with "Transaction not found" for transactions older than ~7 days.
- **Implementation Path**:
  1. Add a `txindex` bucket to `namedb.NameDatabase` mapping `txHash → (blockHeight, txIndex)`
  2. Populate the index during `chain.ProcessBlock()` when processing each transaction in a block
  3. Handle rollbacks in `chain.rollbackNameOperations()` by removing index entries for disconnected blocks
  4. Replace the linear scan in `searchTransaction` and `getTransactionConfirmationStatus` with an index lookup
  5. Optionally, also search the mempool for unconfirmed transactions
  6. This is a significant change that touches namedb, chain, rpc, and client packages
- **Dependencies**: M-2 is self-contained but requires careful testing of the index during chain reorganizations
- **Effort**: Large

---

## M-3: Duplicate Resolver Interface Definition

- **Intended Behavior**: Interfaces should be defined once and imported where needed, following Go's composition principles and the project's stated "composition over reimplementation" design philosophy.
- **Current State**: The `Resolver` interface is defined identically in two packages:
  - `bridge/namecoin.go:29-36`: `Resolver` with `LookupMail(ctx, name) (MailConfig, error)`
  - `mail/router.go:27-31`: `Resolver` with `LookupMail(ctx, name) (bridge.MailConfig, error)`
  
  The `mail.Resolver` already imports and uses `bridge.MailConfig` as its return type, creating a dependency on `bridge` anyway. The comment at line 26 acknowledges: "Note: This interface matches bridge.Resolver for seamless integration."
- **Blocked Goal**: No direct functional impact. Violates the project's composition principle and creates a maintenance risk where interfaces could diverge.
- **Implementation Path**:
  1. Remove the `Resolver` interface definition from `mail/router.go`
  2. Change `mail.Router.resolver` field type from `mail.Resolver` to `bridge.Resolver`
  3. Update `mail.NewRouter()` signature to accept `bridge.Resolver`
  4. Update all callers (internal/test code) to pass `bridge.Resolver`
  5. Verify no external consumers depend on `mail.Resolver` as a type
- **Dependencies**: None
- **Effort**: Small

---

## M-4: onVerAck Handler Is Empty No-Op

- **Intended Behavior**: In the Bitcoin/Namecoin protocol, the version handshake completes when both peers exchange `version` and `verack` messages. After `verack`, the connection is considered fully established and sync can begin.
- **Current State**: `network/peermgr.go:293-295` has an empty function body with only a comment `// Handle verack message`. It is registered as a callback at lines 183 and 243. The `onVersion` handler (line 285-290) triggers sync updates, which works in practice because btcd's peer implementation calls both handlers in sequence.
- **Blocked Goal**: None critical. The current behavior works because btcd/peer handles the version handshake internally. The empty handler is not causing incorrect behavior.
- **Implementation Path**:
  1. Option A (minimal): Add a comment documenting that `onVerAck` is intentionally empty because btcd/peer handles the version handshake internally and `onVersion` already triggers sync
  2. Option B (proper): Add peer state tracking — mark the peer as "handshake complete" in `onVerAck` and only allow sync/data messages from peers that have completed the handshake
  3. For Option B, add a `handshakeComplete` field to peer tracking in `PeerManager.peers`
- **Dependencies**: None
- **Effort**: Small (Option A) / Medium (Option B)

---

## L-1: Contradictory Protocol Compliance Documentation

- **Intended Behavior**: Documentation should accurately and consistently describe the project's protocol compliance status.
- **Current State**: Two sources give conflicting information:
  - `chain/doc.go:76-83` states: "approximately 35% protocol compatibility with Namecoin Core" citing missing features: full AuxPow validation, block version validation, subsidy edge cases
  - `docs/development/PROTOCOL_COMPLIANCE_AUDIT.md:12` states: "100% Protocol Compliance" with 22/22 checks passed
  - `ROADMAP.md:87` states: "Protocol Compliance: 95%+ ... see chain/doc.go Known Limitations"
  
  The 35% figure in doc.go appears to be outdated — it likely predates the AuxPow implementation and protocol compliance work documented in the audit. The protocol compliance audit was last updated 2026-03-23 and reports all checks passing.
- **Blocked Goal**: Documentation accuracy. Users reading `chain/doc.go` will believe the project is only 35% compatible when the actual compliance is much higher.
- **Implementation Path**:
  1. Update `chain/doc.go` Known Limitations section to reflect current compliance status
  2. Replace "approximately 35% protocol compatibility" with an accurate figure consistent with the protocol compliance audit
  3. Keep the specific limitations listed (AuxPow parent chain PoW, subsidy edge cases) but frame them accurately as known limitations, not as evidence of 35% compliance
  4. Add a reference to `docs/development/PROTOCOL_COMPLIANCE_AUDIT.md` for detailed compliance tracking
- **Dependencies**: None
- **Effort**: Small

---

## L-2: AuxPow Chain Merkle Branch Validation Uses Pragmatic Shortcut

- **Intended Behavior**: Per the Namecoin protocol, the chain merkle branch in an AuxPow proof should cryptographically prove that the block hash is committed in the parent block's coinbase transaction. This requires strict verification of the merkle path.
- **Current State**: `chain/auxpow.go:345-375` implements a relaxed verification:
  - If the chain merkle branch is empty (line 345-349), the proof is accepted as "direct commitment"
  - If the computed merkle root doesn't match the coinbase hash (line 357), the code falls back to accepting the proof if the branch depth is ≤32 (line 371)
  - Comments (lines 362-368) explain: "This is a pragmatic approach that works with various merged mining formats while still providing strong security guarantees"
  
  This is documented as an intentional design choice in `chain/doc.go` Known Limitations. The relaxation affects security only if an attacker can create a valid parent block PoW with a false chain merkle commitment.
- **Blocked Goal**: Full protocol compliance (goal #13). This is a known limitation tracked in `docs/development/PROTOCOL_COMPLIANCE_AUDIT.md` under "Mainnet: ⚠️ See Known Limitations."
- **Implementation Path**:
  1. Implement strict chain merkle root verification matching Namecoin Core's `CheckMerkleBranch` logic
  2. Parse the coinbase transaction's outputs to find the merge-mining commitment
  3. Verify the computed chain merkle root matches the commitment in the coinbase
  4. This requires understanding the specific output format used by mining pools for merge-mining commitments
  5. Test against real mainnet blocks with AuxPow (blocks ≥ 19,200)
- **Dependencies**: Requires real mainnet AuxPow block test vectors for validation
- **Effort**: Medium

---

## L-3: Dependency Audit — All Dependencies In Use

- **Intended Behavior**: The `go.mod` file should only contain dependencies that are actively used by the project.
- **Current State**: All direct dependencies in `go.mod` are actively used:
  - `github.com/BurntSushi/toml v1.6.0` — used in `config/configfile.go` for TOML config parsing
  - `github.com/btcsuite/btcd v0.25.0` — core blockchain dependency used throughout
  - `github.com/btcsuite/btcd/btcec/v2 v2.3.5` — used in wallet for ECDSA operations
  - `github.com/btcsuite/btcd/btcutil v1.1.5` — used for block/transaction utilities
  - `github.com/btcsuite/btcd/chaincfg/chainhash v1.1.0` — used for hash operations
  - `github.com/prometheus/client_golang v1.23.2` — used in metrics and prometheus_exporter
  - `go.etcd.io/bbolt v1.4.3` — used in namedb for embedded database
  - `golang.org/x/crypto v0.25.0` — used in wallet/encryption.go for scrypt
  
  Indirect dependencies are pulled by btcd and prometheus. No unused direct dependencies detected.
- **Blocked Goal**: None
- **Implementation Path**: Run `go mod tidy` periodically to verify. Current state is clean.
- **Dependencies**: None
- **Effort**: Small (maintenance only)

---

## Implementation Roadmap (Priority Order)

### Tier 1: High-Impact, Low-Effort
1. **H-1** (Block serving in onGetData) — Small effort, fixes P2P protocol compliance
2. **M-1** (onGetBlocks inventory) — Small effort, fixes legacy sync protocol
3. **L-1** (Documentation consistency) — Small effort, reduces confusion

### Tier 2: High-Impact, Medium-Effort
4. **H-2** (DaemonClient.RegisterName) — Medium effort, completes NameClient contract
5. **M-3** (Duplicate Resolver) — Small effort, improves code quality
6. **M-4** (onVerAck documentation) — Small effort, clarifies intent

### Tier 3: Significant Infrastructure
7. **M-2** (Transaction index) — Large effort, enables full transaction history
8. **L-2** (Strict AuxPow validation) — Medium effort, improves mainnet readiness

### Dependencies Graph
```
H-1 (block serving) ← no dependencies
H-2 (RegisterName) ← no dependencies
M-1 (getblocks) ← no dependencies
M-2 (tx index) ← no dependencies, but benefits from H-1 being done first
M-3 (Resolver) ← no dependencies
M-4 (onVerAck) ← no dependencies
L-1 (docs) ← no dependencies
L-2 (AuxPow) ← benefits from mainnet test vectors
L-3 (deps) ← no dependencies
```

All gaps are independent — they can be addressed in any order. The recommended priority above is based on impact-to-effort ratio.
