# nmcd Development Status

**Last Updated:** 2024-12-15 (roadmap; not all items implemented yet)  
**Status:** 🚧 EXPERIMENTAL – NOT PRODUCTION READY

---

## Executive Summary

The nmcd Namecoin daemon is an experimental, lightweight Namecoin implementation built on btcd libraries. This document tracks **planned protocol-compliance work and milestones**; many items below are still incomplete and nmcd is **not suitable for production mainnet use**.

---

## Planned Work and Status

### Protocol Compliance Audit – Planned Items

The following items represent the intended scope of protocol-compliance work. Many of these are **not yet implemented** or only partially implemented in nmcd. For detailed implementation status, see PROTOCOL_COMPLIANCE_AUDIT.md.

**Critical Issues (3) – Status Unknown:**
- ◻ AuxPow (Merged Mining) Support - Planned full implementation with wire protocol and validation
- ◻ Block Version Validation - Planned AuxPow version bit (0x100) enforcement
- ◻ Subsidy Calculation - Planned Namecoin halving schedule (50 NMC → 25 NMC every 210k blocks)

**High Priority Issues (6) – Status Unknown:**
- ◻ NAME_NEW Fee Requirements - Planned dust limit (546 satoshis) validation
- ◻ NAME_FIRSTUPDATE Timing Window - Planned 12-36,000 block enforcement
- ◻ Transaction Fee Validation - Planned 1000 sat for NAME_NEW, 0.01 NMC for NAME_FIRSTUPDATE/UPDATE
- ◻ Chain ID in NAME_NEW Commitment - Planned cross-chain replay protection
- ◻ Namespace Validation - Planned enforcement of d/, id/, p/ prefixes
- ◻ Script Validation - Planned strict drop opcode and P2PKH suffix enforcement

**Medium Priority Issues (8) – Status Unknown:**
- ◻ Name Transfer Validation - Planned UTXO chain verification for NAME_UPDATE
- ◻ Reorg Handling - Planned exact NAME_NEW height restoration
- ◻ Value Encoding Validation - Planned UTF-8 and JSON validation
- ◻ Double-Spend Detection - Planned prevention of duplicate name operations per block
- ◻ Name Expiration Cleanup - Planned history deletion for expired names
- ◻ Network Magic Bytes - Planned corrections for testnet (0xfabfb5fe) and regtest (0xfabfb5da)
- ◻ Checkpoint System - Planned AuxPow activation checkpoint (block 19,200)
- ◻ Block Difficulty Validation - Planned PoW validation with Namecoin parameters
- ◻ Initial Block Download (IBD) - Planned full getheaders/headers/getdata protocol

**Low Priority Issues (4) – Status Unknown:**
- ◻ Protocol Version Negotiation - Planned Namecoin version 70015 support
- ◻ Error Messages - Planned enhancements with block hash, height, transaction context
- ◻ Metrics/Monitoring - Planned comprehensive metrics with RPC endpoint
- ◻ Test Vectors - Planned infrastructure for Namecoin Core test vectors

### Missing Features Status

**Planned (M1-M12):**
- ◻ M1: AuxPow Block Validation
- ◻ M2: Subsidy Calculation
- ◻ M3: Fee Validation
- ◻ M4: Namespace Validation
- ◻ M5: UTXO Chain Validation
- ◻ M6: Checkpoint System
- ◻ M7: Difficulty Retargeting Validation
- ◻ M8: Initial Block Download (IBD)
- ◻ M9: Mempool - Transaction relay
- ◻ M10: Name Transfer Script Support
- ◻ M11: Name Renewal Optimization
- ◻ M12: Witness Support - Future SegWit compatibility

---

## Current Capabilities

### Basic Name Operations (Partially Implemented)
- NAME_NEW: Pre-registration with commitment (implementation status unknown)
- NAME_FIRSTUPDATE: First registration with name reveal (implementation status unknown)
- NAME_UPDATE: Update name value and extend expiration (implementation status unknown)
- Name expiration tracking (implementation status unknown)
- Thread-safe name database using bbolt

### Network Support (Experimental)
- Mainnet (⚠️ NOT production ready)
- Testnet (experimental)
- Regtest (development/testing)

### Limitations
- ⚠️ Consensus rule implementation status uncertain - see PROTOCOL_COMPLIANCE_AUDIT.md for details
- ⚠️ No mempool/transaction relay support
- ⚠️ Limited network participation capabilities
- ⚠️ Not suitable for production blockchain validation

---

## Test Coverage

Tests exist for various components but comprehensive validation against Namecoin Core behavior is incomplete. See PROTOCOL_COMPLIANCE_AUDIT.md for detailed analysis.

---

## Development Status

### ⚠️ NOT Ready For:
- Production mainnet operation
- Critical blockchain validation
- Mining operations
- Transaction relay services
- Full node services requiring complete consensus rule implementation

### Suitable For:
- Development and testing
- Learning about Namecoin protocol
- Experimental regtest environments
- Research purposes

**Note:** This is an experimental implementation. For production use, consider using Namecoin Core instead.

---

## Future Development Plans

### PLAN.md - Embedded Library API
**Status:** Planning Phase (not started)  
**Effort:** Estimated 4-6 weeks  
**Priority:** Post-protocol compliance work

See PLAN.md for detailed architectural plans to convert nmcd into a reusable library.

### ROADMAP.md - Permamail Implementation
**Status:** Planning Phase (blocked)  
**Effort:** Estimated 4 weeks  
**Priority:** Future feature

See ROADMAP.md for details on planned decentralized email forwarding feature.

---

## Architecture

### Core Components

```
nmcd/
├── chain/           - Blockchain validation (status: see PROTOCOL_COMPLIANCE_AUDIT.md)
├── namedb/          - Name database using bbolt
├── network/         - P2P networking using btcd/peer
├── rpc/             - JSON-RPC server
├── wallet/          - Basic wallet functionality
├── config/          - Network parameters and constants
├── metrics/         - Monitoring infrastructure
└── cmd/nmcd/        - Main binary
```

### Design Principles

1. **Composition over reimplementation** - Leverage btcd libraries where possible
2. **Interface-based networking** - Use net.Conn, not concrete types
3. **Thread-safe operations** - Mutex protection for shared state
4. **Standard library preference** - Minimize external dependencies

---

## Dependencies

**Required:**
- Go 1.24.11+
- github.com/btcsuite/btcd v0.25.0 (Bitcoin protocol libraries)
- github.com/btcsuite/btcd/btcutil v1.1.5 (Bitcoin utilities)
- go.etcd.io/bbolt v1.4.3 (Embedded database)

---

## Contributing

See PROTOCOL_COMPLIANCE_AUDIT.md for detailed analysis of protocol compliance status before contributing.

### Development Process

1. Run tests: `make test`
2. Check formatting: `make fmt`
3. Run linter: `make vet`
4. Build: `make build`

---

## Documentation

- **README.md** - Project overview and quick start
- **PROTOCOL_COMPLIANCE_AUDIT.md** - Detailed protocol compliance analysis
- **PLAN.md** - Library API implementation plan (future)
- **ROADMAP.md** - Permamail feature roadmap (future)
- **COMPLETION_STATUS.md** - This document (development status summary)

---

## License

ISC License (same as btcd and Namecoin Core)

---

## Acknowledgments

Built using btcd libraries from the btcsuite project. Aims to implement Namecoin protocol specifications from Namecoin Core.

---

**Status:** 🚧 EXPERIMENTAL – Not ready for production use. For production Namecoin operations, use Namecoin Core.
