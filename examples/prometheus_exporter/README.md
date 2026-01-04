# Prometheus Exporter Example

This example demonstrates how to use the Prometheus metrics exporter in nmcd to expose metrics for monitoring and alerting.

## Overview

nmcd includes a built-in Prometheus exporter that exposes 32 comprehensive metrics covering:
- Block processing (processed, accepted, orphaned, rejected)
- Name operations (NAME_NEW, NAME_FIRSTUPDATE, NAME_UPDATE)
- Network activity (peers, connections, disconnections)
- Transactions (processed, mempool size)
- Validation errors (AuxPoW, subsidy, name theft attempts, double-spends)
- Performance (block processing time, uptime)

## Running the Example

```bash
cd examples/prometheus_exporter
go run main.go
```

The example will:
1. Simulate some nmcd activity (block processing, name operations, peer connections)
2. Start a Prometheus HTTP server on port 9100
3. Display a sample of the metrics in Prometheus text format
4. Run for 5 seconds then exit

## Accessing Metrics

While the example is running, you can access the metrics at:
```
http://127.0.0.1:9100/metrics
```

## Using with nmcd Daemon

To enable Prometheus metrics in the nmcd daemon:

```bash
# Enable metrics on default port 9100
./nmcd -prometheusaddr=127.0.0.1:9100

# Or use a custom port
./nmcd -prometheusaddr=127.0.0.1:9999
```

## Prometheus Configuration

Add this to your `prometheus.yml` to scrape nmcd metrics:

```yaml
scrape_configs:
  - job_name: 'nmcd'
    static_configs:
      - targets: ['localhost:9100']
    scrape_interval: 15s
```

## Example Queries

Once metrics are being scraped by Prometheus, you can query them:

```promql
# Block processing rate (blocks/second)
rate(nmcd_blocks_processed_total[5m])

# Current peer count
nmcd_peers_connected

# Name operations rate
rate(nmcd_name_operations_total[1h])

# Validation error rate
rate(nmcd_validation_errors_total[5m])

# Average block processing time
nmcd_avg_block_process_time_seconds
```

## Grafana Dashboard

Create a Grafana dashboard with these recommended panels:

1. **Block Processing Rate**: Graph showing `rate(nmcd_blocks_processed_total[5m])`
2. **Peer Connections**: Graph showing `nmcd_peers_connected`, `nmcd_inbound_peers`, `nmcd_outbound_peers`
3. **Name Operations**: Stacked area chart with:
   - `rate(nmcd_name_new_total[5m])`
   - `rate(nmcd_name_firstupdate_total[5m])`
   - `rate(nmcd_name_update_total[5m])`
4. **Validation Errors**: Graph showing `rate(nmcd_validation_errors_total[5m])`
5. **Mempool Size**: Graph showing `nmcd_txs_in_mempool`
6. **Uptime**: Single stat showing `nmcd_uptime_seconds`

## Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
  - name: nmcd_alerts
    rules:
      # Alert if no blocks processed in 30 minutes
      - alert: NmcdNoBlocksProcessed
        expr: rate(nmcd_blocks_processed_total[30m]) == 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "nmcd not processing blocks"

      # Alert if no peers connected
      - alert: NmcdNoPeers
        expr: nmcd_peers_connected == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "nmcd has no peer connections"

      # Alert on high validation error rate
      - alert: NmcdHighValidationErrors
        expr: rate(nmcd_validation_errors_total[5m]) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High validation error rate detected"
```

## Implementation Details

The Prometheus exporter uses the `prometheus.Collector` interface to expose metrics efficiently:
- Zero-copy metric export (metrics are read directly from internal state)
- Thread-safe (uses RWMutex for concurrent access)
- Standard Prometheus text format
- Proper metric types (Counter for monotonic values, Gauge for current state)

See `metrics/prometheus.go` for the implementation.
