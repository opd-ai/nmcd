# Implementation Gaps — 2026-05-25

This document records discrepancies between what `nmcd` claims (in `README.md`, `docs/`, `ROADMAP.md`, and exported GoDoc) and what the current code actually does.

---

## Name-operation validation is silently disabled for some blocks

- **Stated Goal:** *"Blockchain Integration: Embeds btcd's blockchain.BlockChain with name validation hooks"* (README §"Daemon Features") and the project's "Name Operations Reference" in the repository instructions: NAME_NEW, NAME_FIRSTUPDATE and NAME_UPDATE must be validated per Namecoin protocol rules.
- **Current State:** `chain/blockchain.go:765-779` (`validateNameOperations`) returns `nil` (success) whenever `determineBlockHeight(block)` fails — the entire name-operation validation loop is skipped. See AUDIT.md CRIT-1.
- **Impact:** A block whose height cannot be derived (malformed coinbase, genesis edge cases, future protocol changes) bypasses **all** name validation (fee checks, duplicate detection, commitment matching, expiration, UTXO chain). The daemon will accept blocks that violate Namecoin consensus.
- **Closing the Gap:** propagate the error from `determineBlockHeight` (`return fmt.Errorf("…: %w", err)`). Add a regression test that injects a block whose height cannot be determined and asserts validation rejects it.

---

## Expiration comparison convention is inconsistently applied

- **Stated Goal:** Repository convention (stored memory and the comment at `chain/blockchain.go:678`): *"Per namedb convention: `ExpiresAt < currentHeight` means expired"* (strict `<`). The rest of the codebase (`rpc/server.go:779`, `client/embedded.go:293/708/894`, `namedb/namedb.go:376`) follows this.
- **Current State:** Three sites in `chain/blockchain.go` violate it: lines 735 (`validateNameUpdateOp`, `<=`), 2129 (`validateNameUpdate`, `<=`), and 2088 (`validateNameFirstUpdate`, `>`). See AUDIT.md HIGH-1/2/3.
- **Impact:** Consensus divergence at the boundary block. The daemon will accept or reject NAME_UPDATE / NAME_FIRSTUPDATE transactions one block apart from every other Namecoin node, producing forks and refusing legitimate renewals on the final valid block.
- **Closing the Gap:** change `<=` to `<` at lines 735 and 2129; change `>` to `>=` at line 2088. Add table-driven tests covering the boundary block (`record.ExpiresAt == currentHeight`).

---

## CI baseline (`go test -race ./...`) is currently red

- **Stated Goal:** `Makefile` target `test` runs `go test -v ./...`; the project promotes "Thread-Safe" and "comprehensive test coverage".
- **Current State:** `TestLookupActiveNameRecordExpired` in `rpc/coverage_boost_test.go:549-575` fails on every run. The test's setup contradicts the production code's correct `<` convention. See AUDIT.md HIGH-4.
- **Impact:** Any contributor running the documented `make test` workflow gets a failing test and has to triage. CI pipelines that gate on `go test` reject all PRs unless this is fixed or skipped.
- **Closing the Gap:** rewrite the test so that `bestHeight > record.ExpiresAt` (the documented condition for "expired").

---

## SMTP upstream is plaintext for any port other than 587

- **Stated Goal:** `mail/doc.go` and `cmd/permamail/main.go` advertise the binary as a "SMTP relay" using Namecoin DNS for routing; users reasonably assume modern TLS.
- **Current State:** `mail/smtp.go:453-461` upgrades to STARTTLS only when `UpstreamPort == 587`. Port 25 and port 465 (implicit-TLS submission) connect via plain `smtp.Dial` and then call `client.Auth(...)` — SASL credentials and message bodies traverse the network in cleartext. See AUDIT.md HIGH-5.
- **Impact:** Credential disclosure. Email content disclosure. Trivial man-in-the-middle.
- **Closing the Gap:** add an explicit `UpstreamTLS` configuration (`disabled`/`starttls`/`implicit`) and refuse `Auth` over cleartext unless the operator opts in.

---

## Library wallet API `CreateNameUpdateTxRaw` mis-routes change funds

- **Stated Goal:** "Name Updates: Update values and extend expiration (36,000 blocks)" (README §"Library Features"); the receiver-method `(*Wallet).CreateNameUpdateTx` correctly sends change back to the current owner.
- **Current State:** The exported package-level helper `wallet.CreateNameUpdateTxRaw` (intended for callers that don't hold all keys) uses `destAddress` for *both* the name output and the change output, so when a NAME_UPDATE transfers ownership the change is delivered to the new owner. See AUDIT.md HIGH-6.
- **Impact:** Library users calling the documented public helper lose their change to the recipient of a name transfer.
- **Closing the Gap:** add a `changeAddress` parameter, document it, and version the change as a minor release. Add an example.

---

## Mempool feature description matches code, but mempool is not exposed via library

- **Stated Goal:** README §"Daemon Features": *"Transaction Mempool: Validates and relays unconfirmed transactions with automatic expiration."* Stated under **Library Features**: the client exposes `RegisterName`, `UpdateName`, `ListNames`, `ResolveName` only.
- **Current State:** `network/mempool.go` implements validation + expiration; `network/peermgr.go:436-452` wires it to peer relay. However, `client/embedded.go`'s `RegisterName` / `UpdateName` flows do not push the constructed transactions into the mempool — they construct a `*wire.MsgTx` and the caller has no obvious path to broadcast it (compare to RPC `sendrawtransaction` flow). The README's library "Register or Update Names" example implies the transaction will be broadcast, but neither `RegisterName` nor `UpdateName` returns a broadcast-confirmation, only a `TxHash` string.
- **Impact:** Library users believe their NAME_NEW / NAME_FIRSTUPDATE / NAME_UPDATE has been broadcast when in fact it has only been constructed locally. Names will never appear on the network.
- **Closing the Gap:** in embedded mode, after constructing a transaction, submit it to `peerMgr.GetMempool().AddTx` and trigger `relayTransaction`. In daemon mode, send `sendrawtransaction`. Document both behaviors in `docs/EXAMPLES.md`.

---

## `name_update` RPC documented as functional, but per `docs/development/AUDIT.md` it does not broadcast

- **Stated Goal:** README §"Name Methods": `name_update` listed without caveats; example `curl` invocation shown.
- **Current State:** The pre-existing internal audit (`docs/development/AUDIT.md` and `docs/development/PROTOCOL_COMPLIANCE_AUDIT.md`, and the in-repository instructions) acknowledge: *"incomplete `name_update` RPC (creates transaction but doesn't broadcast — requires UTXO management)"*. The README does not surface this.
- **Impact:** Operators following the README's RPC examples submit name updates that succeed at the JSON-RPC level but never propagate to the network.
- **Closing the Gap:** either complete the broadcast path (preferred) or add a prominent warning to the README's RPC section. Mention that `sendrawtransaction` is required.

---

## README claims ~18 000 lines of production code; actual is ~10 000

- **Stated Goal:** README §"Daemon Features" final bullet: *"Focused Implementation: ~18,000 lines of production code (excluding tests)"*; repository instructions in `custom_instruction` use *"~3,000 lines of production code (excluding tests and examples)"*.
- **Current State:** go-stats-generator with `--skip-tests` reports **10 087** lines of production code across 63 files.
- **Impact:** Misleading documentation; potential users sizing the dependency on an inflated estimate.
- **Closing the Gap:** update README to `~10,000 lines`. Reconcile with the older `~3,000 line` figure (which appears to predate the `client`, `mail`, `metrics`, `loadtest`, `bridge` packages).

---

## README claims AuxPow / merged-mining support; mainnet sync is impossible past block 19,200

- **Stated Goal:** No explicit AuxPow claim in `README.md`, but the project's `chain/auxpow.go` (530 lines) and the marketing as a "Namecoin daemon" implies mainnet usability.
- **Current State:** Pre-existing internal audit (`docs/development/PROTOCOL_COMPLIANCE_AUDIT.md`) states: *"~35 % compatible with Namecoin Core. Missing critical features for production use: AuxPow (merged mining) support, block version validation for AuxPow, and Namecoin-specific subsidy calculation. Cannot sync with mainnet past block 19,200 (AuxPow activation). Suitable for development/testing but NOT production mainnet use."* `ValidateAuxPow` exists but per the same audit, integration with `ProcessBlock` is incomplete and the AuxPow code contains a duplicated/unused merkle-root computation at `chain/auxpow.go:311-323`.
- **Impact:** A user reading `README.md` and choosing the daemon for a mainnet wallet, mail relay, or name resolver cannot sync the chain.
- **Closing the Gap:** add a prominent **Status** block at the top of `README.md` stating mainnet is not supported past block 19,200; link to `docs/development/PROTOCOL_COMPLIANCE_AUDIT.md`. Track AuxPow completion in `ROADMAP.md`.

---

## Wallet was historically "unencrypted JSON"; current code supports encryption but README still warns about unencrypted

- **Stated Goal:** Repository instructions: *"Wallet stores unencrypted private keys in `wallet.json`"*.
- **Current State:** `wallet/encryption.go` (258 lines) implements AES-GCM encryption with password-derived keys. `walletpassphrase`/`walletlock` RPCs exist (`rpc/server.go`). However, the unlock passphrase is retained as a Go `string` and cannot be zeroed (AUDIT.md MED-4). Documentation still describes the wallet as unencrypted.
- **Impact:** Users who upgrade do not realize encryption is now available; users who rely on documentation may continue using unencrypted wallets unnecessarily.
- **Closing the Gap:** update README §"Security Considerations" and `docs/OPERATIONS.md` to describe wallet encryption (commands, KDF, threat model). Fix the in-memory passphrase retention per MED-4.

---

## Logging side-effects: file mode 0o644 + dir mode 0o755 do not match the project's privacy posture

- **Stated Goal:** Project conventions emphasize private wallet (`0o600`) and security-conscious design (README §"Security Considerations").
- **Current State:** `internal/logging/logger.go:119,124` writes log file with mode `0o644` and creates parent dir with `0o755`. Logs contain peer IPs, RPC remote_addr, panic stack traces (including filenames), and may reflect wallet/data paths.
- **Impact:** Information leak to any local user on a multi-tenant host.
- **Closing the Gap:** lower modes to `0o600` / `0o700` (AUDIT.md MED-2/3).

---

## `metrics` package mutates process-global state on import

- **Stated Goal:** Library-first design; "import safely into your Go application".
- **Current State:** `metrics/prometheus.go:24-37` `init()` starts a background goroutine refreshing Go runtime stats every 30 seconds. There is no `Stop()`. Any program that transitively imports `nmcd/metrics` (e.g., via `internal/server`) inherits the goroutine forever. See AUDIT.md MED-1.
- **Impact:** Surprising side-effect on library consumers; complicates test isolation and clean shutdown.
- **Closing the Gap:** convert to an explicit `Start(ctx)` / `Stop()` API called from the daemon main; never spawn goroutines in `init`.

---

## Embedded-mode `MaxPeers: 0` documented to disable network, but the relevant code branch is dead

- **Stated Goal:** README lines 184-187: *"`MaxPeers: 0,  // No peer connections`"* (advertised way to disable automatic network sync).
- **Current State:** `client/embedded.go:196` checks `cfg.MaxPeers == 0` but `applyConfigDefaults` (line 145-146) has already overwritten 0 → 8 before this point. The user-facing override path is implemented elsewhere (peer-manager start gating), but the README's literal recipe doesn't take the documented branch.
- **Impact:** No functional break (peers still don't connect because peer-manager honors `MaxPeers == 0` at a different layer), but the code is misleading and a future refactor could break the README's promise silently.
- **Closing the Gap:** apply the `MaxPeers == 0` short-circuit *before* `applyConfigDefaults` overwrites it, or document precisely where the gating happens. See AUDIT.md LOW-2.

---

## Examples use unexpanded `~/.nmcd` as default data directory

- **Stated Goal:** Examples are presented as "ready-to-run" reference code (`docs/EXAMPLES.md`).
- **Current State:** Several `examples/*/main.go` pass `flag.String("datadir", "~/.nmcd", ...)`. Go does not expand `~`; the daemon creates a directory literally named `~` under the user's current working directory.
- **Impact:** Confused users; orphaned `~` directories on disk.
- **Closing the Gap:** use `os.UserHomeDir()` + `filepath.Join`. See AUDIT.md MED-5.

---

## Stated "interface-based" `net.*` usage is followed; nothing to fix here

- **Stated Goal:** Repository instructions: *"Never use `net.TCPConn`, use `net.Conn` instead"*, etc.
- **Current State:** Verified — `network/`, `client/`, `rpc/` use `net.Conn`, `net.Listener`, `net.Addr` throughout. No concrete `*net.TCPConn` / `*net.UDPConn` etc. usage observed.
- **Impact:** None (goal achieved).
- **Closing the Gap:** N/A.
