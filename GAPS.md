# Implementation Gaps — 2026-05-29

## Name validation is stricter than Namecoin consensus → node cannot sync the real network
- **Stated Goal**: README — "Block Synchronization … Automatic Initial Block Download (IBD)
  and ongoing sync with the network via headers-first protocol" and faithful Namecoin name
  validation.
- **Current State**: During block processing (`chain/blockchain.go:307` → `validateNameOutputs`
  → `validateNameFormat` at `chain/blockchain.go:759`), every `NAME_FIRSTUPDATE`/`NAME_UPDATE`
  output is rejected unless: the name starts with `d/`, `id/`, or `p/`
  (`config.IsValidNamespace`, `config/config.go:96-100`); `d/`/`id/` values are valid JSON; and
  every value is valid UTF-8 (`chain/name_script.go:419-461`). Namecoin consensus enforces none
  of these: namespace prefixes are conventions, values are arbitrary bytes (≤ 520), and JSON is
  not required.
- **Impact**: The real Namecoin mainnet contains name transactions that violate these added
  rules. The first such block fails `ProcessBlock`, so IBD halts permanently and the node forks
  away from the network — the project's central daemon/embedded-sync goal is non-functional
  against real data. (The test suite uses only conforming JSON `d/` values, so this is invisible
  to `go test`.)
- **Closing the Gap**: Split validation into two layers — a *consensus* layer used by block
  processing that checks only Namecoin-enforced rules (name length ≤ 255, value length ≤ 520,
  script structure), and a *policy* layer (namespace/JSON/UTF-8) applied only to locally
  originated registrations in `wallet`/`rpc`. Remove the `validateNameFormat` call from
  `validateNameOutputs` (`chain/blockchain.go:759`) and replace it with consensus-only checks.
  Add a regtest/testnet sync test plus a unit test feeding a non-JSON `d/` value and a
  non-standard namespace through `ProcessBlock` and asserting acceptance.

## Consensus value-length limit is 1023, but Namecoin's is 520
- **Stated Goal**: Faithful Namecoin consensus validation (the daemon must agree with the
  network on which blocks/transactions are valid).
- **Current State**: `config/config.go:23-27` defines `MaxValueLength = 1023` and documents it
  as "the consensus limit"; it is enforced for block validation via `chain/name_script.go:415`.
  Namecoin's actual consensus `MAX_VALUE_LENGTH` is 520 bytes.
- **Impact**: nmcd will accept a block or mempool transaction carrying a 521–1023-byte name
  value that the real network rejects, diverging from consensus. This does not block honest IBD
  (real values never exceed 520) but is a latent fork/over-acceptance risk on crafted input.
- **Closing the Gap**: Use 520 as the consensus value-length limit in block validation; retain
  any larger value only as a non-consensus UI/policy constant clearly separated from consensus
  code. Add a unit test asserting a 600-byte value is rejected by `ProcessBlock`.

## Untrusted-script parser can panic on 32-bit builds
- **Stated Goal**: "Pure Go … cross-platform support"; robust handling of untrusted network
  data.
- **Current State**: `chain/name_script.go:358-376` parses an `OP_PUSHDATA4` length into a
  signed `int`; on a 32-bit build a length byte `≥ 0x80` yields a negative `dataLen` that slips
  past the bounds check and panics the slice operation. 64-bit builds are unaffected.
- **Impact**: On 32-bit targets, a single crafted peer transaction/script crashes the consensus
  parser (DoS). This undercuts the "cross-platform" claim for 32-bit deployments.
- **Closing the Gap**: Parse the length as `uint32`/`uint64` and reject values above the maximum
  script-element size before slicing in `readPushData`. Exercise via the existing
  `FuzzParseNameScript` target on a 32-bit build (`GOARCH=386 go test ./chain/`).

## SMTP relay has no upstream connect/IO timeout
- **Stated Goal**: `permamail` SMTP relay that forwards mail to upstream servers resolved from
  Namecoin name records (a shipped command, not just an example).
- **Current State**: `mail/smtp.go:443` uses `context.Background()` (no deadline) and
  `connectUpstream` (`mail/smtp.go:497-548`) dials with `smtp.Dial`/`tls.Dial`, neither of which
  applies a timeout or wires in the context.
- **Impact**: A slow or dead upstream (including a name-record-controlled host) hangs the
  per-connection goroutine indefinitely, enabling resource exhaustion of the relay.
- **Closing the Gap**: Dial with `net.DialTimeout`/`tls.DialWithDialer` and set connection
  deadlines from a bounded context in `connectUpstream`; add a test pointing at a non-accepting
  listener and asserting a timely failure.

## Documented production line-count overstated
- **Stated Goal**: README — "Focused Implementation: ~18,000 lines of production code
  (excluding tests)".
- **Current State**: go-stats-generator reports 10,464 lines of code (excluding comments/blank
  lines and tests) across 70 files; total non-test physical lines (incl. comments/blank) are
  ~21,343 but that figure includes documentation comments.
- **Impact**: Minor/cosmetic — sets an inaccurate expectation of codebase size. No functional
  effect.
- **Closing the Gap**: Update the README figure to reflect the measured ~10.5k LoC of
  production code (or clarify the counting methodology).
