# nmcd Installation Guide

This guide provides step-by-step instructions for installing nmcd on various platforms.

## Table of Contents

- [System Requirements](#system-requirements)
- [Installation Methods](#installation-methods)
  - [Pre-built Binaries (Recommended)](#pre-built-binaries-recommended)
  - [Docker](#docker)
  - [Build from Source](#build-from-source)
- [Platform-Specific Instructions](#platform-specific-instructions)
  - [Linux](#linux)
  - [macOS](#macos)
  - [Windows](#windows)
- [Verification](#verification)
- [Next Steps](#next-steps)

---

## System Requirements

### Hardware Requirements

**Minimum:**
- **CPU:** 2 cores (x86_64 or ARM64)
- **RAM:** 2 GB
- **Disk Space:** 10 GB free space
- **Network:** Stable internet connection for blockchain synchronization

**Recommended:**
- **CPU:** 4+ cores (x86_64 or ARM64)
- **RAM:** 4 GB
- **Disk Space:** 20+ GB free space (blockchain grows over time)
- **Network:** Broadband connection (10+ Mbps)

**Storage Notes:**
- Blockchain data grows over time (~5-10 GB for Namecoin mainnet as of 2026)
- Name database size depends on active names (typically < 500 MB)
- Additional space needed for wallet and transaction data
- SSD recommended for better performance

### Software Requirements

**All Platforms:**
- **Go:** Version 1.24.11 or later (only required for building from source)
- **Operating System:**
  - Linux: kernel 4.0+ (tested on Ubuntu 20.04+, Debian 11+, RHEL 8+, Alpine 3.17+)
  - macOS: 10.15 Catalina or later (Intel and Apple Silicon)
  - Windows: Windows 10 or later (64-bit)

**Memory Usage:**
- **Embedded Mode:** 100-500 MB during normal operation, up to 1 GB during sync
  - UTXO cache: 250 MB (configurable)
  - Name database cache: ~10 MB (10,000 entries)
  - Mempool: ~5-50 MB (depending on transaction volume)
- **Daemon Mode (client only):** ~10 MB

**Network Ports:**
- **8334:** P2P network (mainnet) - must be accessible for incoming connections
- **8336:** RPC server (mainnet) - should be localhost-only unless properly secured
- **18334/18336:** Testnet equivalents
- **18444/18445:** Regtest equivalents
- **9090:** Prometheus metrics (optional)

---

## Installation Methods

### Pre-built Binaries (Recommended)

The easiest way to install nmcd is using pre-built binaries from GitHub Releases.

#### 1. Download the Binary

Visit the [Releases page](https://github.com/opd-ai/nmcd/releases) and download the appropriate binary for your platform:

**Linux (x86_64):**
```bash
wget https://github.com/opd-ai/nmcd/releases/download/v1.0.0/nmcd-linux-amd64.tar.gz
tar -xzf nmcd-linux-amd64.tar.gz
```

**Linux (ARM64):**
```bash
wget https://github.com/opd-ai/nmcd/releases/download/v1.0.0/nmcd-linux-arm64.tar.gz
tar -xzf nmcd-linux-arm64.tar.gz
```

**macOS (Intel):**
```bash
curl -LO https://github.com/opd-ai/nmcd/releases/download/v1.0.0/nmcd-darwin-amd64.tar.gz
tar -xzf nmcd-darwin-amd64.tar.gz
```

**macOS (Apple Silicon):**
```bash
curl -LO https://github.com/opd-ai/nmcd/releases/download/v1.0.0/nmcd-darwin-arm64.tar.gz
tar -xzf nmcd-darwin-arm64.tar.gz
```

**Windows:**
```powershell
# Download from: https://github.com/opd-ai/nmcd/releases/download/v1.0.0/nmcd-windows-amd64.zip
# Extract the ZIP file
```

#### 2. Verify the Download

Each release includes SHA256 checksums:

```bash
# Linux/macOS
sha256sum -c nmcd-checksums.txt

# Or manually
sha256sum nmcd
# Compare with checksum from release page
```

#### 3. Install the Binary

**Linux/macOS:**
```bash
# Install to /usr/local/bin (system-wide)
sudo mv nmcd /usr/local/bin/
sudo chmod +x /usr/local/bin/nmcd

# Or install to ~/bin (user-only)
mkdir -p ~/bin
mv nmcd ~/bin/
chmod +x ~/bin/nmcd
# Add ~/bin to PATH if not already there
```

**Windows:**
```powershell
# Move to a permanent location (e.g., C:\Program Files\nmcd\)
# Add to PATH via System Properties > Environment Variables
```

---

### Docker

nmcd provides official Docker images for quick deployment.

#### Pull the Image

**Latest stable release:**
```bash
docker pull ghcr.io/opd-ai/nmcd:latest
```

**Specific version:**
```bash
docker pull ghcr.io/opd-ai/nmcd:v1.0.0
```

**Platform-specific:**
```bash
# AMD64
docker pull ghcr.io/opd-ai/nmcd:latest --platform linux/amd64

# ARM64 (Raspberry Pi, Apple Silicon, AWS Graviton)
docker pull ghcr.io/opd-ai/nmcd:latest --platform linux/arm64
```

#### Run the Container

**Basic usage:**
```bash
docker run -d \
  --name nmcd \
  -p 8336:8336 \
  -p 8334:8334 \
  -v nmcd-data:/data \
  ghcr.io/opd-ai/nmcd:latest
```

**With custom configuration:**
```bash
docker run -d \
  --name nmcd \
  -p 8336:8336 \
  -p 8334:8334 \
  -e NMCD_RPC_USER=myuser \
  -e NMCD_RPC_PASSWORD=mypassword \
  -e NMCD_NETWORK=mainnet \
  -v nmcd-data:/data \
  ghcr.io/opd-ai/nmcd:latest
```

**Docker Compose:**
```yaml
version: '3.8'

services:
  nmcd:
    image: ghcr.io/opd-ai/nmcd:latest
    container_name: nmcd
    ports:
      - "8336:8336"  # RPC
      - "8334:8334"  # P2P
      - "9090:9090"  # Metrics (optional)
    environment:
      - NMCD_RPC_USER=myuser
      - NMCD_RPC_PASSWORD=mypassword
      - NMCD_NETWORK=mainnet
    volumes:
      - nmcd-data:/data
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--spider", "http://localhost:8336/health"]
      interval: 30s
      timeout: 10s
      retries: 3

volumes:
  nmcd-data:
```

Save as `docker-compose.yml` and run:
```bash
docker-compose up -d
```

---

### Build from Source

Building from source gives you the latest features and allows customization.

#### Prerequisites

1. **Install Go 1.24.11 or later:**

   **Linux (Ubuntu/Debian):**
   ```bash
   # Remove old Go versions if installed via apt
   sudo apt remove golang-go
   
   # Download and install Go 1.24.11
   wget https://go.dev/dl/go1.24.11.linux-amd64.tar.gz
   sudo rm -rf /usr/local/go
   sudo tar -C /usr/local -xzf go1.24.11.linux-amd64.tar.gz
   
   # Add to PATH
   echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
   source ~/.bashrc
   ```

   **macOS:**
   ```bash
   # Using Homebrew
   brew install go@1.24
   
   # Or download from https://go.dev/dl/
   ```

   **Windows:**
   ```
   Download installer from https://go.dev/dl/go1.24.11.windows-amd64.msi
   ```

2. **Install Git:**
   ```bash
   # Linux
   sudo apt install git  # Ubuntu/Debian
   sudo yum install git  # RHEL/CentOS
   
   # macOS
   brew install git
   
   # Windows: Download from https://git-scm.com/download/win
   ```

#### Build Steps

1. **Clone the Repository:**
   ```bash
   git clone https://github.com/opd-ai/nmcd.git
   cd nmcd
   ```

2. **Verify Dependencies:**
   ```bash
   go mod download
   go mod verify
   ```

3. **Build the Binary:**
   ```bash
   # Build nmcd daemon
   go build -v -o nmcd ./cmd/nmcd
   
   # Or use Makefile
   make build
   
   # Build with version info (for releases)
   go build -v \
     -ldflags="-s -w -X main.version=v1.0.0" \
     -o nmcd \
     ./cmd/nmcd
   ```

4. **Run Tests (Optional but Recommended):**
   ```bash
   # Unit tests
   make test
   
   # With race detector
   go test -race ./...
   
   # With coverage
   go test -cover ./...
   ```

5. **Install System-Wide:**
   ```bash
   # Linux/macOS
   sudo mv nmcd /usr/local/bin/
   sudo chmod +x /usr/local/bin/nmcd
   
   # Windows: Move to desired location and add to PATH
   ```

---

## Platform-Specific Instructions

### Linux

#### Ubuntu/Debian

**1. Install from Binary:**
```bash
# Download
wget https://github.com/opd-ai/nmcd/releases/download/v1.0.0/nmcd-linux-amd64.tar.gz
tar -xzf nmcd-linux-amd64.tar.gz

# Install
sudo mv nmcd /usr/local/bin/
sudo chmod +x /usr/local/bin/nmcd

# Verify
nmcd --version
```

**2. Create System User and Directories:**
```bash
# Create dedicated user
sudo useradd -r -s /bin/false -d /var/lib/nmcd nmcd

# Create data directory
sudo mkdir -p /var/lib/nmcd
sudo chown nmcd:nmcd /var/lib/nmcd
sudo chmod 700 /var/lib/nmcd

# Create config directory
sudo mkdir -p /etc/nmcd
```

**3. Set Up as Systemd Service:**
```bash
# Copy systemd service file
sudo curl -o /etc/systemd/system/nmcd.service \
  https://raw.githubusercontent.com/opd-ai/nmcd/main/examples/systemd/nmcd.service

# Create environment file
sudo nano /etc/nmcd/nmcd.env
# Add:
# NMCD_RPC_USER=your_username
# NMCD_RPC_PASSWORD=your_secure_password

# Set permissions
sudo chmod 600 /etc/nmcd/nmcd.env
sudo chown root:root /etc/nmcd/nmcd.env

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable nmcd
sudo systemctl start nmcd

# Check status
sudo systemctl status nmcd
```

**See [examples/systemd/README.md](../examples/systemd/README.md) for detailed systemd configuration.**

#### RHEL/CentOS/Fedora

Same as Ubuntu/Debian, but use `yum` or `dnf` for package management:
```bash
# Install dependencies (if building from source)
sudo dnf install git golang

# Follow same steps as Ubuntu
```

#### Alpine Linux (Docker Base)

```bash
# Install from binary
wget https://github.com/opd-ai/nmcd/releases/download/v1.0.0/nmcd-linux-amd64.tar.gz
tar -xzf nmcd-linux-amd64.tar.gz
mv nmcd /usr/local/bin/
chmod +x /usr/local/bin/nmcd

# Alpine uses OpenRC instead of systemd
# See Alpine documentation for service configuration
```

---

### macOS

#### Install from Binary

**Intel Macs:**
```bash
curl -LO https://github.com/opd-ai/nmcd/releases/download/v1.0.0/nmcd-darwin-amd64.tar.gz
tar -xzf nmcd-darwin-amd64.tar.gz
sudo mv nmcd /usr/local/bin/
sudo chmod +x /usr/local/bin/nmcd
```

**Apple Silicon (M1/M2/M3):**
```bash
curl -LO https://github.com/opd-ai/nmcd/releases/download/v1.0.0/nmcd-darwin-arm64.tar.gz
tar -xzf nmcd-darwin-arm64.tar.gz
sudo mv nmcd /usr/local/bin/
sudo chmod +x /usr/local/bin/nmcd
```

#### macOS Security Note

On first run, macOS may block the binary because it's not code-signed. To allow it:

```bash
# First attempt to run
nmcd --version

# If blocked, allow in System Preferences:
# System Preferences > Security & Privacy > General > "Allow anyway"

# Or use command line:
sudo xattr -r -d com.apple.quarantine /usr/local/bin/nmcd
```

#### Running as Launch Daemon (Optional)

Create `~/Library/LaunchAgents/com.namecoin.nmcd.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.namecoin.nmcd</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/nmcd</string>
        <string>-datadir=/Users/YOUR_USERNAME/.nmcd</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/Users/YOUR_USERNAME/.nmcd/nmcd.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/YOUR_USERNAME/.nmcd/nmcd-error.log</string>
</dict>
</plist>
```

Load the service:
```bash
launchctl load ~/Library/LaunchAgents/com.namecoin.nmcd.plist
```

---

### Windows

#### Install from Binary

1. **Download the Windows binary:**
   - Visit: https://github.com/opd-ai/nmcd/releases/download/v1.0.0/nmcd-windows-amd64.zip
   - Extract to `C:\Program Files\nmcd\`

2. **Add to PATH:**
   - Right-click "This PC" → Properties → Advanced system settings
   - Click "Environment Variables"
   - Under "System variables", find "Path" and click "Edit"
   - Click "New" and add: `C:\Program Files\nmcd`
   - Click "OK" on all dialogs

3. **Create Data Directory:**
   ```powershell
   mkdir C:\Users\YOUR_USERNAME\.nmcd
   ```

4. **Test Installation:**
   ```powershell
   nmcd --version
   ```

#### Run as Windows Service (Optional)

Use [NSSM (Non-Sucking Service Manager)](https://nssm.cc/):

```powershell
# Download NSSM from https://nssm.cc/download
# Extract and install
nssm install nmcd "C:\Program Files\nmcd\nmcd.exe"
nssm set nmcd AppDirectory "C:\Users\YOUR_USERNAME\.nmcd"
nssm set nmcd AppParameters "-datadir=C:\Users\YOUR_USERNAME\.nmcd"
nssm start nmcd
```

---

## Verification

After installation, verify nmcd is working correctly:

### 1. Check Version

```bash
nmcd --version
```

Expected output:
```
nmcd version v1.0.0
```

### 2. Run Help Command

```bash
nmcd --help
```

You should see a list of available command-line flags.

### 3. Test Basic Functionality

**Start in regtest mode (no network required):**
```bash
mkdir -p /tmp/nmcd-test
nmcd -network=regtest -datadir=/tmp/nmcd-test
```

In another terminal, test RPC connectivity:
```bash
curl http://localhost:18445 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"getinfo","params":[]}'
```

Expected response includes blockchain info.

Stop the test daemon with `Ctrl+C` and clean up:
```bash
rm -rf /tmp/nmcd-test
```

### 4. Check System Resources

After starting nmcd, verify resource usage:

```bash
# Linux/macOS
ps aux | grep nmcd
top -p $(pidof nmcd)

# Or using systemd (Linux)
systemctl status nmcd

# Memory should be < 500 MB during normal operation
# CPU usage should stabilize at < 10% after initial sync
```

---

## Next Steps

After successful installation:

1. **Configure nmcd:**
   - See [OPERATIONS.md](OPERATIONS.md) for configuration options
   - Set up RPC authentication
   - Configure network settings

2. **Set Up Monitoring:**
   - Enable Prometheus metrics
   - Configure health checks
   - Set up log collection

3. **Backup Strategy:**
   - Back up wallet.json (contains private keys)
   - Back up blockchain data (optional, can re-sync)
   - See [OPERATIONS.md](OPERATIONS.md#backup-and-restore)

4. **Production Deployment:**
   - Use systemd on Linux for automatic restart
   - Configure resource limits
   - Set up monitoring and alerting
   - See [examples/systemd/README.md](../examples/systemd/README.md)

5. **Integration:**
   - Read [API.md](API.md) for programmatic access
   - Explore [EXAMPLES.md](EXAMPLES.md) for code samples
   - Check [EMBEDDING.md](EMBEDDING.md) for library usage

---

## Troubleshooting

### Installation Issues

**Binary won't execute:**
- **Linux/macOS:** Ensure execute permissions: `chmod +x nmcd`
- **macOS:** Remove quarantine attribute (see macOS section above)
- **Windows:** Check antivirus isn't blocking the binary

**"Go version too old" when building:**
```bash
# Verify Go version
go version

# Should be 1.24.11 or later
# Upgrade Go if necessary
```

**Build fails with dependency errors:**
```bash
# Clear module cache
go clean -modcache

# Re-download dependencies
go mod download
go mod verify

# Retry build
go build -v ./cmd/nmcd
```

**Docker image won't pull:**
```bash
# Check Docker is running
docker info

# Login to GitHub Container Registry (if private)
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# Try with explicit platform
docker pull ghcr.io/opd-ai/nmcd:latest --platform linux/amd64
```

### Startup Issues

**Port already in use:**
```bash
# Check what's using port 8336
sudo lsof -i :8336

# Or on Windows
netstat -ano | findstr :8336

# Either stop the conflicting service or use different ports:
nmcd -rpcaddr=127.0.0.1:9336
```

**Permission denied errors:**
```bash
# Ensure data directory is writable
ls -la ~/.nmcd  # Linux/macOS
# Fix permissions
chmod 700 ~/.nmcd
```

**Systemd service won't start:**
```bash
# Check service status
sudo systemctl status nmcd

# View logs
sudo journalctl -u nmcd -n 50

# Common issues:
# - Binary not found: Check ExecStart path
# - Permission errors: Check User/Group settings
# - Environment file: Verify /etc/nmcd/nmcd.env exists and has correct permissions
```

### Getting Help

- **Documentation:** https://github.com/opd-ai/nmcd/tree/main/docs
- **Issues:** https://github.com/opd-ai/nmcd/issues
- **Examples:** https://github.com/opd-ai/nmcd/tree/main/examples
- **Community:** GitHub Discussions

---

## See Also

- [OPERATIONS.md](OPERATIONS.md) - Operations and maintenance guide
- [API.md](API.md) - API reference and usage
- [EXAMPLES.md](EXAMPLES.md) - Code examples and patterns
- [examples/systemd/README.md](../examples/systemd/README.md) - Systemd deployment guide
