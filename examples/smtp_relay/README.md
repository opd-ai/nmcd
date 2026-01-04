# SMTP Relay Example

This example demonstrates how to use the nmcd mail relay to forward email from .bit addresses to real email addresses.

## Overview

The SMTP relay:
1. Listens for incoming SMTP connections
2. Accepts mail addressed to .bit domains (e.g., alice@mail.bit)
3. Resolves the .bit address using Namecoin to find the real email address
4. Forwards the mail to an upstream SMTP server for delivery

## Building

```bash
cd examples/smtp_relay
go build
```

## Usage

### Basic Usage

Forward .bit emails using Gmail as the upstream SMTP server:

```bash
# Set credentials via environment variables (recommended for security)
export SMTP_USER="your-email@gmail.com"
export SMTP_PASSWORD="your-app-password"

./smtp_relay \
  -listen=":2525" \
  -upstream-host="smtp.gmail.com" \
  -upstream-port=587
```

**Security Note:** Always use environment variables for passwords instead of command-line arguments. Passwords in command-line arguments are visible in process listings and logs.

### Configuration Options

- `-listen`: Address to listen on for incoming SMTP connections (default: `:2525`)
- `-upstream-host`: Upstream SMTP server hostname (required)
- `-upstream-port`: Upstream SMTP server port (default: `587`)
- `-upstream-user`: Upstream SMTP username (env: `SMTP_USER`) - environment variable preferred
- `-cache-ttl`: Mail routing cache TTL duration (default: `1h`)
- `-datadir`: Namecoin data directory (default: `~/.nmcd`)
- `-network`: Network to use - mainnet, testnet, or regtest (default: `mainnet`)

**Environment Variables:**
- `SMTP_USER`: SMTP username (preferred over `-upstream-user` flag)
- `SMTP_PASSWORD`: SMTP password (always use this instead of command-line flag)

### Testing with a Local SMTP Server

For testing, you can use a local SMTP server like MailHog:

1. Install and run MailHog:
```bash
# Install MailHog (https://github.com/mailhog/MailHog)
go install github.com/mailhog/MailHog@latest

# Run MailHog (SMTP on :1025, Web UI on :8025)
MailHog
```

2. Run the relay pointing to MailHog:
```bash
./smtp_relay \
  -listen=":2525" \
  -upstream-host="localhost" \
  -upstream-port=1025
```

3. Send a test email:
```bash
# Using swaks (Swiss Army Knife SMTP tool)
swaks \
  --to alice@mail.bit \
  --from sender@example.com \
  --server localhost:2525 \
  --body "Test message"
```

4. View the email in MailHog's web interface at http://localhost:8025

## Namecoin Name Configuration

For the relay to work, names must be registered in Namecoin with email configuration:

```json
{
  "email": "alice@gmail.com",
  "backup": ["alice.backup@protonmail.com"],
  "pubkey": "base64-encoded-public-key"
}
```

Register a name with email config:

```bash
# Using nmcd client
nmcd name_new mail/alice
nmcd name_firstupdate mail/alice <rand> '{"email":"alice@gmail.com"}'
```

## Production Deployment

### Security Considerations

1. **Listen only on localhost**: Use `-listen="localhost:2525"` and put a reverse proxy (nginx) in front
2. **Use environment variables for credentials**: Never expose passwords in command-line arguments or logs
3. **Use TLS**: The relay automatically uses STARTTLS when connecting to upstream SMTP on port 587
4. **Firewall**: Block direct access to port 2525 from the internet
5. **File permissions**: Protect credential files and systemd environment files with `chmod 600`

### Recommended Setup

```bash
# Set credentials securely
export SMTP_USER="relay@example.com"
export SMTP_PASSWORD="app-specific-password"

# Run relay on localhost only
./smtp_relay \
  -listen="localhost:2525" \
  -upstream-host="smtp.gmail.com" \
  -upstream-port=587 \
  -cache-ttl=30m
```

Configure your mail server (Postfix, etc.) to forward .bit addresses to localhost:2525.

### Systemd Service Example

Create `/etc/systemd/system/nmcd-smtp-relay.service`:

```ini
[Unit]
Description=Namecoin SMTP Relay
After=network.target

[Service]
Type=simple
User=nmcd
WorkingDirectory=/opt/nmcd
EnvironmentFile=/etc/nmcd/smtp-credentials.env
ExecStart=/opt/nmcd/smtp_relay \
  -listen=localhost:2525 \
  -upstream-host=smtp.gmail.com \
  -upstream-port=587
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Create `/etc/nmcd/smtp-credentials.env` with restricted permissions:

```bash
# Create directory and file
sudo mkdir -p /etc/nmcd
sudo touch /etc/nmcd/smtp-credentials.env
sudo chmod 600 /etc/nmcd/smtp-credentials.env
sudo chown nmcd:nmcd /etc/nmcd/smtp-credentials.env

# Edit the file and add:
SMTP_USER=relay@example.com
SMTP_PASSWORD=your-secure-password-here
```

Enable and start:
```bash
sudo systemctl enable nmcd-smtp-relay
sudo systemctl start nmcd-smtp-relay
```

## Monitoring

The relay logs all connections and forwarding attempts. Check logs:

```bash
# With systemd
journalctl -u nmcd-smtp-relay -f

# Direct output
./smtp_relay ... 2>&1 | tee smtp-relay.log
```

## Troubleshooting

### "Failed to connect to upstream SMTP"

Check that the upstream host and port are correct:
```bash
# Test SMTP connection
telnet smtp.gmail.com 587
```

### "550 Only .bit addresses accepted"

The relay only accepts recipient addresses ending in `.bit`. Make sure you're sending to `user@domain.bit`.

### "lookup failed for alice: name not found"

The name doesn't exist in Namecoin or has expired. Check with:
```bash
nmcd name_show alice
```

### Routing not working

Check the router resolution directly:
```bash
# Test in Go code
resolved, err := router.Route(ctx, "alice@mail.bit")
```

## See Also

- [bridge_adapter example](../bridge_adapter) - Namecoin name resolution
- [mail_router example](../mail_router) - .bit address routing
- Main README.md - nmcd documentation
