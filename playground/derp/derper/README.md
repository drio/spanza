# DERP Server Setup

This directory contains the setup for running the Tailscale DERP server
for testing purposes.

## Quick Start (Development Mode)

```bash
make run-dev
```

This runs the server on `http://localhost:3340` without TLS.

## Available Make Targets

- `make build` - Build the derper binary
- `make run-dev` - Run in development mode (port 3340, no TLS)
- `make clean` - Remove built binary
- `make test-connection` - Test if server is running
- `make help` - Show all targets

## Development Mode vs Production Mode

### Development Mode (`--dev` flag)

Used for local testing:
- **Port**: 3340 (plain HTTP)
- **TLS**: Disabled
- **Key**: Ephemeral (generated on startup)
- **Use case**: Local testing, development

Command:
```bash
./derper --dev
```

### Production Mode (Port 443 with TLS)

For real-world deployments where only HTTPS/443 is allowed:
- **Port**: 443 (HTTPS)
- **TLS**: Required (LetsEncrypt or manual certificates)
- **Key**: Persistent (stored in config file)
- **Use case**: Production relay server

## Running in Production (Port 443)

To run a DERP server in production on port 443, you need:

### 1. A Public Domain Name

You need a real domain pointing to your server's IP:
```
derp.example.com → 203.0.113.10
```

### 2. TLS Certificate Options

#### Option A: LetsEncrypt (Automatic, Recommended)

```bash
./derper \
  --hostname=derp.example.com \
  --certmode=letsencrypt \
  --certdir=/var/lib/derper/certs \
  -a :443
```

**What this does:**
- Automatically obtains TLS certificate from LetsEncrypt
- Renews certificate automatically before expiration
- Stores certificates in `/var/lib/derper/certs`
- Listens on port 443 (requires root or CAP_NET_BIND_SERVICE)

**Requirements:**
- Server must be publicly accessible on port 443
- Domain must point to server's public IP
- Port 80 must also be open (for LetsEncrypt HTTP-01 challenge)

#### Option B: Manual Certificate

If you already have certificates:

```bash
./derper \
  --hostname=derp.example.com \
  --certmode=manual \
  --certdir=/path/to/certs \
  -a :443
```

Place your certificates in the cert directory:
- `derp.example.com.crt` - Certificate file
- `derp.example.com.key` - Private key file

### 3. Persistent Key Configuration

Production servers need a persistent private key (so clients can verify
the server identity across restarts):

```bash
# First run creates the key
./derper -c /var/lib/derper/derper.key --hostname=derp.example.com

# Subsequent runs use the same key
./derper -c /var/lib/derper/derper.key --hostname=derp.example.com
```

The config file (`derper.key`) stores the server's private key in JSON:
```json
{
  "PrivateKey": "privkey:..."
}
```

### 4. Running as a Service

For production, run as a systemd service:

```bash
sudo useradd -r -s /bin/false derper
sudo mkdir -p /var/lib/derper/certs
sudo chown derper:derper /var/lib/derper

# Create /etc/systemd/system/derper.service
[Unit]
Description=Tailscale DERP Server
After=network.target

[Service]
Type=simple
User=derper
ExecStart=/usr/local/bin/derper \
  -c /var/lib/derper/derper.key \
  --hostname=derp.example.com \
  --certmode=letsencrypt \
  --certdir=/var/lib/derper/certs \
  -a :443
Restart=always
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl enable derper
sudo systemctl start derper
sudo systemctl status derper
```

### 5. Firewall Configuration

Open required ports:
```bash
# HTTPS (DERP traffic)
sudo ufw allow 443/tcp

# HTTP (LetsEncrypt challenge only)
sudo ufw allow 80/tcp

# STUN (optional, for NAT traversal assistance)
sudo ufw allow 3478/udp
```

### 6. Testing the Production Server

From a client:
```bash
curl https://derp.example.com
# Should return the DERP server homepage

# Test DERP endpoint
curl https://derp.example.com/derp
# Should return "GET request for DERP websocket"
```

## How Clients Connect to Port 443

### HTTP Upgrade Method (Default)

1. Client connects to `https://derp.example.com:443`
2. TLS handshake establishes encrypted connection
3. Client sends HTTP Upgrade request:
   ```
   GET /derp HTTP/1.1
   Upgrade: DERP
   Connection: Upgrade
   ```
4. Server responds with `101 Switching Protocols`
5. Connection is "hijacked" - becomes raw TCP carrying DERP frames
6. All subsequent traffic is DERP protocol over TLS

### WebSocket Fallback

If HTTP Upgrade fails (some proxies block it), DERP falls back to WebSocket:

1. Client connects to `wss://derp.example.com:443/derp`
2. WebSocket handshake succeeds (proxies allow this)
3. Each DERP frame is sent as a WebSocket binary message
4. Server unwraps WebSocket frames to get DERP frames

This ensures DERP works even through restrictive corporate proxies.

## Why Port 443 Matters

Many restrictive networks only allow outbound connections to:
- Port 80 (HTTP)
- Port 443 (HTTPS)

By running DERP on port 443 with TLS:
- Traffic looks like normal HTTPS to firewalls/proxies
- Can traverse corporate networks
- Provides encryption in transit
- Works everywhere HTTPS works

## Summary: Dev vs Production

| Aspect | Development | Production |
|--------|-------------|------------|
| **Port** | 3340 | 443 |
| **TLS** | No | Yes (required) |
| **Certificate** | None | LetsEncrypt or manual |
| **Domain** | localhost | Real domain required |
| **Key** | Ephemeral | Persistent config file |
| **Firewall** | Local only | Must allow 80, 443 |
| **Use Case** | Testing | Real relay server |

## Next Steps

For this playground:
1. Use `make run-dev` for local testing
2. Test with clients A and B on localhost
3. Once working, consider deploying to a real server with port 443

For your Spanza relay:
1. Support both UDP (preferred) and TCP/443 (fallback)
2. Use similar TLS setup with LetsEncrypt
3. Implement HTTP upgrade or WebSocket transport
4. This gives maximum compatibility with firewalls
