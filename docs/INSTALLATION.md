# nmcd Installation Guide

Installation instructions for nmcd across platforms and deployment scenarios.

## Table of Contents

- [Quick Install](#quick-install)
- [System Requirements](#system-requirements)
- [Installation Methods](#installation-methods)
- [Platform-Specific Instructions](#platform-specific-instructions)
- [Verification](#verification)
- [Configuration](#configuration)
- [First Run](#first-run)
- [Troubleshooting](#troubleshooting)

---

## Quick Install

### Linux/macOS

```bash
# Download and extract
curl -LO https://github.com/opd-ai/nmcd/releases/latest/download/nmcd-linux-amd64.tar.gz
tar xzf nmcd-linux-amd64.tar.gz
sudo mv nmcd /usr/local/bin/
sudo chmod +x /usr/local/bin/nmcd

# Verify
nmcd --version
```

### From Source

```bash
git clone https://github.com/opd-ai/nmcd.git
cd nmcd
make build
sudo cp nmcd /usr/local/bin/
```

---

## System Requirements

### Minimum

- **OS:** Linux, macOS, Windows, FreeBSD
- **CPU:** 2 cores (x86_64, ARM64)
- **RAM:** 4 GB
- **Disk:** 20 GB SSD
- **Network:** 1 Mbps

### Recommended

- **CPU:** 4+ cores
- **RAM:** 8+ GB
- **Disk:** 50+ GB NVMe SSD
- **Network:** 10+ Mbps

### Software

- **Go:** 1.24.11+ (source builds only)
- **Git:** Any recent version
- **Make:** GNU Make (source builds)

---

## Installation Methods

### 1. Binary Releases (Recommended)

**Linux (x86_64):**
```bash
curl -LO https://github.com/opd-ai/nmcd/releases/latest/download/nmcd-linux-amd64.tar.gz
tar xzf nmcd-linux-amd64.tar.gz
sudo mv nmcd /usr/local/bin/
```

**Linux (ARM64):**
```bash
curl -LO https://github.com/opd-ai/nmcd/releases/latest/download/nmcd-linux-arm64.tar.gz
tar xzf nmcd-linux-arm64.tar.gz
sudo mv nmcd /usr/local/bin/
```

**macOS (Intel):**
```bash
curl -LO https://github.com/opd-ai/nmcd/releases/latest/download/nmcd-darwin-amd64.tar.gz
tar xzf nmcd-darwin-amd64.tar.gz
sudo mv nmcd /usr/local/bin/
```

**macOS (Apple Silicon):**
```bash
curl -LO https://github.com/opd-ai/nmcd/releases/latest/download/nmcd-darwin-arm64.tar.gz
tar xzf nmcd-darwin-arm64.tar.gz
sudo mv nmcd /usr/local/bin/
```

**Windows:**
```powershell
# Download from https://github.com/opd-ai/nmcd/releases/latest
# Extract nmcd.exe
# Add to PATH or move to C:\Windows\System32
```

### 2. From Source

**Prerequisites:** Go 1.24.11+, Git, Make

```bash
# Clone repository
git clone https://github.com/opd-ai/nmcd.git
cd nmcd

# Build
make build

# Install
sudo make install

# Or manually
go build -v ./cmd/nmcd
sudo cp nmcd /usr/local/bin/
```

**Build options:**
```bash
# Static binary (no dynamic linking)
CGO_ENABLED=0 go build -ldflags="-s -w" ./cmd/nmcd

# With version info
go build -ldflags="-X main.version=$(git describe --tags)" ./cmd/nmcd

# Cross-compile for Linux from macOS
GOOS=linux GOARCH=amd64 go build ./cmd/nmcd
```

### 3. Docker

```bash
# Pull image
docker pull ghcr.io/opd-ai/nmcd:latest

# Run
docker run -d \
  --name nmcd \
  -p 8336:8336 \
  -p 8334:8334 \
  -v nmcd-data:/data \
  ghcr.io/opd-ai/nmcd:latest
```

**Build from source:**
```bash
docker build -t nmcd:local .
docker run -d --name nmcd -p 8336:8336 nmcd:local
```

### 4. Package Managers

**Homebrew (macOS/Linux):**
```bash
brew tap opd-ai/tap
brew install nmcd
```

**Snap (Linux):**
```bash
sudo snap install nmcd
```

**APT (Debian/Ubuntu):**
```bash
# Add repository
curl -fsSL https://repo.nmcd.io/gpg.key | sudo gpg --dearmor -o /usr/share/keyrings/nmcd.gpg
echo "deb [signed-by=/usr/share/keyrings/nmcd.gpg] https://repo.nmcd.io/apt stable main" | \
  sudo tee /etc/apt/sources.list.d/nmcd.list

# Install
sudo apt update
sudo apt install nmcd
```

**DNF/YUM (Fedora/RHEL):**
```bash
sudo dnf config-manager --add-repo https://repo.nmcd.io/rpm/nmcd.repo
sudo dnf install nmcd
```

---

## Platform-Specific Instructions

### Linux (Debian/Ubuntu)

```bash
# Install dependencies
sudo apt update
sudo apt install -y curl tar

# Download and install
curl -LO https://github.com/opd-ai/nmcd/releases/latest/download/nmcd-linux-amd64.tar.gz
tar xzf nmcd-linux-amd64.tar.gz
sudo mv nmcd /usr/local/bin/
sudo chmod +x /usr/local/bin/nmcd

# Create service user
sudo useradd -r -s /bin/false nmcd
sudo mkdir -p /var/lib/nmcd
sudo chown nmcd:nmcd /var/lib/nmcd

# Install systemd service
sudo curl -L https://raw.githubusercontent.com/opd-ai/nmcd/main/examples/systemd/nmcd.service \
  -o /etc/systemd/system/nmcd.service
sudo systemctl daemon-reload
sudo systemctl enable nmcd
sudo systemctl start nmcd
```

### macOS

```bash
# Using Homebrew (recommended)
brew install nmcd

# Or manual install
curl -LO https://github.com/opd-ai/nmcd/releases/latest/download/nmcd-darwin-arm64.tar.gz
tar xzf nmcd-darwin-arm64.tar.gz
sudo mv nmcd /usr/local/bin/

# Create launch daemon (optional)
sudo curl -L https://raw.githubusercontent.com/opd-ai/nmcd/main/examples/launchd/io.nmcd.plist \
  -o /Library/LaunchDaemons/io.nmcd.plist
sudo launchctl load /Library/LaunchDaemons/io.nmcd.plist
```

### Windows

1. Download from [releases page](https://github.com/opd-ai/nmcd/releases/latest)
2. Extract `nmcd.exe` to `C:\Program Files\nmcd\`
3. Add to PATH: System Properties → Environment Variables → Path → Add `C:\Program Files\nmcd`
4. Run from Command Prompt or PowerShell

**Windows Service:**
```powershell
# Install NSSM (service manager)
choco install nssm

# Create service
nssm install nmcd "C:\Program Files\nmcd\nmcd.exe"
nssm set nmcd AppDirectory "C:\Users\%USERNAME%\.nmcd"
nssm set nmcd AppParameters "-network=mainnet"
nssm start nmcd
```

### FreeBSD

```bash
# Install Go
pkg install go

# Build from source
git clone https://github.com/opd-ai/nmcd.git
cd nmcd
make build
install -m 755 nmcd /usr/local/bin/

# Create rc.d script
fetch -o /usr/local/etc/rc.d/nmcd \
  https://raw.githubusercontent.com/opd-ai/nmcd/main/examples/freebsd/nmcd.rc
chmod +x /usr/local/etc/rc.d/nmcd

# Enable and start
sysrc nmcd_enable="YES"
service nmcd start
```

---

## Verification

### Check Installation

```bash
# Version
nmcd --version

# Help
nmcd --help

# Test RPC (after starting)
curl -u user:pass http://localhost:8336/rpc \
  -d '{"method":"getinfo","params":[],"id":1}'
```

### Verify Binary

```bash
# Check GPG signature (if provided)
gpg --verify nmcd-linux-amd64.tar.gz.asc nmcd-linux-amd64.tar.gz

# Check SHA256
sha256sum nmcd-linux-amd64.tar.gz
# Compare with published checksums
```

---

## Configuration

### Initial Setup

```bash
# Create data directory
mkdir -p ~/.nmcd

# Create configuration
cat > ~/.nmcd/nmcd.conf << EOF
network = "mainnet"
rpcuser = "namecoin"
rpcpassword = "$(openssl rand -base64 32)"
prometheusaddr = "127.0.0.1:9090"
