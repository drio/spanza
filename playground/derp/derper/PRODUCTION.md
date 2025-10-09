# DERP Production Deployment Guide

This guide walks you through deploying a DERP server in production on port
443 with automatic TLS certificates.

## Prerequisites

1. **A Linux server** (Ubuntu/Debian recommended)
2. **A domain name** pointing to your server's public IP
   - Example: `derp.example.com` → `203.0.113.10`
3. **Firewall rules** allowing:
   - Port 80/tcp (for LetsEncrypt HTTP-01 challenge)
   - Port 443/tcp (for DERP over HTTPS)
   - Port 3478/udp (optional, for STUN)

## Quick Start

### Option 1: Manual Testing (Foreground)

For quick testing before setting up systemd:

```bash
# 1. Build the derper binary
make build

# 2. Generate a persistent key
make gen-prod-key

# 3. Run in production mode (requires sudo for port 443)
make run-prod HOSTNAME=derp.example.com
```

The server will:
- Listen on port 443
- Automatically obtain a LetsEncrypt certificate
- Store certs in `./prod-config/certs/`
- Use persistent key from `./prod-config/derper.key`

Press Ctrl+C to stop.

### Option 2: Systemd Service (Recommended)

For production deployment that survives reboots:

```bash
# 1. Install as systemd service
sudo make install-systemd HOSTNAME=derp.example.com

# 2. Enable and start the service
sudo systemctl enable derper
sudo systemctl start derper

# 3. Check status
sudo systemctl status derper

# 4. View logs
sudo journalctl -u derper -f
```

## Configuration Details

### File Locations (Systemd)

| File | Purpose |
|------|---------|
| `/usr/local/bin/derper` | Binary |
| `/var/lib/derper/derper.key` | Persistent server private key |
| `/var/lib/derper/certs/` | LetsEncrypt certificates |
| `/etc/systemd/system/derper.service` | Systemd unit file |

### File Locations (Manual)

| File | Purpose |
|------|---------|
| `./derper` | Binary |
| `./prod-config/derper.key` | Persistent server private key |
| `./prod-config/certs/` | LetsEncrypt certificates |

## Firewall Configuration

### UFW (Ubuntu/Debian)

```bash
# Allow HTTPS (DERP traffic)
sudo ufw allow 443/tcp

# Allow HTTP (LetsEncrypt challenges)
sudo ufw allow 80/tcp

# Allow STUN (optional)
sudo ufw allow 3478/udp

# Enable firewall
sudo ufw enable
```

### Firewalld (RHEL/CentOS/Fedora)

```bash
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --permanent --add-port=3478/udp
sudo firewall-cmd --reload
```

### Cloud Providers (AWS/GCP/Azure)

Don't forget to open these ports in your cloud provider's security group/
firewall rules as well!

## Testing the Production Server

### 1. Test HTTPS Endpoint

```bash
curl https://derp.example.com
```

Should return HTML page describing the DERP server.

### 2. Test DERP Endpoint

```bash
curl https://derp.example.com/derp
```

Should return "GET request for DERP websocket".

### 3. Test with Clients

Update your client commands to use HTTPS:

```bash
# Client B (receiver)
./clientB --server https://derp.example.com/derp --key <key> --echo

# Client A (sender)
./clientA --server https://derp.example.com/derp --key <key> --peer <pubkey>
```

## Troubleshooting

### Certificate Issues

**Problem**: "acme/autocert: unable to satisfy http-01 challenge"

**Solutions**:
- Ensure port 80 is open and accessible
- Verify DNS: `dig derp.example.com` should return your server IP
- Check firewall rules on both server and cloud provider
- Ensure no other service is using port 80

### Port 443 Permission Denied

**Problem**: "bind: permission denied" on port 443

**Solutions**:
- Run with `sudo` (manual mode)
- Or grant capability: `sudo setcap cap_net_bind_service=+ep /usr/local/bin/derper`
- Systemd handles this with `AmbientCapabilities=CAP_NET_BIND_SERVICE`

### Check Logs

```bash
# Systemd service
sudo journalctl -u derper -f

# Manual run
# Logs go to stdout
```

### Verify Certificate Renewal

LetsEncrypt certificates expire after 90 days. The derper will automatically
renew them. Check the certificate expiry:

```bash
echo | openssl s_client -connect derp.example.com:443 2>/dev/null | \
  openssl x509 -noout -dates
```

## Managing the Service

```bash
# Start
sudo systemctl start derper

# Stop
sudo systemctl stop derper

# Restart
sudo systemctl restart derper

# Status
sudo systemctl status derper

# Enable on boot
sudo systemctl enable derper

# Disable on boot
sudo systemctl disable derper

# View logs
sudo journalctl -u derper -f

# View logs since boot
sudo journalctl -u derper -b
```

## Updating the Server

```bash
# 1. Rebuild binary
cd /path/to/playground/derp/derper
make build

# 2. Copy new binary
sudo cp ./derper /usr/local/bin/derper

# 3. Restart service
sudo systemctl restart derper

# 4. Verify
sudo systemctl status derper
```

## Security Considerations

### 1. Server Key Protection

The server's private key (`derper.key`) authenticates the server. Protect it:

```bash
# Set restrictive permissions (systemd)
sudo chmod 600 /var/lib/derper/derper.key
sudo chown derper:derper /var/lib/derper/derper.key

# Manual mode
chmod 600 ./prod-config/derper.key
```

### 2. Running as Non-Root

The systemd service runs as the `derper` user, not root. This follows the
principle of least privilege.

### 3. Firewall Best Practices

- Only open required ports (80, 443, optionally 3478)
- Use a reverse proxy if you need additional security layers
- Consider rate limiting at the firewall level

## Monitoring

### Basic Health Check

```bash
# Create a simple health check script
cat > /usr/local/bin/derper-health-check.sh << 'EOF'
#!/bin/bash
if curl -sf https://derp.example.com > /dev/null; then
  echo "OK"
  exit 0
else
  echo "FAIL"
  exit 1
fi
EOF

chmod +x /usr/local/bin/derper-health-check.sh
```

### Systemd Health Integration

Add to your systemd service file:

```ini
[Service]
ExecStartPost=/usr/local/bin/derper-health-check.sh
```

## Advanced Configuration

### Custom Certificate (Non-LetsEncrypt)

If you have your own certificates:

```bash
# Place certificates
sudo cp your-cert.crt /var/lib/derper/certs/derp.example.com.crt
sudo cp your-cert.key /var/lib/derper/certs/derp.example.com.key

# Update systemd service to use --certmode=manual
sudo systemctl edit derper
```

### STUN Server

The DERP server also provides STUN (Session Traversal Utilities for NAT).
Clients can discover their public IP:port by querying the STUN endpoint on
port 3478/udp.

This is automatically enabled when you run derper.

## Uninstalling

### Systemd Service

```bash
# Stop and disable service
sudo systemctl stop derper
sudo systemctl disable derper

# Remove files
sudo rm /etc/systemd/system/derper.service
sudo rm /usr/local/bin/derper
sudo rm -rf /var/lib/derper

# Remove user
sudo userdel derper

# Reload systemd
sudo systemctl daemon-reload
```

## Next Steps

Once your production DERP server is running:

1. **Test with clients** from different networks
2. **Monitor certificate expiration** (auto-renews at ~60 days)
3. **Set up monitoring** (health checks, uptime monitoring)
4. **Consider redundancy** (multiple DERP servers in different regions)
5. **Document your setup** for future reference

## Summary

You now have a production DERP server running on port 443 with automatic
TLS certificates. This server can relay traffic between clients even when
they're behind NAT or firewalls, as long as they can reach HTTPS (port 443).

For questions or issues, check:
- DERP server logs: `sudo journalctl -u derper -f`
- Tailscale documentation: https://tailscale.com/kb/1232/derp-servers
- DERP protocol docs: https://pkg.go.dev/tailscale.com/derp
