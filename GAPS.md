# Implementation Gaps — 2026-05-27

These are gaps between what `nmcd`'s README and GoDoc claim and what the code actually does. Each gap references concrete file/line evidence and proposes a closing action.

## G-1 — NAME_NEW commitment hash is not Namecoin-Core-compatible

- **Stated Goal**: The README and `chain/doc.go` describe `nmcd` as a "Namecoin daemon" implementing the "two-phase NAME_NEW → NAME_FIRSTUPDATE protocol" of Namecoin. The implication — and what library users will assume — is that names registered via `nmcd` are visible to the wider Namecoin network and vice-versa.
- **Current State**: `chain/name_script.go:65-81` (`computeCommitHash`) computes `RIPEMD160(SHA256(rand ‖ name ‖ chainID))` where `chainID = chainParams.Net` (4 bytes, little-endian network magic). Namecoin Core's `CNameScript::buildCommitment` (src/names/common.cpp) computes `RIPEMD160(SHA256(rand ‖ name))` — **no** chain-ID suffix. Both producer (wallet `CreateNameNewTx`) and validator (`chain/blockchain.go:596, 1084, 1095, 1377, 1552`) use the same custom function, so internal tests pass.
- **Impact**: Hard consensus split with Namecoin Core. Every `NAME_FIRSTUPDATE` produced by `nmcd` fails Core's validation; every Core `NAME_FIRSTUPDATE` accepted from peers fails `nmcd`'s `GetNameNew` lookup at `chain/blockchain.go:599`, so the registration is rejected. On a live Namecoin network, no `nmcd` node can interoperate with any non-`nmcd` node for name registrations. Resolution of pre-existing names still works because that path doesn't recompute the commitment.
- **Closing the Gap**:
  1. Drop the `chainID` suffix to match upstream (preferred), OR
  2. Reposition the project in README/docs as a "Namecoin-derivative" with its own commitment scheme, and document the wire-incompatibility prominently.
  - Add a golden test in `chain/name_script_test.go` asserting `hex.EncodeToString(computeCommitHash(rand, name, &NamecoinMainNetParams))` matches a known Namecoin Core test vector.

## G-2 — Production LOC claim overstates the codebase by ~75 %

- **Stated Goal**: README claims "~18 000 lines of production code" as a measure of project maturity / completeness.
- **Current State**: `go-stats-generator analyze . --skip-tests` reports 10 238 non-test lines across 68 source files. The actual figure is roughly 57 % of the claim.
- **Impact**: Mis-sets expectations for users evaluating maturity. Auditors using LOC as a proxy for "what's there" will over-estimate test coverage per LOC and under-estimate per-feature LOC density.
- **Closing the Gap**: Update README to "~10 200 lines of production code (plus ~14 000 lines of tests)" or — more usefully — replace the LOC claim with a feature-completeness table or `go-stats-generator` summary that is automatically refreshable.

## G-3 — AuxPow validation is documented as "simplified" but README claims full Namecoin compatibility

- **Stated Goal**: README and `chain/doc.go` claim full Namecoin block validation, including merge-mining. Namecoin's PoW *is* AuxPow.
- **Current State**: `chain/auxpow.go:259-394` (`ValidateAuxPow`) intentionally does **not** parse the merged-mining commitment header (`0xfabe6d6d || merkle_root || merkle_size || nonce`). Source comments at L327-331, L342, L359-365 explicitly acknowledge this is a "pragmatic" / "robust across mining pools" simplification. In the empty-chain-merkle-branch branch (L345-348) no commitment check is performed at all; in the non-empty branch a fallback at L386 accepts any coinbase that *contains* the 32-byte computed root as a byte substring.
- **Impact**: An attacker willing to grind a parent-chain coinbase that *happens to* contain the 32 target bytes — but without the surrounding magic / size / nonce structure — produces an AuxPow `nmcd` accepts but Namecoin Core rejects. The parent-chain PoW difficulty check (L279-282) still applies, so the attack is bounded by parent-chain hash cost, not free. Still: any `nmcd` node accepting blocks rejected by the rest of the network forks itself off-chain.
- **Closing the Gap**: Implement the Namecoin Core check (`src/auxpow.cpp::check`):
  1. Locate magic `0xfabe6d6d` in the coinbase scriptSig immediately preceding the merkle root.
  2. Validate the 4-byte merkle size = `1 << len(ChainMerkleBranch)`.
  3. Validate the position computed from the 4-byte nonce equals the side-bit walk over `ChainMerkleBranch.SideMask`.
  4. Require exactly one occurrence (reject multiple magic sequences).
  - Add test vectors from real Namecoin merge-mined blocks (e.g. block 19 200, block 24 000) plus negative tests for forged coinbases. See `AUDIT.md` finding H-2 / H-3.

## G-4 — "Thread-safe: all operations safe for concurrent use" is undermined by non-idempotent `Stop()`

- **Stated Goal**: README emphasises thread-safety for embedded use ("all operations safe for concurrent use", "designed for embedding in concurrent applications").
- **Current State**: `network/peermgr.go:767`, `network/sync.go:57`, and `rpc/ratelimit.go:189` each call `close(<chan>)` in their respective `Stop()`/`stop()` methods with no `sync.Once`. By contrast `network/mempool.go:42, 255` uses `stopOnce sync.Once` correctly. A second `Stop()` invocation triggers `panic: close of closed channel`. `EmbeddedClient.Close()` guards itself with `c.closed` (`client/embedded.go:1172`), so embedded users are partially protected — but any test, daemon shutdown handler, or library wrapper that calls `peerMgr.Stop()` or `Server.Stop()` twice will panic the process.
- **Impact**: Thread-safe construction + non-idempotent destruction violates the stated invariant. Surfaces under SIGINT-races and double-close patterns common in defer-based cleanup.
- **Closing the Gap**: Apply the project's own `sync.Once` pattern from `Mempool` to all three sites. See `AUDIT.md` finding H-1.

## G-5 — RPC defaults expose the wallet when bound to non-loopback addresses

- **Stated Goal**: README documents the RPC interface and embedded daemon. Wallet methods (`walletpassphrase`, `sendrawtransaction`, `name_update`) require sensitive access.
- **Current State**: `rpc/server.go:391` skips authentication entirely when both `rpcUser` and `rpcPassword` are empty. The `Config` struct in `client/embedded.go` and the daemon entrypoint do not require credentials at construction time, and do not warn when the listen address is non-loopback.
- **Impact**: A user who follows the README and binds to `0.0.0.0:8336` or a public interface without explicitly setting credentials exposes wallet operations to any IP. Convention is documented elsewhere as "auth optional" but the README does not mark it as required for non-loopback bindings.
- **Closing the Gap**: In `rpc.NewServer`, refuse to start when `ListenAddr` resolves to a non-loopback address and credentials are empty; alternatively, log a `Warn`-level message ("RPC server bound to public address without authentication") on every request. Document the default explicitly in README. See `AUDIT.md` finding M-10.

## G-6 — `name_show` JSON shape diverges from Namecoin Core

- **Stated Goal**: README presents `name_show` alongside other JSON-RPC methods as Namecoin-compatible.
- **Current State**: `rpc/name_handlers.go:46-61` clamps `expires_in` to `0` when the record is expired, then sets `expired: true`. Namecoin Core returns a *negative* `expires_in` for expired records (e.g. `-12345` if expired 12 345 blocks ago) so consumers can compute "how long ago".
- **Impact**: Existing Namecoin client libraries that compare `expires_in < 0` for expiry will not detect expiry; libraries that compute "expired since" by negating `expires_in` will compute zero. Cross-implementation tooling breaks.
- **Closing the Gap**: Remove the clamp at L49-51; let `expires_in` go negative. Keep the `expired` boolean for clarity. See `AUDIT.md` finding M-9.

## G-7 — Headers-first IBD can stall after best-peer disconnect

- **Stated Goal**: README claims "headers-first IBD sync" comparable to Bitcoin Core / Namecoin.
- **Current State**: `network/sync.go:243-256` (`UpdatePeerHeight`) only ratchets `bestHeight` upward. `network/sync.go:280` assigns `bestPeer = findReplacementPeer(p)` after the best peer disconnects, but `findReplacementPeer` (`L286-305`) picks an arbitrary connected peer regardless of advertised height. `bestHeight` retains the disconnected peer's value, so `syncTick` (L93) keeps trying to sync against a peer that may be at a much lower height.
- **Impact**: After a network split that drops the tallest peer, IBD progress can stall indefinitely without an error or log entry. The condition heals only when a new peer with `height > stale bestHeight` connects.
- **Closing the Gap**: Track per-peer advertised heights in a `map[*peer.Peer]int32`; on disconnect, recompute `bestHeight = max(remaining)` and `bestPeer = argmax`. See `AUDIT.md` findings M-1, M-2.

## G-8 — `BroadcastTx` reports success with zero peer relay

- **Stated Goal**: README documents `sendrawtransaction` RPC for transaction submission.
- **Current State**: `network/peermgr.go:642-666` logs "no peers connected, transaction not relayed" but returns `nil` (success). `rpc/blockchain_handlers.go`'s `sendrawtransaction` handler propagates that as a JSON-RPC success and returns the txid.
- **Impact**: Wallet UIs and integrations believe the transaction was broadcast and proceed to wait for confirmations that never arrive. Users may double-spend or re-issue the transaction unnecessarily.
- **Closing the Gap**: Either change the BroadcastTx contract to return an error/sentinel when no peers were reached, or attach a `warning` field to the RPC response when relay count is zero. See `AUDIT.md` finding M-5.
