# Systemd Service Files for nmcd

This directory contains example systemd service files and configuration for running nmcd as a system daemon on Linux.

## Files

- **nmcd.service** - Systemd unit file for running nmcd as a service
- **nmcd.conf.example** - Example configuration file (TOML format)
- **nmcd.env.example** - Example environment variables file
- **README.md** - This file

## Quick Start

### 1. Install nmcd Binary

```bash
# Build and install nmcd
make build
sudo cp nmcd /usr/local/bin/
sudo chmod +x /usr/local/bin/nmcd
```

### 2. Create System User

```bash
sudo useradd -r -s /bin/false -d /var/lib/nmcd nmcd
```

### 3. Create Data Directory

```bash
sudo mkdir -p /var/lib/nmcd
sudo chown nmcd:nmcd /var/lib/nmcd
sudo chmod 700 /var/lib/nmcd
```

### 4. Set Up Configuration

Choose one of the following methods for credential management:

#### Option A: Environment File (Recommended for systemd)

```bash
# Create config directory
sudo mkdir -p /etc/nmcd

# Copy and edit environment file
sudo cp nmcd.env.example /etc/nmcd/nmcd.env
sudo nano /etc/nmcd/nmcd.env

# Set secure permissions
sudo chmod 600 /etc/nmcd/nmcd.env
sudo chown root:root /etc/nmcd/nmcd.env
```

#### Option B: Configuration File

```bash
# Create data directory if it doesn't exist
sudo mkdir -p /var/lib/nmcd
sudo chown nmcd:nmcd /var/lib/nmcd

# Copy and edit config file
sudo cp nmcd.conf.example /var/lib/nmcd/nmcd.conf
sudo nano /var/lib/nmcd/nmcd.conf

# Set secure permissions
sudo chmod 600 /var/lib/nmcd/nmcd.conf
sudo chown nmcd:nmcd /var/lib/nmcd/nmcd.conf
```

### 5. Install Systemd Service

```bash
# Copy service file
sudo cp nmcd.service /etc/systemd/system/

# Reload systemd
sudo systemctl daemon-reload

# Enable service to start on boot
sudo systemctl enable nmcd

# Start service
sudo systemctl start nmcd
```

### 6. Verify Service

```bash
# Check service status
sudo systemctl status nmcd

# View logs
sudo journalctl -u nmcd -f

# Test RPC connection (requires credentials)
curl -u username:password http://127.0.0.1:8336 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"getinfo","params":[]}'
```

## Configuration Priority

nmcd uses the following priority order for configuration (highest to lowest):

1. **Command-line flags** - Override everything
2. **Environment variables** - Override config file
3. **Configuration file** (`~/.nmcd/nmcd.conf` or `/var/lib/nmcd/nmcd.conf`)
4. **Built-in defaults**

## Security Best Practices

### Credential Management

**Never** pass RPC credentials via command-line flags in production. They will be visible in process listings (`ps`, `top`, etc.) to all users on the system.

**Recommended approaches** (in order of preference):

1. **Environment file with systemd** (best for system services)
   - Credentials stored in `/etc/nmcd/nmcd.env`
   - Only readable by root (`chmod 600`)
   - Loaded by systemd, not visible in process listings

2. **Configuration file** (good for user services)
   - Credentials stored in `~/.nmcd/nmcd.conf`
   - Only readable by owner (`chmod 600`)
   - Not visible in process listings

3. **Environment variables** (acceptable for containers/development)
   - Set before starting nmcd: `export NMCD_RPC_USER=user`
   - Not visible in process listings
   - May be visible in container orchestration logs

### File Permissions

Always set restrictive permissions on files containing credentials:

```bash
# Environment file (systemd)
chmod 600 /etc/nmcd/nmcd.env
chown root:root /etc/nmcd/nmcd.env

# Config file (user service)
chmod 600 ~/.nmcd/nmcd.conf

# Config file (system service)
chmod 600 /var/lib/nmcd/nmcd.conf
chown nmcd:nmcd /var/lib/nmcd/nmcd.conf
```

### Network Security

- **RPC server** should only listen on localhost (`127.0.0.1`) unless you have a secure network setup
- Use a **firewall** to restrict access to port 8334 (P2P) if running a public node
- Consider using a **reverse proxy** (nginx, caddy) with HTTPS if exposing RPC externally
- Use **strong passwords** for RPC authentication (20+ random characters)

### Systemd Security Features

The provided service file includes several security hardening features:

- `NoNewPrivileges=true` - Prevents privilege escalation
- `PrivateTmp=true` - Isolates /tmp directory
- `ProtectSystem=strict` - Makes most of filesystem read-only
- `ProtectHome=true` - Makes home directories inaccessible
- `ReadWritePaths=/var/lib/nmcd` - Only data directory is writable

## Monitoring

### Systemd Status

```bash
# Service status
sudo systemctl status nmcd

# Recent logs
sudo journalctl -u nmcd -n 100

# Follow logs in real-time
sudo journalctl -u nmcd -f

# Logs since boot
sudo journalctl -u nmcd -b
```

### Prometheus Metrics

If Prometheus metrics are enabled (via `prometheusaddr` setting):

```bash
# View metrics
curl http://127.0.0.1:9090/metrics

# Example Prometheus scrape config
# Add to /etc/prometheus/prometheus.yml:
#
# scrape_configs:
#   - job_name: 'nmcd'
#     static_configs:
#       - targets: ['localhost:9090']
```

## Troubleshooting

### Service Won't Start

```bash
# Check service status
sudo systemctl status nmcd

# Check logs for errors
sudo journalctl -u nmcd -n 50

# Test binary manually
sudo -u nmcd /usr/local/bin/nmcd -datadir=/var/lib/nmcd
```

### Permission Errors

```bash
# Verify data directory ownership
ls -la /var/lib/nmcd

# Fix ownership if needed
sudo chown -R nmcd:nmcd /var/lib/nmcd
sudo chmod 700 /var/lib/nmcd
```

### RPC Authentication Errors

```bash
# Verify credentials are loaded
sudo systemctl show nmcd | grep Environment

# Check environment file exists and is readable
ls -la /etc/nmcd/nmcd.env

# Verify environment file format (should have NMCD_RPC_USER and NMCD_RPC_PASSWORD)
sudo cat /etc/nmcd/nmcd.env
```

### Port Already in Use

```bash
# Check what's using port 8336 (RPC)
sudo lsof -i :8336

# Check what's using port 8334 (P2P)
sudo lsof -i :8334
```

## Advanced Configuration

### Multiple Networks

Run separate instances for different networks (mainnet, testnet):

```bash
# Create testnet service
sudo cp /etc/systemd/system/nmcd.service /etc/systemd/system/nmcd-testnet.service

# Edit service file to use different ports and data directory
sudo nano /etc/systemd/system/nmcd-testnet.service
# Change: -network=testnet -rpcaddr=127.0.0.1:18336 -listen=0.0.0.0:18334
# Change: -datadir=/var/lib/nmcd-testnet

# Create separate environment file
sudo cp /etc/nmcd/nmcd.env /etc/nmcd/nmcd-testnet.env

# Enable and start
sudo systemctl enable nmcd-testnet
sudo systemctl start nmcd-testnet
```

### Resource Limits

Adjust resource limits in the service file:

```ini
[Service]
# Memory limit (default: 2G)
MemoryMax=4G

# CPU quota (default: 200% = 2 cores)
CPUQuota=400%

# File descriptor limit (default: 65536)
LimitNOFILE=131072
```

### Custom Log Location

If you prefer file-based logs instead of journald:

```ini
[Service]
StandardOutput=append:/var/log/nmcd/nmcd.log
StandardError=append:/var/log/nmcd/nmcd-error.log
```

Remember to create the log directory:

```bash
sudo mkdir -p /var/log/nmcd
sudo chown nmcd:nmcd /var/log/nmcd
```

## Support

For issues or questions:
- GitHub Issues: https://github.com/opd-ai/nmcd/issues
- Documentation: https://github.com/opd-ai/nmcd
