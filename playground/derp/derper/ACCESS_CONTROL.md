# DERP Server Access Control

By default, a DERP server is **open to anyone** who knows the server address.
This document describes strategies for restricting access.

## The Problem

A standalone DERP server has no built-in authentication. Anyone can:
1. Connect to your DERP server
2. Register with their public key
3. Use it to relay traffic to/from other clients on your server

This means:
- **Bandwidth costs** - Others can use your bandwidth
- **Legal concerns** - You're relaying traffic you can't inspect
- **Resource exhaustion** - Malicious users could DoS your server

## Solutions

### 1. Firewall-Based Access Control (Simplest)

Restrict connections at the network level.

#### UFW Example (Ubuntu/Debian)

```bash
# Default deny
sudo ufw default deny incoming

# Allow only specific IPs
sudo ufw allow from 203.0.113.5 to any port 443
sudo ufw allow from 203.0.113.6 to any port 443

# Or allow specific subnets
sudo ufw allow from 192.168.1.0/24 to any port 443
sudo ufw allow from 10.0.0.0/8 to any port 443

sudo ufw enable
```

#### Firewalld Example (RHEL/CentOS/Fedora)

```bash
# Create rich rule for specific IPs
sudo firewall-cmd --permanent --add-rich-rule='
  rule family="ipv4" source address="203.0.113.5" port port="443" protocol="tcp" accept'

sudo firewall-cmd --reload
```

#### Cloud Provider Security Groups

On AWS/GCP/Azure, configure security groups to allow 443 only from:
- Your office IP
- Your home IP
- VPN endpoint IPs
- Known client IPs

**Pros**:
- Simple to implement
- Works immediately
- No code changes needed
- Effective against unknown attackers

**Cons**:
- Doesn't work with dynamic IPs
- Clients roaming between networks need firewall updates
- Doesn't scale to many clients

**Best for**: Small deployments, fixed client IPs

---

### 2. VPN + Private DERP

Run DERP on a private network accessible only via VPN.

```bash
# DERP binds to private interface only
./derper \
  -c /var/lib/derper/derper.key \
  --hostname=derp.internal.vpn \
  -a 10.8.0.1:443
```

Clients must:
1. Connect to VPN (WireGuard, OpenVPN, etc.)
2. Get private IP (e.g., 10.8.0.5)
3. Connect to DERP at 10.8.0.1:443

**Pros**:
- Strong network isolation
- VPN handles authentication
- Works with dynamic public IPs

**Cons**:
- Requires VPN infrastructure
- Adds latency (VPN + DERP)
- Defeats purpose if DERP is for NAT traversal

**Best for**: Corporate environments with existing VPN

---

### 3. Reverse Proxy with Client Certificates

Use nginx/Caddy with mTLS (mutual TLS) client certificate authentication.

#### Nginx Configuration

```nginx
server {
    listen 443 ssl http2;
    server_name derp.example.com;

    # Server certificate (LetsEncrypt)
    ssl_certificate /etc/letsencrypt/live/derp.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/derp.example.com/privkey.pem;

    # Client certificate verification
    ssl_client_certificate /etc/nginx/client-ca.crt;
    ssl_verify_client on;
    ssl_verify_depth 2;

    location /derp {
        # Proxy to DERP running on localhost
        proxy_pass http://127.0.0.1:3340;
        proxy_http_version 1.1;

        # Required for HTTP Upgrade
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
    }
}
```

#### Generate Client Certificates

```bash
# Create CA
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt \
  -subj "/CN=DERP Client CA"

# Generate client certificate
openssl genrsa -out client1.key 2048
openssl req -new -key client1.key -out client1.csr \
  -subj "/CN=client1"
openssl x509 -req -days 365 -in client1.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out client1.crt

# Bundle for client
cat client1.crt client1.key > client1.pem
```

#### Client Configuration

Clients must provide their certificate when connecting. You'd need to modify
your DERP client code to use the certificate.

**Pros**:
- Strong cryptographic authentication
- Standard TLS mechanism
- Scales to many clients
- Can revoke certificates (CRL)

**Cons**:
- Complex setup (PKI infrastructure)
- Requires client code changes
- Certificate distribution and management

**Best for**: Organizations with PKI expertise

---

### 4. Custom Authentication Layer

Modify the DERP connection flow to add authentication. This requires forking
or wrapping the Tailscale derper code.

#### Option A: Pre-Shared Key in HTTP Headers

Add a secret token that clients must provide:

```go
// Modified DERP server checks for Authorization header
func handleDERPUpgrade(w http.ResponseWriter, r *http.Request) {
    expectedToken := os.Getenv("DERP_AUTH_TOKEN")
    actualToken := r.Header.Get("Authorization")

    if actualToken != "Bearer "+expectedToken {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    // Continue with normal DERP upgrade...
}
```

Clients must send the token:
```go
client, err := derphttp.NewClient(privateKey, serverURL, log.Printf, netMon)
client.SetHTTPHeader("Authorization", "Bearer secret-token-here")
```

#### Option B: Public Key Allowlist

Only allow specific client public keys:

```go
// allowlist.txt contains one public key per line
var allowedKeys = loadAllowlist("/etc/derper/allowlist.txt")

func handleClientConnection(clientKey key.NodePublic) error {
    if !allowedKeys.Contains(clientKey) {
        return fmt.Errorf("client %s not in allowlist", clientKey)
    }
    // Continue...
}
```

#### Option C: Challenge-Response

Implement a challenge-response protocol:
1. Client connects with public key
2. Server sends challenge (random bytes)
3. Client proves ownership of secret (signs challenge)
4. Server verifies signature against known secrets

**Pros**:
- Flexible, can implement any auth scheme
- Can combine with rate limiting
- Application-aware access control

**Cons**:
- Requires forking/modifying Tailscale code
- Maintenance burden (keeping up with upstream)
- More complex than other options

**Best for**: Developers comfortable modifying Go code

---

### 5. Rate Limiting (Defense in Depth)

Even with access control, add rate limiting to prevent abuse:

#### Using nginx (if using reverse proxy)

```nginx
http {
    # Limit requests per IP
    limit_req_zone $binary_remote_addr zone=derp:10m rate=10r/s;

    server {
        location /derp {
            limit_req zone=derp burst=20;
            # ... proxy config ...
        }
    }
}
```

#### Using iptables

```bash
# Limit new connections per IP
sudo iptables -A INPUT -p tcp --dport 443 -m state --state NEW \
  -m recent --set --name DERP
sudo iptables -A INPUT -p tcp --dport 443 -m state --state NEW \
  -m recent --update --seconds 60 --hitcount 20 --name DERP \
  -j DROP
```

#### Application-Level (requires code modification)

Track bandwidth/connections per client public key and enforce limits.

---

## Recommendations

### For Your Spanza Project

Since you're building a WireGuard relay, here's what I recommend:

#### Phase 1: Development
- **No auth** - Keep it simple while testing
- Run on non-standard ports or private networks

#### Phase 2: Limited Deployment
- **Firewall-based** (Option 1) - Simple and effective
- Add rate limiting
- Monitor logs for suspicious activity

#### Phase 3: Production
- **Client certificates** (Option 3) if you can modify clients
- **OR Pre-shared key** (Option 4A) for simpler implementation
- Combine with firewall rules for defense in depth
- Add monitoring and alerting

### For Learning DERP Specifically

- Start **without authentication** to understand the protocol
- Add **firewall rules** when running on public servers
- Consider the **authentication layer** as a separate learning exercise

---

## Monitoring and Detection

Even with access control, monitor for abuse:

### Log Analysis

```bash
# Watch DERP connections (if logging enabled)
sudo journalctl -u derper -f | grep -i "new client"

# Watch for high bandwidth usage
sudo iftop -i eth0 -f "port 443"

# Count unique client IPs
sudo journalctl -u derper --since "1 hour ago" | \
  grep -oP '\d+\.\d+\.\d+\.\d+' | sort -u | wc -l
```

### Alerts

Set up alerts for:
- Unusual number of connections
- High bandwidth usage
- Connections from unexpected countries/IPs
- Connection attempts from blocked IPs

---

## Comparison Matrix

| Method | Security | Complexity | Client Changes | Scalability |
|--------|----------|------------|----------------|-------------|
| Firewall | Medium | Low | None | Low |
| VPN | High | Medium | None (if VPN exists) | Medium |
| Client Certs | High | High | Required | High |
| Pre-shared Key | Medium | Medium | Required | High |
| Public Key Allowlist | Medium | Medium | Required | Medium |

---

## Summary

For a production DERP server without Tailscale's control plane:

1. **Start with firewall rules** (quick win)
2. **Add rate limiting** (defense in depth)
3. **Consider client certs or pre-shared key** (strong auth)
4. **Monitor usage** (detect abuse)
5. **Document who has access** (operational clarity)

The "right" answer depends on:
- How many clients do you have?
- Do clients have static IPs?
- Can you modify client code?
- What's your threat model?

For learning purposes with the DERP playground, firewall rules are sufficient.
For a real relay service, you'll want stronger authentication.
