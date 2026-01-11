# Development Documentation

This directory contains development artifacts, audit reports, and implementation summaries that document the development history and internal architecture of nmcd. These files are primarily useful for contributors and maintainers.

## Contents

### Audit and Compliance Reports

- **[PROTOCOL_COMPLIANCE_AUDIT.md](PROTOCOL_COMPLIANCE_AUDIT.md)** - Detailed audit of Namecoin protocol compliance (95% complete, last updated: January 11, 2026)

### Implementation Summaries

- **[AUXPOW_PROGRESS.md](AUXPOW_PROGRESS.md)** - AuxPow (merged mining) implementation progress report
- **[AUXPOW_TESTING_SUMMARY.md](AUXPOW_TESTING_SUMMARY.md)** - AuxPow mainnet testing infrastructure implementation
- **[CONCURRENCY_IMPROVEMENTS.md](CONCURRENCY_IMPROVEMENTS.md)** - Concurrency optimization implementation summary
- **[COVERAGE.md](COVERAGE.md)** - Test coverage analysis and improvement roadmap
- **[FUZZING_SUMMARY.md](FUZZING_SUMMARY.md)** - Fuzzing implementation for security testing
- **[IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md)** - Phase 2 foundation implementation (embedded client)
- **[MEMORY_OPTIMIZATION.md](MEMORY_OPTIMIZATION.md)** - Memory optimization implementation summary
- **[NETWORK_OPTIMIZATION_SUMMARY.md](NETWORK_OPTIMIZATION_SUMMARY.md)** - Network performance optimization summary
- **[SUBSIDY_VERIFICATION.md](SUBSIDY_VERIFICATION.md)** - Block subsidy calculation verification against Namecoin Core
- **[TRANSACTION_RELAY_RESOLUTION.md](TRANSACTION_RELAY_RESOLUTION.md)** - Transaction relay implementation details
- **[UTXO_RESTORATION.md](UTXO_RESTORATION.md)** - UTXO restoration during blockchain reorganizations

### Planning and Roadmap

- **[PLAN.md](PLAN.md)** - Production readiness plan and development roadmap
- **[PROTOCOL_COMPLIANCE_PLAN.md](PROTOCOL_COMPLIANCE_PLAN.md)** - Plan to achieve 100% Namecoin protocol compliance (current: 95%)

## For Users

If you're looking for user-facing documentation, see:

- **[/README.md](../../README.md)** - Main project documentation
- **[/docs/](../)** - User guides (Installation, Operations, API, Examples, etc.)
- **[/CHANGELOG.md](../../CHANGELOG.md)** - Version history and API stability policy

## For Contributors

These documents provide valuable context for understanding:

- **Design decisions** - Why certain approaches were chosen
- **Implementation details** - How complex features were built
- **Known limitations** - Current constraints and areas for improvement
- **Testing strategies** - How critical features are verified
- **Protocol compliance** - Status of Namecoin protocol implementation

When working on nmcd, these documents can help you understand the rationale behind the current implementation and identify areas that may need attention.
