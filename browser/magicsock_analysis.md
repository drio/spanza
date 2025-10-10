# How Tailscale's MagicSock Handles WASM/Browser

## Summary

Tailscale's `magicsock` package **already handles WASM/browser environments** by conditionally disabling UDP and using DERP-only mode. We can learn from their approach, but we don't need to import their entire MagicSock - we can implement a simpler, custom `conn.Bind` for our use case.

## Key Findings from MagicSock

### 1. WASM-Specific Checks Throughout

MagicSock has numerous `runtime.GOOS == "js"` checks that disable UDP functionality:

```go
// Line 1486-1488: UDP sending is disabled in WASM
func (c *Conn) sendUDP(ipp netip.AddrPort, b []byte, isDisco bool, isGeneveEncap bool) (sent bool, err error) {
    if runtime.GOOS == "js" {
        return false, errNoUDP
    }
    // ... normal UDP code ...
}

// Line 1230-1240: Fake endpoint for WASM
if runtime.GOOS == "js" {
    // Return fake endpoint - control plane requires *something*
    return []tailcfg.Endpoint{
        {
            Addr: netip.MustParseAddrPort("[fe80:123:456:789::1]:12345"),
            Type: tailcfg.EndpointLocal,
        },
    }, nil
}

// Line 1408-1410: Fake port number for WASM
func (c *Conn) LocalPort() uint16 {
    if runtime.GOOS == "js" {
        return 12345
    }
    laddr := c.pconn4.LocalAddr()
    return uint16(laddr.Port)
}
```

### 2. DERP-Only Receive Mode

**This is the key insight!** In WASM, MagicSock only registers the DERP receiver:

```go
// Line 3228-3231: Only use DERP receiver in WASM
func (c *connBind) Open(ignoredPort uint16) ([]conn.ReceiveFunc, uint16, error) {
    // ...
    fns := []conn.ReceiveFunc{c.receiveIPv4(), c.receiveIPv6(), c.receiveDERP}
    if runtime.GOOS == "js" {
        fns = []conn.ReceiveFunc{c.receiveDERP}  // ← DERP ONLY!
    }
    return fns, c.LocalPort(), nil
}
```

### 3. DERP Receive Function

The `receiveDERP` function (line 666-686) reads from a channel:

```go
func (c *connBind) receiveDERP(buffs [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
    for dm := range c.derpRecvCh {  // ← Read from channel!
        if c.isClosed() {
            break
        }
        n, ep := c.processDERPReadResult(dm, buffs[0])
        if n == 0 {
            continue
        }
        sizes[0] = n
        eps[0] = ep
        return 1, nil
    }
    return 0, net.ErrClosed
}
```

The `derpRecvCh` is populated by `runDerpReader` goroutine (line 488+).

### 4. DERP Send Path

Sending via DERP uses a special "magic IP" to distinguish DERP from UDP:

```go
// Line 1591-1631: Send either via UDP or DERP
func (c *Conn) sendAddr(addr netip.AddrPort, pubKey key.NodePublic, b []byte, isDisco bool, isGeneveEncap bool) (sent bool, err error) {
    if addr.Addr() != tailcfg.DerpMagicIPAddr {
        return c.sendUDP(addr, b, isDisco, isGeneveEncap)  // Normal UDP
    }

    // DERP path:
    regionID := int(addr.Port())
    ch := c.derpWriteChanForRegion(regionID, pubKey)
    if ch == nil {
        return false, nil
    }

    pkt := bytes.Clone(b)  // Clone the packet
    wr := derpWriteRequest{addr, pubKey, pkt, isDisco}

    // Send to DERP write channel (non-blocking with retries)
    ch <- wr
    return true, nil
}
```

## Architecture

```
┌──────────────────────────────────────┐
│       WireGuard Device               │
│  (golang.zx2c4.com/wireguard/device) │
└──────────────┬───────────────────────┘
               │ conn.Bind interface
               │
               ▼
┌──────────────────────────────────────┐
│         MagicSock (or custom)        │
│                                      │
│  ┌────────────┐    ┌──────────────┐ │
│  │ Send()     │    │ ReceiveFunc  │ │
│  │  UDP/DERP  │    │  UDP/DERP    │ │
│  └─────┬──────┘    └──────▲───────┘ │
└────────┼──────────────────┼─────────┘
         │                  │
    In WASM/Browser:        │
         │                  │
         ▼                  │
┌─────────────────┐         │
│  derpWriteChan  │         │
└────────┬────────┘         │
         │                  │
         ▼                  │
┌──────────────────────────────────────┐
│         DERP Client                  │
│      (derphttp.Client)               │
│                                      │
│  runDerpWriter  →  dc.Send()         │
│  runDerpReader  →  dc.Recv()         │
└──────────────┬──────────▲────────────┘
               │          │
               ▼          │
         WebSocket connection
```

## Can We Reuse MagicSock?

**Decision: No, we should implement our own simpler version.**

**Why not reuse MagicSock:**
1. **Complexity**: MagicSock is ~4000 lines handling many scenarios we don't need:
   - NAT traversal
   - STUN
   - Multiple DERP regions
   - Path discovery
   - Peer management
   - UDP relay

2. **Dependencies**: Would pull in a lot of Tailscale internals:
   - Control plane integration
   - Health monitoring
   - Metrics
   - Network monitor

3. **Overkill**: We have a simpler use case:
   - Single DERP server
   - Browser → DERP → Server (no NAT traversal)
   - No need for UDP fallback

**What we should do instead:**
Implement a minimal `derpBind` that:
- Implements `conn.Bind` interface
- Sends: Writes to DERP client directly
- Receives: Reads from DERP client in a goroutine
- Much simpler (~200 lines vs 4000 lines)

## Implementation Plan

### Step 1: Create `derpBind` type

```go
type derpBind struct {
    derpClient   *derphttp.Client
    remotePubKey key.NodePublic
    recvCh       chan []byte
    closeCh      chan struct{}
    closed       bool
    mu           sync.Mutex
}
```

### Step 2: Implement `conn.Bind` interface

Required methods:
- `Open(port uint16) ([]conn.ReceiveFunc, uint16, error)`
- `Close() error`
- `Send(buffs [][]byte, endpoint conn.Endpoint) error`
- `SetMark(mark uint32) error` (no-op)
- `ParseEndpoint(s string) (conn.Endpoint, error)`

### Step 3: Implement receive goroutine

```go
func (b *derpBind) receiveLoop() {
    for {
        msg, err := b.derpClient.Recv()
        if err != nil {
            return
        }

        switch m := msg.(type) {
        case derp.ReceivedPacket:
            b.recvCh <- m.Data
        }
    }
}
```

### Step 4: Replace in `createWireGuard()`

```go
// OLD:
wgDevice = device.NewDevice(
    tun,
    conn.NewDefaultBind(),  // ❌ Tries to create UDP sockets
    device.NewLogger(...),
)

// NEW:
derpBind := newDerpBind(derpClient, remotePubKey)
wgDevice = device.NewDevice(
    tun,
    derpBind,  // ✅ Uses DERP instead of UDP
    device.NewLogger(...),
)
```

## Next Steps

1. Implement `derpBind` type with `conn.Bind` interface
2. Add receive goroutine that reads from DERP and feeds WireGuard
3. Modify `createWireGuard()` and `connectDERP()` flow
4. Test packet flow end-to-end
