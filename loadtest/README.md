# Load & Stress Testing for nmcd

This directory contains load and stress testing utilities for validating nmcd's production readiness. These tests ensure the daemon can handle sustained load, detect memory leaks, and recover gracefully from failures.

## Overview

The load testing infrastructure provides:

- **RPC Load Testing**: Validates handling of concurrent RPC requests at high throughput
- **Memory Leak Detection**: Monitors memory growth over extended periods
- **Continuous Operation**: Tests daemon stability during sustained operation
- **Connection Exhaustion**: Validates handling of many concurrent connections
- **Database Corruption Testing**: Ensures DB integrity after sudden termination

## Quick Start

### Running Tests Programmatically

```go
package main

import (
    "fmt"
    "time"
    "github.com/opd-ai/nmcd/loadtest"
)

func main() {
    config := loadtest.LoadTestConfig{
        RPCURL:      "http://localhost:8336",
        RPCUser:     "testuser",
        RPCPassword: "testpass",
        Duration:    5 * time.Minute,
        Concurrency: 50,
        RateLimit:   1000, // 1000 req/s
    }

    result, err := loadtest.RPCLoadTest(config)
    if err != nil {
        panic(err)
    }

    loadtest.PrintResult(result)
}
```

### Running Tests via CLI

```bash
# Build the load test tool
make loadtest

# Run all tests with default settings (60 seconds)
./loadtest-tool

# Run RPC load test with 500 concurrent clients for 5 minutes
./loadtest-tool -test rpc -concurrency 500 -duration 5m

# Run memory leak detection for 24 hours
./loadtest-tool -test memory -duration 24h

# Run continuous operation test for 72 hours with 100k operations
./loadtest-tool -test continuous -namecount 100000 -duration 72h
```

## Test Types

### 1. RPC Load Test

Validates RPC server performance under concurrent load.

**What it tests:**
- Concurrent request handling
- Throughput capacity (requests/second)
- Request latency (average, p95, p99)
- Error rate under load

**Success Criteria:**
- Throughput ≥ 1000 req/s (with 500 concurrent clients)
- Failure rate < 1%
- P99 latency < 100ms

**Example:**
```bash
./loadtest-tool -test rpc -concurrency 500 -duration 5m -ratelimit 1000
```

### 2. Memory Leak Detection

Monitors memory usage over time to detect leaks.

**What it tests:**
- Memory growth rate during sustained operation
- Memory stability over 24+ hours
- Memory usage patterns

**Success Criteria:**
- Memory growth < 10 MB/hour over 24 hours
- No unbounded growth patterns
- Stable baseline after warmup period

**Example:**
```bash
./loadtest-tool -test memory -duration 24h
```

### 3. Continuous Operation Test

Validates daemon stability during extended runtime.

**What it tests:**
- Long-running operation without crashes
- Health check responsiveness
- Error recovery
- Request success rate

**Success Criteria:**
- No crashes during 72-hour test
- Success rate ≥ 99.9%
- Health checks always responsive
- 100,000 names processed successfully

**Example:**
```bash
./loadtest-tool -test continuous -duration 72h -namecount 100000
```

### 4. Connection Exhaustion Test

Tests handling of many concurrent peer connections.

**What it tests:**
- Maximum concurrent connections
- Connection pool management
- Resource cleanup
- Graceful degradation under pressure

**Success Criteria:**
- Handle 1000+ concurrent connections
- No connection leaks
- Graceful rejection when limits reached

**Example:**
```go
// This test requires custom implementation based on network layer
// See examples in network package tests
```

### 5. Database Corruption Test

Validates DB integrity after sudden termination.

**What it tests:**
- Database consistency after kill -9
- Write-ahead logging effectiveness
- Recovery from incomplete transactions
- Data integrity verification

**Success Criteria:**
- Database opens successfully after crash
- No data corruption detected
- All committed transactions preserved
- Incomplete transactions rolled back

**Example:**
```bash
# Run nmcd with writes
nmcd -datadir /tmp/testdata &
PID=$!

# Generate some load
./loadtest-tool -duration 30s &

# Wait a bit, then kill -9
sleep 15
kill -9 $PID

# Verify database integrity
nmcd -datadir /tmp/testdata -checkdb
```

## Test Configuration

### Environment Variables

```bash
export NMCD_RPC_URL="http://localhost:8336"
export NMCD_RPC_USER="testuser"
export NMCD_RPC_PASSWORD="testpass"
```

### Configuration File

Create `loadtest.conf`:

```toml
rpc_url = "http://localhost:8336"
rpc_user = "testuser"
rpc_password = "testpass"
duration = "5m"
concurrency = 100
rate_limit = 1000
```

## Performance Targets

Based on Phase 3 requirements from [PLAN.md](../docs/development/PLAN.md):

| Metric | Target | Test |
|--------|--------|------|
| RPC Throughput | > 1000 req/s | RPC Load Test |
| Concurrent Clients | 500 | RPC Load Test |
| Sustained Operation | 72 hours | Continuous Operation |
| Names Processed | 100,000 | Continuous Operation |
| Memory Growth | < 10 MB/hour | Memory Leak Detection |
| Connection Capacity | 1000 peers | Connection Exhaustion |
| Database Recovery | 100% | Corruption Test |

## Running Tests in CI/CD

### GitHub Actions Example

```yaml
name: Load Tests

on:
  schedule:
    - cron: '0 0 * * 0'  # Weekly

jobs:
  load-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.24'
      
      - name: Start nmcd
        run: |
          go build ./cmd/nmcd
          ./nmcd -datadir /tmp/nmcd-test &
          sleep 5
      
      - name: Run load tests
        run: |
          cd loadtest/cmd
          go build
          ./loadtest-tool -duration 5m -test all
```

## Interpreting Results

### Successful Test

```
=== RPC Load Test Results ===
Duration:         300.0s
Total Requests:   150000
Successful:       149850
Failed:           150
Throughput:       500.00 req/s
Avg Latency:      20ms
P95 Latency:      45ms
P99 Latency:      80ms

✅ PASS: All criteria met
```

### Warning Signs

```
⚠️  WARNING: Throughput 750 req/s is below target 1000 req/s
⚠️  WARNING: Failure rate 2.5% is above 1% threshold
⚠️  WARNING: Memory growth 25 MB/hour exceeds 10 MB/hour threshold
```

### Failure Indicators

- Throughput < 500 req/s
- Failure rate > 5%
- Memory growth > 50 MB/hour
- Daemon crashes during test
- Database corruption detected

## Troubleshooting

### Low Throughput

**Symptoms:** RPS < 1000 with 500 concurrent clients

**Potential Causes:**
- Network latency
- CPU bottleneck
- Lock contention
- Database I/O limits

**Solutions:**
- Check CPU usage: `top`, `htop`
- Profile with pprof: `go tool pprof http://localhost:6060/debug/pprof/profile`
- Review lock usage in hot paths
- Consider database tuning (batch writes, indexes)

### Memory Leaks

**Symptoms:** Continuous memory growth > 10 MB/hour

**Potential Causes:**
- Unclosed connections
- Cached data not evicted
- Goroutine leaks
- Large object retention

**Solutions:**
- Check goroutine count: `curl http://localhost:6060/debug/pprof/goroutine?debug=1`
- Memory profile: `go tool pprof http://localhost:6060/debug/pprof/heap`
- Review cache eviction policies
- Verify connection cleanup

### High Failure Rate

**Symptoms:** Error rate > 1%

**Potential Causes:**
- Resource exhaustion
- Timeout issues
- Database contention
- Rate limiting

**Solutions:**
- Check logs for error patterns
- Monitor system resources
- Review timeout configurations
- Adjust rate limits if needed

## Best Practices

1. **Start Small**: Begin with shorter tests (5 minutes) before running 72-hour tests
2. **Monitor Resources**: Watch CPU, memory, disk I/O during tests
3. **Isolated Environment**: Run load tests on dedicated hardware
4. **Clean State**: Start each test with fresh database and configuration
5. **Document Results**: Track performance trends over time
6. **Automate**: Integrate into CI/CD for regression detection

## Integration with Monitoring

### Prometheus Metrics

During load tests, monitor these Prometheus metrics:

```
# Request rate
rate(rpc_requests_total[5m])

# Error rate
rate(errors_total[5m])

# Memory usage
go_memstats_alloc_bytes

# Goroutine count
go_goroutines
```

### Grafana Dashboards

Create dashboards showing:
- Request throughput over time
- Latency percentiles (p50, p95, p99)
- Error rate by type
- Memory usage trend
- Goroutine count

## References

- [PLAN.md Phase 3: Testing & QA](../docs/development/PLAN.md#phase-3-testing--quality-assurance-estimated-6-7-days)
- [Go Testing Best Practices](https://golang.org/doc/effective_go#testing)
- [Prometheus Monitoring](https://prometheus.io/docs/introduction/overview/)

## Contributing

To add new load tests:

1. Add test function to `runner.go`
2. Add corresponding test to `runner_test.go`
3. Update CLI in `cmd/main.go` if needed
4. Document in this README
5. Update [PLAN.md](../docs/development/PLAN.md) success criteria if applicable

## License

See [LICENSE](../LICENSE) file in repository root.
