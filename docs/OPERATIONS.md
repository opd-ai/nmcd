# nmcd Operations Guide

Production operations guide for nmcd: configuration, monitoring, backup, troubleshooting, and maintenance.

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

Precedence (highest to lowest):
1. Command-line flags
2. Environment variables
3. Configuration file
4. Built-in defaults

### Configuration File

Create TOML config at `~/.nmcd/nmcd.conf`:

```toml
# Network
network = "mainnet"  # mainnet, testnet, regtest
datadir = "/var/lib/nmcd"

# RPC server
rpcaddr = "127.0.0.1:8336"
rpcuser = "namecoin"
rpcpassword = "your_secure_password"

# Network
listen = "0.0.0.0:8334"
maxpeers = 125

# Metrics (optional)
prometheusaddr = "127.0.0.1:9090"

# Logging
loglevel = "info"  # debug, info, warn, error
logformat = "json"  # text, json
```

**Secure permissions:**
```bash
sudo chmod 600 /etc/nmcd/nmcd.conf
sudo chown root:root /etc/nmcd/nmcd.conf
```

### Environment Variables

```bash
export NMCD_NETWORK=mainnet
export NMCD_RPC_USER=namecoin
export NMCD_RPC_PASSWORD=secure_password
export NMCD_DATA_DIR=/var/lib/nmcd
export NMCD_LOG_LEVEL=info
```

### Command-Line Flags

```bash
nmcd -network=mainnet -datadir=/var/lib/nmcd \
  -rpcuser=namecoin -rpcpassword=secure_password \
  -rpcaddr=127.0.0.1:8336 -listen=0.0.0.0:8334
```

**Key flags:**
- `-network`: mainnet, testnet, regtest (default: mainnet)
- `-datadir`: Data directory (default: ~/.nmcd)
- `-rpcaddr`: RPC bind address (default: 127.0.0.1:8336)
- `-rpcuser`, `-rpcpassword`: RPC authentication
- `-listen`: P2P listen address (default: :8334)
- `-maxpeers`: Max peer connections (default: 125)
- `-addpeer`: Add specific peer
- `-prometheusaddr`: Metrics endpoint (optional)

---

## Running nmcd

### Manual Startup

```bash
# Foreground
nmcd -network=mainnet -datadir=/var/lib/nmcd

# Background
nohup nmcd -network=mainnet -datadir=/var/lib/nmcd > /var/log/nmcd/nmcd.log 2>&1 &
```

### Systemd Service

**Service file** (`/etc/systemd/system/nmcd.service`):

```ini
[Unit]
Description=Namecoin Daemon (nmcd)
After=network.target

[Service]
Type=simple
User=nmcd
Group=nmcd
EnvironmentFile=/etc/nmcd/nmcd.env
ExecStart=/usr/local/bin/nmcd
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

**Setup:**
```bash
# Create user
sudo useradd -r -s /bin/false nmcd
sudo mkdir -p /var/lib/nmcd /var/log/nmcd
sudo chown nmcd:nmcd /var/lib/nmcd /var/log/nmcd

# Install service
sudo systemctl daemon-reload
sudo systemctl enable nmcd
sudo systemctl start nmcd
sudo systemctl status nmcd
```

### Docker

```bash
docker run -d \
  --name nmcd \
  -p 8336:8336 \
  -p 8334:8334 \
  -v nmcd-data:/data \
  -e NMCD_NETWORK=mainnet \
  nmcd:latest
```

**Docker Compose:**
```yaml
version: '3.8'
services:
  nmcd:
    image: nmcd:latest
    container_name: nmcd
    ports:
      - "8336:8336"
      - "8334:8334"
    volumes:
      - nmcd-data:/data
    environment:
      NMCD_NETWORK: mainnet
      NMCD_RPC_USER: namecoin
      NMCD_RPC_PASSWORD: secure_password
    restart: unless-stopped
```

### Health Checks

```bash
# Check RPC
curl -u namecoin:password http://localhost:8336/rpc \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"getinfo","params":[],"id":1}'

# Check connectivity
curl -u namecoin:password http://localhost:8336/rpc \
  -d '{"method":"getconnectioncount","params":[],"id":1}'

# Prometheus metrics
curl http://localhost:9090/metrics
```

---

## Monitoring

### Prometheus Metrics

nmcd exports 32 metrics on `/metrics` endpoint when `-prometheusaddr` is set.

**Key metrics:**

```
# RPC
nmcd_rpc_requests_total{method="getinfo"}
nmcd_rpc_request_duration_seconds{method="getinfo"}
nmcd_rpc_errors_total{method="getinfo"}

# Blockchain
nmcd_blockchain_height
nmcd_blockchain_best_hash
nmcd_blockchain_difficulty

# P2P
nmcd_network_peers_connected
nmcd_network_bytes_received_total
nmcd_network_bytes_sent_total

# Mempool
nmcd_mempool_transactions
nmcd_mempool_size_bytes

# Name database
nmcd_namedb_names_count
nmcd_namedb_size_bytes
```

**Example queries:**

```promql
# Request rate
rate(nmcd_rpc_requests_total[5m])

# Error rate
rate(nmcd_rpc_errors_total[5m]) / rate(nmcd_rpc_requests_total[5m])

# P2P traffic
rate(nmcd_network_bytes_received_total[5m])

# Mempool growth
delta(nmcd_mempool_transactions[1h])
```

### Alerting Rules

```yaml
groups:
  - name: nmcd
    rules:
      - alert: NmcdDown
        expr: up{job="nmcd"} == 0
        for: 5m
        
      - alert: NmcdNoPeers
        expr: nmcd_network_peers_connected < 3
        for: 15m
        
      - alert: NmcdHighErrorRate
        expr: rate(nmcd_rpc_errors_total[5m]) / rate(nmcd_rpc_requests_total[5m]) > 0.1
        for: 10m
        
      - alert: NmcdMempoolFull
        expr: nmcd_mempool_transactions > 4500
        for: 30m
```

### Log Monitoring

```bash
# Follow logs
journalctl -u nmcd -f

# Error logs
journalctl -u nmcd -p err

# JSON format
journalctl -u nmcd -o json-pretty
```

**Key log patterns:**
- `"level":"error"` - Errors requiring attention
- `"msg":"peer connected"` - Network activity
- `"msg":"new block"` - Block sync progress
- `"msg":"transaction relayed"` - Mempool activity

---

## Backup and Restore

### Data Directory Structure

```
~/.nmcd/
├── names.db         # Name database (bbolt)
├── wallet.json      # Wallet (encrypted)
├── blocks/          # Block storage
└── chainstate/      # Blockchain state
```

### Backup Strategy

**1. Name Database (Critical):**
```bash
# Stop nmcd
sudo systemctl stop nmcd

# Backup
cp ~/.nmcd/names.db ~/.nmcd/names.db.backup
tar czf nmcd-names-$(date +%Y%m%d).tar.gz ~/.nmcd/names.db

# Restart
sudo systemctl start nmcd
```

**2. Full Backup:**
```bash
sudo systemctl stop nmcd
tar czf nmcd-full-$(date +%Y%m%d).tar.gz ~/.nmcd/
sudo systemctl start nmcd
```

**3. Hot Backup (bbolt):**
```go
// Use bbolt's Backup method for consistent snapshots
db.View(func(tx *bolt.Tx) error {
    return tx.CopyFile("/backup/names.db", 0600)
})
```

### Restore

```bash
# Stop service
sudo systemctl stop nmcd

# Restore
tar xzf nmcd-full-20260110.tar.gz -C ~/

# Verify permissions
chmod 700 ~/.nmcd
chmod 600 ~/.nmcd/names.db ~/.nmcd/wallet.json

# Start service
sudo systemctl start nmcd
```

### Disaster Recovery

**Wallet recovery:**
```bash
# From mnemonic/seed phrase
nmcd-wallet recover --mnemonic "your twelve word phrase..."

# From private keys
nmcd-wallet import-key <WIF_private_key>
```

**Blockchain re-sync:**
```bash
# Remove corrupted data
rm -rf ~/.nmcd/blocks ~/.nmcd/chainstate

# Keep names and wallet
# Restart - will re-sync from genesis
```

---

## Performance Tuning

### System Requirements

**Minimum:**
- CPU: 2 cores
- RAM: 4 GB
- Disk: 20 GB SSD
- Network: 1 Mbps

**Recommended:**
- CPU: 4+ cores
- RAM: 8+ GB
- Disk: 50+ GB NVMe SSD
- Network: 10+ Mbps

### Database Optimization

```toml
# Increase bbolt page size for better performance
[database]
pagesize = 16384  # Default: 4096
```

### Network Tuning

```toml
maxpeers = 125      # More peers = better relay
maxinbound = 100
maxoutbound = 25
```

**Linux kernel tuning:**
```bash
# /etc/sysctl.conf
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216
net.ipv4.tcp_congestion_control = bbr
```

### File Descriptors

```bash
# /etc/security/limits.conf
nmcd soft nofile 65536
nmcd hard nofile 65536
```

---

## Troubleshooting

### Common Issues

**1. Cannot connect to peers**
```bash
# Check network
nmcd-cli getconnectioncount

# Check firewall
sudo ufw allow 8334/tcp

# Add bootstrap peers
nmcd -addpeer=seed.nmc.markasoftware.com
```

**2. RPC connection refused**
```bash
# Check binding
netstat -an | grep 8336

# Verify credentials
curl -u user:pass http://localhost:8336/rpc ...

# Check firewall (if remote access needed - NOT recommended)
sudo ufw allow from trusted_ip to any port 8336
```

**3. High memory usage**
```bash
# Check metrics
curl http://localhost:9090/metrics | grep memory

# Reduce cache sizes (if implemented)
# Reduce max peers
nmcd -maxpeers=50
```

**4. Database corruption**
```bash
# Restore from backup
sudo systemctl stop nmcd
cp ~/.nmcd/names.db.backup ~/.nmcd/names.db
sudo systemctl start nmcd
```

**5. Slow sync**
- Verify SSD usage (not HDD)
- Check network bandwidth
- Increase peer count
- Check CPU/RAM availability

### Debug Mode

```bash
# Enable debug logging
nmcd -loglevel=debug

# Specific subsystem debugging
nmcd -loglevel=debug -debugsubsystem=rpc,network
```

### Recovery Procedures

**Corrupted blockchain:**
```bash
rm -rf ~/.nmcd/blocks ~/.nmcd/chainstate
# Re-sync from network
```

**Lost wallet:**
- Restore from backup
- Restore from mnemonic (if recorded)
- Keys cannot be recovered without backup

---

## Upgrades and Migrations

### Version Upgrades

**1. Prepare:**
```bash
# Backup
tar czf nmcd-pre-upgrade-$(date +%Y%m%d).tar.gz ~/.nmcd/

# Note current version
nmcd --version
```

**2. Upgrade:**
```bash
# Stop service
sudo systemctl stop nmcd

# Install new version
sudo cp nmcd /usr/local/bin/nmcd
sudo chmod +x /usr/local/bin/nmcd

# Start service
sudo systemctl start nmcd

# Verify
nmcd --version
journalctl -u nmcd -f
```

**3. Rollback (if needed):**
```bash
sudo systemctl stop nmcd
sudo cp nmcd.old /usr/local/bin/nmcd
tar xzf nmcd-pre-upgrade-*.tar.gz -C ~/
sudo systemctl start nmcd
```

### Database Migrations

Check CHANGELOG.md for breaking database changes. Some versions may require:
- Re-indexing
- Name database rebuild
- Format migrations (automatic)

### Network Migrations

**Testnet to Mainnet:**
```bash
# Separate data directories
nmcd -network=testnet -datadir=~/.nmcd-testnet
nmcd -network=mainnet -datadir=~/.nmcd-mainnet
```

---

## Security Best Practices

### Access Control

**1. RPC Security:**
- Use strong passwords (32+ characters, random)
- Bind to localhost only (127.0.0.1)
- Never expose RPC to public internet
- Use firewall for any non-local access

**2. Wallet Security:**
```bash
# Encrypted wallet
nmcd-wallet encrypt --password "strong_passphrase"

# Secure permissions
chmod 600 ~/.nmcd/wallet.json
```

**3. File Permissions:**
```bash
chmod 700 ~/.nmcd
chmod 600 ~/.nmcd/nmcd.conf
chmod 600 ~/.nmcd/wallet.json
chmod 644 ~/.nmcd/names.db
```

### Network Security

**Firewall rules:**
```bash
# P2P (allow)
sudo ufw allow 8334/tcp

# RPC (deny public, allow localhost)
sudo ufw deny 8336/tcp

# Metrics (localhost only or VPN)
sudo ufw deny 9090/tcp
```

**Reverse proxy (if remote RPC needed):**
```nginx
location /rpc {
    proxy_pass http://127.0.0.1:8336;
    proxy_ssl_verify on;
    auth_basic "Restricted";
    auth_basic_user_file /etc/nginx/.htpasswd;
}
```

### Monitoring & Auditing

```bash
# Regular health checks
* */4 * * * curl -sf http://localhost:9090/metrics > /dev/null || systemctl restart nmcd

# Log rotation
/var/log/nmcd/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
}

# Security updates
sudo apt update && sudo apt upgrade -y
```

### Incident Response

**If compromised:**
1. Stop service immediately
2. Isolate system from network
3. Review logs for unauthorized access
4. Rotate all credentials
5. Restore from known-good backup
6. Update and patch before reconnecting

---

## See Also

- [Installation Guide](INSTALLATION.md)
- [API Documentation](API.md)
- [Examples](EXAMPLES.md)
- [Embedding Guide](EMBEDDING.md)
