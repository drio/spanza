# Production Deployment

This directory contains everything needed for production DERP server deployment.

## Directory Structure

```
prod/
├── README.md           # This file
├── setup.sh            # Production setup script
├── config/             # Server configuration (created by setup.sh)
│   └── derper.key      # Persistent server private key
└── certs/              # TLS certificates (created by LetsEncrypt)
    └── ...             # Auto-managed by derper
```

## Quick Start

### 1. Generate Server Key

```bash
cd prod
./setup.sh gen-key
```

This creates a persistent server key at `config/derper.key`.

### 2. Run in Foreground (Testing)

```bash
./setup.sh run derp.example.com
```

Runs the server on port 443 with automatic LetsEncrypt certificates.
Press Ctrl+C to stop.

### 3. Install as Systemd Service (Production)

```bash
sudo ./setup.sh install derp.example.com
sudo systemctl enable derper
sudo systemctl start derper
```

### 4. Test the Server

```bash
./setup.sh test derp.example.com
```

## Prerequisites

- **Domain name** pointing to your server's public IP
- **Ports open**: 80/tcp and 443/tcp
- **Root access** (for port 443 and systemd)

## Commands

### Generate Key
```bash
./setup.sh gen-key
```

Generates a persistent server private key. This key identifies your DERP
server to clients.

### Run Server (Foreground)
```bash
./setup.sh run <hostname>
```

Runs the server in the foreground. Useful for testing before installing
as a service.

### Install Systemd Service
```bash
sudo ./setup.sh install <hostname>
```

Installs DERP as a systemd service that:
- Starts on boot
- Restarts on failure
- Runs as unprivileged `derper` user
- Has port 443 binding capability

### Uninstall Service
```bash
sudo ./setup.sh uninstall
```

Removes the systemd service. Optionally removes data and user.

### Test Server
```bash
./setup.sh test <hostname>
```

Tests that the server is responding correctly:
- HTTPS endpoint
- DERP endpoint
- TLS certificate validity

## Managing the Service

After installing with systemd:

```bash
# Start
sudo systemctl start derper

# Stop
sudo systemctl stop derper

# Restart
sudo systemctl restart derper

# Status
sudo systemctl status derper

# View logs
sudo journalctl -u derper -f

# Enable on boot
sudo systemctl enable derper

# Disable on boot
sudo systemctl disable derper
```

## File Locations

### Manual Run (via setup.sh run)
- Binary: `../derper`
- Config: `./config/derper.key`
- Certs: `./certs/`

### Systemd Service
- Binary: `/usr/local/bin/derper`
- Config: `/var/lib/derper/derper.key`
- Certs: `/var/lib/derper/certs/`
- Service: `/etc/systemd/system/derper.service`

## Firewall Configuration

Open these ports:

```bash
# UFW (Ubuntu/Debian)
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Firewalld (RHEL/CentOS)
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --reload
```

Don't forget cloud provider security groups!

## Testing with Clients

Update your client commands to use HTTPS:

```bash
# From playground/derp directory
./bin/clientB --server https://derp.example.com/derp --key <key> --echo
./bin/clientA --server https://derp.example.com/derp --key <key> --peer <pubkey>
```

## Troubleshooting

### Certificate Issues

**Error**: "acme/autocert: unable to satisfy http-01 challenge"

**Solutions**:
- Ensure port 80 is open and accessible
- Verify DNS with `dig derp.example.com`
- Check both server and cloud provider firewall rules

### Permission Denied on Port 443

**Error**: "bind: permission denied"

**Solutions**:
- Run with sudo: `sudo ./setup.sh run hostname`
- Or use systemd (handles capabilities automatically)

### View Logs

```bash
# Systemd service
sudo journalctl -u derper -f

# Manual run
# Logs go to stdout
```

## Security

### Protect the Server Key

```bash
# Manual deployment
chmod 600 ./config/derper.key

# Systemd deployment (automatic)
# /var/lib/derper/derper.key is owned by derper:derper with mode 600
```

### Non-Root Execution

The systemd service runs as the `derper` user (unprivileged) and uses
`AmbientCapabilities=CAP_NET_BIND_SERVICE` to bind to port 443.

## Updating

```bash
# Rebuild binary
cd ..
make build

# If running manually
cd prod
./setup.sh run derp.example.com

# If running as systemd service
sudo cp ../derper /usr/local/bin/derper
sudo systemctl restart derper
```

## Certificate Renewal

LetsEncrypt certificates expire after 90 days. The derper automatically
renews them before expiration. No action needed.

Check expiry:
```bash
echo | openssl s_client -connect derp.example.com:443 2>/dev/null | \
  openssl x509 -noout -dates
```

## Example Deployment

```bash
# On your server with domain derp.example.com

# 1. Build derper
cd playground/derp/derper
make build

# 2. Set up production
cd prod
./setup.sh gen-key
sudo ./setup.sh install derp.example.com

# 3. Start service
sudo systemctl enable derper
sudo systemctl start derper

# 4. Check status
sudo systemctl status derper
sudo journalctl -u derper -f

# 5. Test from another machine
./setup.sh test derp.example.com
```

## Next Steps

Once deployed:
1. Test with clients from different networks
2. Monitor logs for any issues
3. Set up monitoring/alerting
4. Consider deploying multiple DERP servers in different regions
5. Document your specific deployment for future reference
