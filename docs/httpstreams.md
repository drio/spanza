# HTTP Upgrade vs WebSockets

## Decision

We use **HTTP Upgrade to a custom binary protocol** instead of WebSockets for tunneling WireGuard packets over HTTPS.

## Why Not WebSockets?

While WebSockets are widely supported and well-understood, they add unnecessary overhead for our use case:

### WebSocket Overhead

After the HTTP upgrade handshake, WebSockets require:
- **Frame headers**: 2-14 bytes per message
- **Masking**: Client messages must XOR all payload bytes with a random 4-byte key (RFC 6455 requirement)
- **Control frames**: Must handle ping/pong/close frames
- **Complexity**: Need WebSocket library to manage framing protocol

### Our Needs

We're relaying opaque WireGuard packets:
- No need for text frames, fragmentation, or control messages
- Each packet is independent and self-contained
- Don't need WebSocket features like subprotocols or extensions

## HTTP Upgrade Approach

Similar to Tailscale DERP, we use HTTP's upgrade mechanism without WebSocket framing:

### Client Handshake
```http
GET /relay HTTP/1.1
Host: server.example.com
Upgrade: spanza
Connection: Upgrade
```

### Server Response
```http
HTTP/1.1 101 Switching Protocols
Upgrade: spanza
Connection: Upgrade
```

### After Upgrade

The connection becomes a **raw bidirectional TCP stream** over TLS:
- Write WireGuard packets directly (no framing)
- Read WireGuard packets directly (no unmasking)
- Zero overhead per packet
- Simple implementation using `http.Hijacker`

## Comparison

| Feature | HTTP Upgrade | WebSockets |
|---------|--------------|------------|
| Handshake | HTTP 101 Switching Protocols | HTTP 101 Switching Protocols |
| After handshake | Raw TCP stream | WebSocket framing protocol |
| Overhead per packet | 0 bytes | 2-14 bytes + XOR masking |
| Client writes | Raw bytes | Must wrap in frames + mask |
| Server writes | Raw bytes | Must wrap in frames |
| Implementation | http.Hijack + io.ReadWriter | WebSocket library |
| Firewall traversal | ✅ HTTPS | ✅ HTTPS |

## Implementation

Server side:
```go
func handleUpgrade(w http.ResponseWriter, r *http.Request) {
    if r.Header.Get("Upgrade") != "spanza" {
        http.Error(w, "Bad Request", 400)
        return
    }

    hijacker, ok := w.(http.Hijacker)
    if !ok {
        http.Error(w, "Hijacking not supported", 500)
        return
    }

    w.WriteHeader(http.StatusSwitchingProtocols)
    conn, brw, err := hijacker.Hijack()
    // ... handle raw TCP stream
}
```

Client side:
```go
req, _ := http.NewRequest("GET", serverURL, nil)
req.Header.Set("Upgrade", "spanza")
req.Header.Set("Connection", "Upgrade")

// Write request, read response
// After 101 response, use connection as raw TCP stream
```

## Prior Art

- **Tailscale DERP**: Uses `Upgrade: DERP` to tunnel their relay protocol
- **HTTP/2**: Uses `Upgrade: h2c` for cleartext HTTP/2
- **SPDY**: Used HTTP Upgrade before being replaced by HTTP/2

## Benefits

1. **Simplicity**: No framing/masking logic needed
2. **Performance**: Zero per-packet overhead
3. **Compatibility**: Still tunnels over HTTPS through firewalls
4. **Proven**: Tailscale uses this at scale
5. **Flexibility**: Full control over the binary protocol
