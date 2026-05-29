# Implementation Gaps — 2026-05-29

## Daemon-mode expired-name filtering does not match listing promise
- **Stated Goal**: README advertises name listing with filters/pagination and embedded/daemon interoperability.
- **Current State**: Daemon mode can return expired names even when default filtering should exclude them (`rpc/name_handlers.go:488-492` + `client/daemon.go:617-619`).
- **Impact**: Consumers may treat expired names as active, causing incorrect routing/resolution and daemon-vs-embedded behavior drift.
- **Closing the Gap**: Unify expiry semantics between RPC and daemon client filtering (use explicit `expired` signal or non-clamped `expires_in`) and add regression tests for IncludeExpired behavior.

## Expiration representation is inconsistent across name RPC methods
- **Stated Goal**: README positions the daemon RPC API as a stable, standard interface.
- **Current State**: `name_show` returns negative `expires_in` for expired entries (`rpc/name_handlers.go:50-53`) while `name_list`/`name_scan` clamp to zero (`rpc/name_handlers.go:488-492`, `rpc/name_handlers.go:622-626`).
- **Impact**: Client implementations cannot rely on one invariant for expiration interpretation across methods, increasing integration complexity and error risk.
- **Closing the Gap**: Pick one consistent `expires_in` contract for all name endpoints, document it, and enforce with tests.

## Thread-safety claim is weakened by caller-visible filter mutation
- **Stated Goal**: README claims thread-safe operations for concurrent use.
- **Current State**: List operations mutate caller-owned `ListFilter` objects (`client/embedded.go:873-877`, `client/daemon.go:606-610`).
- **Impact**: Shared filter reuse can produce races or unexpected side effects outside client internals.
- **Closing the Gap**: Make normalization side-effect-free by copying filter input to local value structs before applying defaults/caps.
