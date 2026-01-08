# nmcd Operations Guide

This guide covers day-to-day operations, monitoring, backup, troubleshooting, and maintenance of nmcd in production environments.

## Table of Contents

- [Configuration](#configuration)
- [Running nmcd](#running-nmcd)
- [Monitoring](#monitoring)
- [Backup and Restore](#backup-and-restore)
- [Performance Tuning](#performance-tuning)
- [Troubleshooting](#troubleshooting)
- [Upgrades and Migrations](#upgrades-and-migrations)
- [Security Best Practices](#security-best-practices)

---

## Configuration

### Configuration Methods

nmcd supports multiple configuration methods with the following precedence (highest to lowest):

1. **Command-line flags** (highest priority)
2. **Environment variables**
3. **Configuration file**
4. **Built-in defaults** (lowest priority)

### Configuration File

Create a TOML configuration file at `~/.nmcd/nmcd.conf` (Linux/macOS) or `%USERPROFILE%\.nmcd\nmcd.conf` (Windows).

**Example `nmcd.conf`:**
```toml
# Network configuration
network = "mainnet"  # Options: mainnet, testnet, regtest
datadir = "/var/lib/nmcd"

# RPC server settings
rpcaddr = "127.0.0.1:8336"
rpcuser = "namecoin"
rpcpassword = "your_secure_password_here"

# Network settings
listen = "0.0.0.0:8334"
maxpeers = 125

# Prometheus metrics (optional)
prometheusaddr = "127.0.0.1:9090"

# Logging
loglevel = "info"  # Options: debug, info, warn, error
logformat = "json"  # Options: text, json
```

**System-wide configuration (Linux):**
```bash
# For systemd services
sudo mkdir -p /etc/nmcd
sudo nano /etc/nmcd/nmcd.conf
sudo chmod 600 /etc/nmcd/nmcd.conf
```

### Environment Variables

Set environment variables for configuration:

```bash
# Linux/macOS
export NMCD_NETWORK=mainnet
export NMCD_RPC_USER=namecoin
export NMCD_RPC_PASSWORD=secure_password
export NMCD_DATA_DIR=/var/lib/nmcd
export NMCD_LOG_LEVEL=info
```

**Environment file for systemd:**
```bash
# /etc/nmcd/nmcd.env
NMCD_NETWORK=mainnet
NMCD_RPC_USER=namecoin
NMCD_RPC_PASSWORD=secure_password
NMCD_DATA_DIR=/var/lib/nmcd
NMCD_LOG_LEVEL=info
NMCD_PROMETHEUS_ADDR=127.0.0.1:9090
```

Set secure permissions:
```bash
sudo chmod 600 /etc/nmcd/nmcd.env
sudo chown root:root /etc/nmcd/nmcd.env
```

### Command-Line Flags

Override any configuration with command-line flags:

```bash
nmcd \
  -network=mainnet \
  -datadir=/var/lib/nmcd \
  -rpcaddr=127.0.0.1:8336 \
  -rpcuser=namecoin \
  -rpcpassword=secure_password \
  -listen=0.0.0.0:8334 \
  -maxpeers=125 \
  -prometheusaddr=127.0.0.1:9090 \
  -loglevel=info
```

**⚠️ Security Warning:** Never use `-rpcuser` and `-rpcpassword` flags in production! They're visible in process listings. Use environment variables or config files instead.

### Key Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `-network` | mainnet | Network mode: mainnet, testnet, regtest |
| `-datadir` | ~/.nmcd | Data directory path |
| `-rpcaddr` | 127.0.0.1:8336 | RPC server address |
| `-rpcuser` | (none) | RPC authentication username |
| `-rpcpassword` | (none) | RPC authentication password |
| `-listen` | 0.0.0.0:8334 | P2P network listen address |
| `-maxpeers` | 125 | Maximum peer connections |
| `-prometheusaddr` | (none) | Prometheus metrics endpoint |
| `-loglevel` | info | Log level: debug, info, warn, error |
| `-logformat` | text | Log format: text, json |

---

## Running nmcd

### Manual Startup

**Foreground (interactive):**
```bash
nmcd -datadir=/var/lib/nmcd
```

**Background (daemon):**
```bash
# Linux/macOS
nohup nmcd -datadir=/var/lib/nmcd > nmcd.log 2>&1 &

# Save PID for later
echo $! > nmcd.pid
```

**Graceful Shutdown:**
```bash
# Send SIGTERM for graceful shutdown
kill $(cat nmcd.pid)

# Or SIGINT (Ctrl+C if running in foreground)
```

### Systemd Service (Linux)

**Recommended for production deployments.**

1. **Install service file:**
   ```bash
   sudo cp examples/systemd/nmcd.service /etc/systemd/system/
   sudo systemctl daemon-reload
   ```

2. **Configure credentials:**
   ```bash
   sudo nano /etc/nmcd/nmcd.env
   # Add NMCD_RPC_USER and NMCD_RPC_PASSWORD
   sudo chmod 600 /etc/nmcd/nmcd.env
   ```

3. **Enable and start:**
   ```bash
   sudo systemctl enable nmcd
   sudo systemctl start nmcd
   ```

4. **Check status:**
   ```bash
   sudo systemctl status nmcd
   ```

**Service Management:**
```bash
# Start service
sudo systemctl start nmcd

# Stop service
sudo systemctl stop nmcd

# Restart service
sudo systemctl restart nmcd

# Reload configuration (graceful)
sudo systemctl reload nmcd

# View logs
sudo journalctl -u nmcd -f

# View last 100 lines
sudo journalctl -u nmcd -n 100

# View logs since boot
sudo journalctl -u nmcd -b
```

See [examples/systemd/README.md](../examples/systemd/README.md) for detailed systemd configuration.

### Docker

**Run as container:**
```bash
docker run -d \
  --name nmcd \
  --restart unless-stopped \
  -p 8336:8336 \
  -p 8334:8334 \
  -e NMCD_RPC_USER=namecoin \
  -e NMCD_RPC_PASSWORD=secure_password \
  -v nmcd-data:/data \
  ghcr.io/opd-ai/nmcd:latest
```

**Container Management:**
```bash
# View logs
docker logs -f nmcd

# Stop container
docker stop nmcd

# Start container
docker start nmcd

# Restart container
docker restart nmcd

# Remove container (data preserved in volume)
docker rm nmcd

# Access container shell
docker exec -it nmcd sh
```

**Docker Compose:**
```bash
# Start services
docker-compose up -d

# Stop services
docker-compose down

# View logs
docker-compose logs -f

# Restart services
docker-compose restart
```

---

## Monitoring

### Health Checks

nmcd provides health and readiness endpoints for monitoring:

**Health Check (Liveness Probe):**
```bash
curl http://localhost:8336/health
```

Response when healthy:
```json
{
  "status": "ok",
  "block_height": 450000,
  "peers": 8
}
```

**Readiness Check:**
```bash
curl http://localhost:8336/ready
```

Response when ready (synced):
```json
{
  "status": "ready",
  "block_height": 450000,
  "peers": 8,
  "syncing": false
}
```

Response when syncing:
```json
{
  "status": "syncing",
  "block_height": 320000,
  "peers": 12,
  "syncing": true
}
```

### Prometheus Metrics

Enable Prometheus metrics with `-prometheusaddr`:

```bash
nmcd -prometheusaddr=127.0.0.1:9090
```

**Scrape metrics:**
```bash
curl http://localhost:9090/metrics
```

**Key Metrics:**

| Metric | Type | Description |
|--------|------|-------------|
| `nmcd_blocks_processed_total` | Counter | Total blocks processed |
| `nmcd_last_block_height` | Gauge | Current block height |
| `nmcd_peers_connected` | Gauge | Number of connected peers |
| `nmcd_name_operations_total` | Counter | Total name operations processed |
| `nmcd_name_new_total` | Counter | Total `NAME_NEW` operations processed |
| `nmcd_name_firstupdate_total` | Counter | Total `NAME_FIRSTUPDATE` operations processed |
| `nmcd_name_update_total` | Counter | Total `NAME_UPDATE` operations processed |
| `nmcd_txs_processed_total` | Counter | Total transactions processed |
| `nmcd_txs_in_mempool` | Gauge | Current mempool size |
| `nmcd_namedb_size_bytes` | Gauge | Name database size |
| `nmcd_rpc_requests_total{method}` | Counter | RPC requests by method |
| `nmcd_rpc_duration_seconds{method}` | Histogram | RPC request duration |
| `nmcd_errors_total{type}` | Counter | Errors by type (validation, network, database) |
| `nmcd_go_goroutines` | Gauge | Number of goroutines |
| `nmcd_go_memstats_alloc_bytes` | Gauge | Allocated memory |
| `nmcd_go_memstats_heap_inuse_bytes` | Gauge | Heap memory in use |

**Prometheus Configuration:**

Add to `/etc/prometheus/prometheus.yml`:
```yaml
scrape_configs:
  - job_name: 'nmcd'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 15s
```

Reload Prometheus:
```bash
sudo systemctl reload prometheus
```

### Grafana Dashboards

**Example Queries:**

**Block Height Over Time:**
```promql
nmcd_last_block_height
```

**Peer Count:**
```promql
nmcd_peers_connected
```

**RPC Request Rate:**
```promql
rate(nmcd_rpc_requests_total[5m])
```

**Error Rate by Type:**
```promql
rate(nmcd_errors_total[5m])
```

**Memory Usage:**
```promql
nmcd_go_memstats_heap_inuse_bytes / 1024 / 1024  # Convert to MB
```

**Database Size:**
```promql
nmcd_namedb_size_bytes / 1024 / 1024  # Convert to MB
```

### Log Monitoring

**Structured Logging:**

nmcd supports JSON logging for easy parsing:

```bash
nmcd -logformat=json -loglevel=info
```

Example JSON log:
```json
{
  "time": "2026-01-08T10:15:30Z",
  "level": "INFO",
  "component": "chain",
  "operation": "ProcessBlock",
  "msg": "Block processed successfully",
  "block_height": 450123,
  "block_hash": "000000000000..."
}
```

**Log Levels:**
- `debug`: Detailed diagnostic information
- `info`: General informational messages (default)
- `warn`: Warning messages for potential issues
- `error`: Error messages for failures

**Parse JSON logs:**
```bash
# Filter errors only
journalctl -u nmcd -o cat | jq 'select(.level=="ERROR")'

# Group by component
journalctl -u nmcd -o cat | jq -r '.component' | sort | uniq -c

# Monitor specific operation
journalctl -u nmcd -f -o cat | jq 'select(.operation=="ProcessBlock")'
```

### Alerting

**Example Prometheus Alerts:**

```yaml
groups:
  - name: nmcd_alerts
    rules:
      # Daemon down
      - alert: NmcdDown
        expr: up{job="nmcd"} == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "nmcd is down"
          description: "nmcd has been down for more than 5 minutes"

      # High error rate
      - alert: NmcdHighErrorRate
        expr: rate(nmcd_errors_total[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High error rate in nmcd"
          description: "nmcd error rate is {{ $value }} errors/sec"

      # No peers
      - alert: NmcdNoPeers
        expr: nmcd_peers_connected == 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "nmcd has no peer connections"
          description: "nmcd has been without peers for 10 minutes"

      # High memory usage
      - alert: NmcdHighMemory
        expr: nmcd_go_memstats_heap_inuse_bytes > 1073741824  # 1 GB
        for: 30m
        labels:
          severity: warning
        annotations:
          summary: "nmcd memory usage is high"
          description: "nmcd is using {{ $value | humanize }}B of memory"

      # Sync stalled
      - alert: NmcdSyncStalled
        expr: rate(nmcd_blocks_processed_total[30m]) == 0
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "nmcd blockchain sync appears stalled"
          description: "No new blocks processed in the last hour"
```

---

## Backup and Restore

### What to Back Up

**Critical (must back up):**
1. **Wallet file:** `~/.nmcd/wallet.json` or `/var/lib/nmcd/wallet.json`
   - Contains private keys for name operations
   - **Loss of this file means permanent loss of name control**
   - Encrypted with password (if encryption enabled)

**Important (recommended):**
2. **Configuration files:**
   - `nmcd.conf` - Settings and preferences
   - `nmcd.env` - Environment configuration
   - Recovery time: Minutes to reconfigure

**Optional (can be re-synced):**
3. **Blockchain data:**
   - `~/.nmcd/blocks/` or `/var/lib/nmcd/blocks/`
   - Can be re-downloaded from network (takes hours/days)
   - Backup saves re-sync time

4. **Name database:**
   - `~/.nmcd/names.db` or `/var/lib/nmcd/names.db`
   - Can be rebuilt from blockchain
   - Backup saves rebuild time (~30 minutes)

### Backup Procedures

#### Wallet Backup

**Manual backup:**
```bash
# Stop nmcd first (ensures consistency)
sudo systemctl stop nmcd

# Backup wallet
cp ~/.nmcd/wallet.json ~/wallet-backup-$(date +%Y%m%d).json

# Or for system installation
sudo cp /var/lib/nmcd/wallet.json /backup/wallet-$(date +%Y%m%d).json
sudo chmod 600 /backup/wallet-*.json

# Restart nmcd
sudo systemctl start nmcd
```

**Automated daily backup:**
```bash
#!/bin/bash
# /usr/local/bin/nmcd-backup-wallet.sh

BACKUP_DIR="/backup/nmcd"
DATA_DIR="/var/lib/nmcd"
DATE=$(date +%Y%m%d-%H%M%S)

mkdir -p "$BACKUP_DIR"

# Backup wallet (no need to stop nmcd for wallet-only backup)
cp "$DATA_DIR/wallet.json" "$BACKUP_DIR/wallet-$DATE.json"
chmod 600 "$BACKUP_DIR/wallet-$DATE.json"

# Keep only last 30 backups
find "$BACKUP_DIR" -name "wallet-*.json" -mtime +30 -delete

echo "Wallet backed up to $BACKUP_DIR/wallet-$DATE.json"
```

Add to crontab:
```bash
# Daily backup at 2 AM
0 2 * * * /usr/local/bin/nmcd-backup-wallet.sh
```

#### Full Data Backup

**Stop nmcd for consistency:**
```bash
sudo systemctl stop nmcd
```

**Backup entire data directory:**
```bash
# Create compressed backup
sudo tar -czf /backup/nmcd-full-$(date +%Y%m%d).tar.gz \
  /var/lib/nmcd

# Or use rsync for incremental backups
sudo rsync -av --delete \
  /var/lib/nmcd/ \
  /backup/nmcd-data/
```

**Restart nmcd:**
```bash
sudo systemctl start nmcd
```

**Alternative: Hot backup (no downtime):**
```bash
# Use file system snapshots (LVM, ZFS, etc.)
# Example with LVM:
sudo lvcreate -s -n nmcd-snap -L 10G /dev/vg0/nmcd
sudo mount /dev/vg0/nmcd-snap /mnt/snapshot
sudo tar -czf /backup/nmcd-$(date +%Y%m%d).tar.gz /mnt/snapshot
sudo umount /mnt/snapshot
sudo lvremove -f /dev/vg0/nmcd-snap
```

#### Remote Backup

**Secure copy to remote server:**
```bash
# Using SCP
scp ~/.nmcd/wallet.json backup-server:/backups/nmcd/

# Using rsync over SSH
rsync -av --delete -e ssh \
  /var/lib/nmcd/ \
  backup-server:/backups/nmcd/
```

**Encrypted backup:**
```bash
# Encrypt before upload
tar -czf - /var/lib/nmcd | \
  gpg --encrypt --recipient your@email.com | \
  ssh backup-server "cat > /backups/nmcd-$(date +%Y%m%d).tar.gz.gpg"
```

### Restore Procedures

#### Restore Wallet

```bash
# Stop nmcd
sudo systemctl stop nmcd

# Restore wallet from backup
sudo cp /backup/wallet-20260108.json /var/lib/nmcd/wallet.json
sudo chown nmcd:nmcd /var/lib/nmcd/wallet.json
sudo chmod 600 /var/lib/nmcd/wallet.json

# Restart nmcd
sudo systemctl start nmcd

# Verify wallet loaded
curl -u user:pass http://localhost:8336 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"listaddresses","params":[]}'
```

#### Full Data Restore

```bash
# Stop nmcd
sudo systemctl stop nmcd

# Clear existing data (if any)
sudo rm -rf /var/lib/nmcd/*

# Restore from backup
sudo tar -xzf /backup/nmcd-full-20260108.tar.gz -C /

# Or from rsync backup
sudo rsync -av /backup/nmcd-data/ /var/lib/nmcd/

# Fix permissions
sudo chown -R nmcd:nmcd /var/lib/nmcd
sudo chmod 700 /var/lib/nmcd

# Restart nmcd
sudo systemctl start nmcd

# Verify restoration
sudo systemctl status nmcd
curl http://localhost:8336/health
```

#### Disaster Recovery

**Scenario: Lost wallet file**

1. **Check backups:**
   ```bash
   ls -lh /backup/wallet-*.json
   ```

2. **If no backup exists:**
   - **Names are permanently lost** if wallet was the only copy
   - Create new wallet and generate new addresses
   - Cannot recover old name registrations

3. **Prevention:** Always maintain multiple backups (local + remote)

**Scenario: Corrupted database**

1. **Stop nmcd:**
   ```bash
   sudo systemctl stop nmcd
   ```

2. **Remove corrupted database:**
   ```bash
   sudo rm /var/lib/nmcd/names.db
   ```

3. **Restart nmcd (will rebuild):**
   ```bash
   sudo systemctl start nmcd
   # Name database will be rebuilt from blockchain (~30 minutes)
   ```

4. **Monitor rebuild:**
   ```bash
   sudo journalctl -u nmcd -f
   ```

---

## Performance Tuning

### Resource Limits

**Systemd service limits:**

Edit `/etc/systemd/system/nmcd.service`:
```ini
[Service]
# Memory
MemoryMax=4G          # Maximum memory (default: 2G)
MemoryHigh=3G         # Start throttling at 3G

# CPU
CPUQuota=400%         # Use up to 4 CPU cores (default: 200% = 2 cores)

# File descriptors
LimitNOFILE=131072    # Max open files (default: 65536)

# Core dumps (for debugging)
LimitCORE=0           # Disable core dumps (or set to unlimited for debugging)
```

Reload after changes:
```bash
sudo systemctl daemon-reload
sudo systemctl restart nmcd
```

### Database Optimization

**Name cache size:**

The name database uses an LRU cache (default: 10,000 entries). Adjust based on your workload:

```go
// For applications: Modify when creating EmbeddedClient
// Currently not exposed via config - requires code change
// See namedb/cache.go for implementation
```

**bbolt settings:**

bbolt (the underlying key-value store) uses memory-mapped files. Performance is primarily I/O bound:

- **Use SSD storage** for significantly better performance
- **Ensure sufficient RAM** for OS page cache (bbolt benefits from large page cache)
- **Regular database size:** ~100-500 MB for Namecoin mainnet

### Network Optimization

**Peer connections:**
```bash
# Increase max peers for better connectivity
nmcd -maxpeers=200

# Or decrease for resource-constrained systems
nmcd -maxpeers=50
```

**Bandwidth considerations:**
- Each peer connection uses ~5-50 KB/s
- Initial sync requires substantial bandwidth (5-10 GB download)
- Ongoing operation: ~100-500 KB/s with 125 peers

### Monitoring Performance

**Check CPU usage:**
```bash
# Linux
top -p $(pidof nmcd)

# Detailed CPU breakdown
pidstat -p $(pidof nmcd) 1
```

**Check memory usage:**
```bash
# Current memory usage
ps aux | grep nmcd

# Detailed memory breakdown
pmap $(pidof nmcd)
```

**Check disk I/O:**
```bash
# Install iotop if needed
sudo apt install iotop

# Monitor I/O
sudo iotop -p $(pidof nmcd)
```

**Check database size:**
```bash
du -sh ~/.nmcd/names.db
# Expected: 100-500 MB
```

**RPC performance:**
```bash
# Measure RPC latency
time curl -u user:pass http://localhost:8336 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"name_show","params":["d/example"]}'

# Should be < 10ms for cached names
```

### Performance Targets

Based on production testing:

| Metric | Target | Actual Performance |
|--------|--------|-------------------|
| Name resolution (cached) | < 1 ms | ~176 ns (p95) ✅ |
| Name resolution (uncached) | < 10 ms | ~1-2 ms ✅ |
| RPC throughput | > 1,000 req/s | ~17,000 req/s ✅ |
| Memory usage (normal) | < 500 MB | 100-400 MB ✅ |
| Memory usage (sync) | < 1 GB | 400-800 MB ✅ |
| Block processing | > 100 blocks/s | Varies by hardware |

---

## Troubleshooting

### Common Issues

#### nmcd Won't Start

**Symptoms:** Service fails to start or immediately exits

**Diagnosis:**
```bash
# Check service status
sudo systemctl status nmcd

# View recent logs
sudo journalctl -u nmcd -n 50

# Try running manually
sudo -u nmcd /usr/local/bin/nmcd -datadir=/var/lib/nmcd
```

**Common causes:**

1. **Port already in use:**
   ```bash
   # Check what's using port 8336
   sudo lsof -i :8336
   
   # Solution: Stop conflicting service or use different port
   ```

2. **Permission errors:**
   ```bash
   # Check data directory permissions
   ls -la /var/lib/nmcd
   
   # Fix ownership
   sudo chown -R nmcd:nmcd /var/lib/nmcd
   sudo chmod 700 /var/lib/nmcd
   ```

3. **Missing dependencies:**
   ```bash
   # Verify binary is executable
   ldd /usr/local/bin/nmcd
   
   # nmcd is statically linked, should show minimal dependencies
   ```

#### Sync Not Progressing

**Symptoms:** Block height not increasing

**Diagnosis:**
```bash
# Check peer connections
curl http://localhost:8336 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"getpeerinfo","params":[]}'

# Check sync status
curl http://localhost:8336/ready
```

**Solutions:**

1. **No peers:**
   ```bash
   # Verify network connectivity
   ping -c 3 seed.nmc.markasoftware.com
   
   # Check firewall allows outbound connections on port 8334
   sudo iptables -L OUTPUT | grep 8334
   
   # Manually add peers
   # (Note: Current implementation auto-connects via DNS seeds)
   ```

2. **Stalled at specific block:**
   ```bash
   # Check logs for validation errors
   sudo journalctl -u nmcd | grep ERROR
   
   # May indicate blockchain database corruption
   # Solution: Delete blockchain data and re-sync
   sudo systemctl stop nmcd
   sudo rm -rf /var/lib/nmcd/blocks
   sudo systemctl start nmcd
   ```

#### High Memory Usage

**Symptoms:** nmcd using > 1 GB RAM

**Diagnosis:**
```bash
# Check current memory usage
ps aux | grep nmcd

# Enable Go profiling (add to command)
nmcd -cpuprofile=cpu.prof -memprofile=mem.prof

# Analyze memory profile
go tool pprof http://localhost:6060/debug/pprof/heap
```

**Solutions:**

1. **During initial sync:** Normal to use up to 1 GB
2. **After sync:** Should stabilize at < 500 MB
3. **Memory leak suspected:**
   ```bash
   # Restart nmcd
   sudo systemctl restart nmcd
   
   # Monitor memory over 24 hours
   # If growth > 10 MB/hour, report issue
   ```

#### RPC Authentication Failures

**Symptoms:** RPC requests return 401 Unauthorized

**Diagnosis:**
```bash
# Test with curl
curl -v -u user:pass http://localhost:8336 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"getinfo","params":[]}'
```

**Solutions:**

1. **Check credentials:**
   ```bash
   # Verify environment file
   sudo cat /etc/nmcd/nmcd.env
   
   # Ensure NMCD_RPC_USER and NMCD_RPC_PASSWORD are set
   ```

2. **Verify credentials loaded:**
   ```bash
   sudo systemctl show nmcd | grep Environment
   ```

3. **Test without authentication:**
   ```bash
   # Temporarily remove credentials to isolate issue
   # If it works without auth, credential configuration is wrong
   ```

#### Database Corruption

**Symptoms:** Errors reading/writing names, unexpected crashes

**Diagnosis:**
```bash
# Check logs
sudo journalctl -u nmcd | grep "database"

# Look for errors like:
# - "checksum mismatch"
# - "unexpected fault address"
# - "corrupted database"
```

**Recovery:**
```bash
# Stop nmcd
sudo systemctl stop nmcd

# Backup corrupted database (for analysis)
sudo cp /var/lib/nmcd/names.db /tmp/names.db.corrupted

# Remove corrupted database
sudo rm /var/lib/nmcd/names.db

# Restart nmcd (will rebuild from blockchain)
sudo systemctl start nmcd

# Monitor rebuild
sudo journalctl -u nmcd -f

# Rebuild takes ~30 minutes for mainnet
```

### Performance Issues

#### Slow RPC Responses

**Expected latency:**
- Cached name lookups: < 1 ms
- Uncached name lookups: < 10 ms
- Complex queries (name_list): < 100 ms

**Diagnosis:**
```bash
# Measure latency
time curl -u user:pass http://localhost:8336 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"name_show","params":["d/example"]}'
```

**Solutions:**

1. **Slow disk:** Use SSD instead of HDD
2. **High CPU load:** Reduce maxpeers or CPU limit
3. **Memory pressure:** Increase available RAM

#### High CPU Usage

**Normal CPU usage:**
- Initial sync: 50-100% (multiple cores)
- Normal operation: 5-20%
- During block processing: Brief spikes to 50-80%

**If sustained > 80%:**
```bash
# Check what operations are consuming CPU
sudo perf top -p $(pidof nmcd)

# Or enable CPU profiling
nmcd -cpuprofile=cpu.prof
# After some time: go tool pprof cpu.prof
```

### Getting Help

If you can't resolve the issue:

1. **Gather diagnostic information:**
   ```bash
   # System info
   uname -a
   nmcd --version
   
   # Configuration
   nmcd -h  # Show config (redact passwords!)
   
   # Logs
   sudo journalctl -u nmcd -n 200 > nmcd-logs.txt
   
   # Resource usage
   ps aux | grep nmcd
   df -h ~/.nmcd
   ```

2. **Search existing issues:**
   - https://github.com/opd-ai/nmcd/issues

3. **Open new issue with:**
   - Detailed problem description
   - Steps to reproduce
   - System information
   - Relevant log excerpts
   - What you've already tried

---

## Upgrades and Migrations

### Upgrading nmcd

#### Minor/Patch Upgrades (e.g., v1.0.0 → v1.0.1 or v1.1.0)

Minor and patch upgrades maintain backward compatibility.

**Binary upgrade:**
```bash
# Stop nmcd
sudo systemctl stop nmcd

# Backup current binary
sudo cp /usr/local/bin/nmcd /usr/local/bin/nmcd.bak

# Download new version
wget https://github.com/opd-ai/nmcd/releases/download/v1.0.1/nmcd-linux-amd64.tar.gz
tar -xzf nmcd-linux-amd64.tar.gz

# Install new binary
sudo mv nmcd /usr/local/bin/
sudo chmod +x /usr/local/bin/nmcd

# Restart nmcd
sudo systemctl start nmcd

# Verify version
nmcd --version

# Check logs
sudo journalctl -u nmcd -f
```

**Docker upgrade:**
```bash
# Pull new image
docker pull ghcr.io/opd-ai/nmcd:v1.0.1

# Stop and remove old container
docker stop nmcd
docker rm nmcd

# Start new container (data preserved in volume)
docker run -d \
  --name nmcd \
  --restart unless-stopped \
  -p 8336:8336 \
  -p 8334:8334 \
  -v nmcd-data:/data \
  ghcr.io/opd-ai/nmcd:v1.0.1
```

**Rollback if needed:**
```bash
# Stop new version
sudo systemctl stop nmcd

# Restore old binary
sudo mv /usr/local/bin/nmcd.bak /usr/local/bin/nmcd

# Restart
sudo systemctl start nmcd
```

#### Major Upgrades (e.g., v1.x → v2.0)

Major upgrades may include breaking changes. **Always read the CHANGELOG before upgrading.**

**Pre-upgrade checklist:**
- [ ] Read release notes and CHANGELOG
- [ ] Back up wallet and configuration
- [ ] Back up entire data directory (optional)
- [ ] Test upgrade on non-production instance first
- [ ] Plan rollback strategy

**Example upgrade procedure:**
```bash
# 1. Backup
sudo systemctl stop nmcd
sudo tar -czf /backup/nmcd-pre-upgrade-$(date +%Y%m%d).tar.gz /var/lib/nmcd

# 2. Upgrade binary
# (same as minor upgrade)

# 3. Run migration if needed
# (check release notes for migration commands)

# 4. Update configuration
# (check for deprecated options)

# 5. Start new version
sudo systemctl start nmcd

# 6. Monitor for issues
sudo journalctl -u nmcd -f
```

### Database Migrations

Some upgrades may require database migrations:

**Check if migration needed:**
```bash
# Release notes will specify if migration is required
# Example:
# "v2.0.0 requires database migration. Run: nmcd -migrate-db"
```

**Run migration:**
```bash
# Stop service
sudo systemctl stop nmcd

# Run migration (as nmcd user)
sudo -u nmcd /usr/local/bin/nmcd -datadir=/var/lib/nmcd -migrate-db

# Restart service
sudo systemctl start nmcd
```

### Configuration Migrations

**Deprecated options:**

When options are deprecated, nmcd will log warnings:

```
WARN: Option 'oldsetting' is deprecated, use 'newsetting' instead
```

Update your configuration:
```bash
# Edit config file
sudo nano /etc/nmcd/nmcd.conf

# Or update environment file
sudo nano /etc/nmcd/nmcd.env

# Reload service
sudo systemctl restart nmcd
```

---

## Security Best Practices

### Credential Management

**Never use command-line flags for credentials in production:**
```bash
# ❌ BAD - Visible in process listings
nmcd -rpcuser=admin -rpcpassword=secret

# ✅ GOOD - Use environment file
# /etc/nmcd/nmcd.env:
# NMCD_RPC_USER=admin
# NMCD_RPC_PASSWORD=secure_random_password
```

**Generate strong passwords:**
```bash
# Generate 32-character random password
openssl rand -base64 32

# Or use pwgen
pwgen -s 32 1
```

**Secure file permissions:**
```bash
# Config files
sudo chmod 600 /etc/nmcd/nmcd.conf
sudo chmod 600 /etc/nmcd/nmcd.env

# Wallet
sudo chmod 600 /var/lib/nmcd/wallet.json

# Data directory
sudo chmod 700 /var/lib/nmcd
```

### Network Security

**RPC server:**
```bash
# Bind to localhost only (default)
nmcd -rpcaddr=127.0.0.1:8336

# Never bind to 0.0.0.0 unless behind firewall/reverse proxy
```

**Firewall rules:**
```bash
# Allow P2P (8334) from anywhere for mainnet
sudo ufw allow 8334/tcp

# Restrict RPC (8336) to localhost only
sudo ufw deny 8336/tcp
sudo ufw allow from 127.0.0.1 to any port 8336

# Or allow from specific IP
sudo ufw allow from 192.168.1.100 to any port 8336
```

**Reverse proxy for RPC (if external access needed):**

Use nginx or caddy with HTTPS:

```nginx
# /etc/nginx/sites-available/nmcd
server {
    listen 443 ssl http2;
    server_name nmcd.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://127.0.0.1:8336;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        
        # Rate limiting
        limit_req zone=nmcd_rpc burst=10 nodelay;
        
        # Basic auth (in addition to RPC auth)
        auth_basic "nmcd RPC";
        auth_basic_user_file /etc/nginx/.htpasswd;
    }
}
```

### Wallet Security

**Encrypt wallet:**
```bash
# Via RPC
curl -u user:pass http://localhost:8336 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"encryptwallet","params":["strong_passphrase"]}'

# nmcd will restart automatically
```

**Lock wallet when not in use:**
```bash
# Lock wallet (removes keys from memory)
curl -u user:pass http://localhost:8336 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"walletlock","params":[]}'

# Unlock for operations (with timeout)
curl -u user:pass http://localhost:8336 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"walletpassphrase","params":["passphrase",300]}'
# Auto-locks after 300 seconds
```

**Backup encrypted wallet:**
```bash
# Even encrypted wallets need backup
# Backup file is also encrypted
cp /var/lib/nmcd/wallet.json /backup/wallet-encrypted.json
```

### System Hardening

**Run as non-root user:**
```bash
# nmcd should never run as root
# systemd service uses User=nmcd
sudo systemctl cat nmcd | grep User
```

**Resource limits (prevent DoS):**
```bash
# Systemd limits (already in provided service file)
MemoryMax=2G
CPUQuota=200%
LimitNOFILE=65536
```

**Disable core dumps:**
```bash
# In systemd service file
LimitCORE=0
```

**Enable security features:**
```bash
# In systemd service file (already enabled)
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/nmcd
```

### Update Policy

**Stay current:**
- Subscribe to release notifications: https://github.com/opd-ai/nmcd/releases
- Security updates should be applied promptly
- Test updates in staging before production

**Security advisories:**
- Monitor: https://github.com/opd-ai/nmcd/security/advisories
- Subscribe to security mailing list (if available)

---

## See Also

- [INSTALLATION.md](INSTALLATION.md) - Installation guide
- [API.md](API.md) - API reference
- [EXAMPLES.md](EXAMPLES.md) - Code examples
- [examples/systemd/README.md](../examples/systemd/README.md) - Systemd deployment
- [CHANGELOG.md](../CHANGELOG.md) - Version history and upgrade notes
