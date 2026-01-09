# nmcd Performance Benchmarks

**Last Updated:** 2026-01-07  
**Version:** v0.1.0  
**Test Environment:** AMD EPYC 7763 64-Core Processor (2 cores for benchmarks), Linux amd64

## Executive Summary

This document contains baseline performance metrics for nmcd's core operations. All benchmarks were run using Go's built-in benchmarking framework with the race detector disabled for maximum performance measurement accuracy.

**Key Findings:**
- ✅ Name resolution: **1.15 µs** per lookup (meets < 1ms target)
- ✅ Script parsing: **9-53 ns** per script (meets < 100µs target)
- ✅ JSON-RPC parsing: **1.32 µs** per request (meets < 100µs target)
- ⚠️ Name write operations: **337 ms** per write (includes disk I/O and fsync)
- ⚠️ Listing 10,000 names: **1.97 s** (needs optimization for large datasets)

---

## Benchmark Results

### Name Database Performance

**Read Operations:**
- GetName: 1.15 µs/op (849 B/op, 24 allocs)
- GetName (concurrent): 1.17 µs/op
- GetName (not found): 1.01 µs/op

**Write Operations:**
- PutName: 337 ms/op (22.4 KB/op, 111 allocs)
- DeleteName: 426 ms/op

**UTXO Operations:**
- AddUTXO: 609 ms/op
- GetUTXO: 1.38 µs/op

### Blockchain Script Operations

- Parse NAME_NEW: 9.70 ns/op (0 allocs)
- Parse NAME_FIRSTUPDATE: 53.2 ns/op (2 allocs)
- Parse NAME_UPDATE: 49.4 ns/op (2 allocs)

### RPC Performance

- JSON Request Parsing: 1.32 µs/op (392 B/op, 10 allocs)
- Rate Limiting: ~10 ns/op (0 allocs)

---

## Running Benchmarks

```bash
# All benchmarks
go test -bench=. -benchmem ./...

# Specific package
go test -bench=. -benchmem ./namedb
go test -bench=. -benchmem ./chain
go test -bench=. -benchmem ./rpc
```

---

## Performance Targets Met

- ✅ Name resolution: < 1ms (actual: 1.15 µs - 870x better)
- ✅ Script parsing: < 100µs (actual: 9-53 ns)
- ✅ RPC throughput: > 1000 req/s (capable of ~850k req/s for reads)

See [PLAN.md](/docs/development/PLAN.md) Phase 3 for complete performance targets and success criteria.

---

## Relationship to [PLAN.md](/docs/development/PLAN.md) (Phase 3)

The benchmarks in this document correspond to the **Phase 3 performance goals** described in [PLAN.md](/docs/development/PLAN.md).
Use [PLAN.md](/docs/development/PLAN.md) as the source of truth for:

- Target latency and throughput for core operations (name lookups, name writes, RPC calls)
- Success criteria for each phase (baseline, optimization, and regression testing)
- Non-goals and out-of-scope scenarios for the current release

This PERFORMANCE document focuses on **measured behavior**, optimization guidance, and operational recommendations, while [PLAN.md](/docs/development/PLAN.md) focuses on **roadmap and acceptance criteria**.

---

## Recommended Hardware Profiles

The following profiles are intended as **practical guidance** for operators running nmcd in different environments.
These are recommendations, not hard requirements.

### Development / Local Testing

- **CPU:** 2 vCPUs (modern x86_64)  
- **RAM:** 4 GB  
- **Disk:** 10–20 GB SSD (NVMe preferred)  
- **Network:** 10 Mbps down / 2 Mbps up  

Expected behavior:
- Full chain validation may be CPU-bound during initial sync.
- Name database operations remain responsive for up to ~50k names.

### Small Production Node

- **CPU:** 4 vCPUs  
- **RAM:** 8–16 GB  
- **Disk:** 100 GB SSD or NVMe  
- **Network:** ≥ 50 Mbps symmetric recommended  

Expected behavior:
- Stable RPC latency for read-heavy workloads.
- Adequate performance for thousands of names and light update traffic.

### Research / Heavy Analysis

- **CPU:** 8+ vCPUs  
- **RAM:** 32+ GB  
- **Disk:** 500 GB NVMe, high IOPS  
- **Network:** ≥ 100 Mbps symmetric  

Expected behavior:
- Suitable for chain analysis workloads and large-scale name datasets.
- Disk throughput and fsync behavior become the main bottlenecks.

---

## Optimization Recommendations

This section summarizes actionable optimizations to improve nmcd performance while preserving correctness.

### Name Database (bbolt) Tuning

- **Use fast storage:** Prefer NVMe SSDs over HDDs; name writes are currently dominated by disk I/O and fsync latency.
- **Dedicated disk:** For heavy write workloads, place `names.db` on a dedicated disk or partition to reduce contention.
- **File system options:** Where safe and appropriate for your environment, consider:
  - Mount options that reduce unnecessary metadata writes.
  - Avoiding network filesystems (NFS, SMB) for the data directory.
- **Batching writes:** When performing many name updates:
  - Group related modifications together instead of issuing one update per RPC call.
  - Prefer batch operations in higher-level tooling (scripts/clients) to amortize fsync costs.

### RPC Layer

- **Keep RPC payloads small:** Large JSON responses (e.g., `name_list` over very large datasets) will increase latency and memory usage.
- **Reuse HTTP connections:** When building clients, reuse HTTP connections rather than opening a new TCP connection per request.
- **Limit concurrent heavy calls:** Avoid running many concurrent `name_list` or history queries; they are more resource-intensive than simple lookups.

### Concurrency and Parallelism

- **Leverage concurrent reads:** The name database and blockchain wrappers are optimized for concurrent read operations; concurrent `name_show` calls scale well on multi-core CPUs.
- **Avoid unnecessary global locks in external tooling:** When integrating nmcd into larger systems, avoid serializing all RPC calls through a single global lock in your application.

---

## Troubleshooting Performance Issues

When nmcd appears slow or unresponsive, use the following checklist to localize the bottleneck.

### 1. Identify the Symptom

- **High latency for read RPCs (e.g., `name_show`, `getinfo`)**
- **High latency for write RPCs (e.g., `name_update`)**
- **Slow startup or long initial sync**
- **Increased latency under load or during spikes**

### 2. Basic Diagnostics

- Check CPU usage: is nmcd saturating one or more cores?
- Check disk I/O: monitor IOPS and fsync latency.
- Check memory usage: ensure the system is not swapping.
- Inspect logs for warnings or errors related to database operations.

### 3. Common Causes and Remedies

**Slow name writes (NAME_FIRSTUPDATE / NAME_UPDATE)**
- Cause: Disk I/O and fsync latency in bbolt.
- Remedies:
  - Move data directory to a faster disk (prefer NVMe).
  - Reduce write frequency by batching updates where permissible.

**Slow `name_list` over large datasets**
- Cause: Walking large portions of the name database and serializing large responses.
- Remedies:
  - Use more targeted queries (e.g., `name_show`) where possible.
  - Introduce pagination or filtering in higher-level clients.

**High CPU during script parsing or validation**
- Cause: Intensive blockchain validation or custom analysis workloads.
- Remedies:
  - Increase the number of vCPUs.
  - Distribute heavy analysis across multiple nodes where feasible.

---

## Comparison with Alternative Implementations

nmcd is a **lightweight, pure Go** implementation of a Namecoin daemon that prioritizes simplicity and composability over absolute peak performance.
When evaluating performance, consider the following qualitative comparisons:

- **Namecoin Core (reference implementation)**
  - Typically more feature-complete and optimized for long-term production use.
  - Implemented in C++ with different performance characteristics, especially for I/O and memory management.
  - nmcd currently lacks features such as AuxPow support, which also affects how far it can sync on mainnet.

- **Research / Experimental Nodes**
  - nmcd is designed to be easy to read, modify, and integrate for research purposes.
  - Its performance is sufficient for many experimental and testing scenarios, especially where Go tooling and libraries are preferred.

The benchmark results in this document should be interpreted in light of these design goals.
nmcd aims for **predictable, reasonable performance** on modern hardware, while keeping the codebase small and maintainable.

---

## Future Work

Planned and potential performance improvements include:

- Reducing allocation counts in hot RPC paths.
- Exploring more efficient name listing strategies (pagination, streaming, or index structures).
- Refining database access patterns to reduce lock contention under high concurrency.
- Adding more comprehensive benchmarks for wallet and network behavior.

Any such changes should be accompanied by updated benchmarks in this document and revised targets in [PLAN.md](/docs/development/PLAN.md).
