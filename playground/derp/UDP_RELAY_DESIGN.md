# UDP-to-DERP Gateway Design

This document describes how to build a relay that tunnels WireGuard UDP traffic
through DERP servers.

## The Problem

Standard WireGuard:
- Sends UDP packets to `peer_ip:51820`
- Cannot traverse restrictive firewalls (UDP blocked, only HTTPS allowed)
- No relay/bounce capability

You want:
- WireGuard peer → UDP → **Relay** → DERP → **Relay** → UDP → WireGuard peer
- Works through firewalls that only allow HTTPS (port 443)

## Architecture

```
┌─────────────────┐                ┌─────────────────┐
│   WireGuard A   │                │   WireGuard B   │
│  (192.168.1.5)  │                │  (10.0.0.5)     │
└────────┬────────┘                └────────▲────────┘
         │ UDP                              │ UDP
         │ :51820                           │ :51820
         ▼                                  │
┌─────────────────┐                ┌─────────────────┐
│   UDP Gateway   │                │   UDP Gateway   │
│   (Client A)    │                │   (Client B)    │
│                 │                │                 │
│ UDP: 0.0.0.0    │                │ UDP: 0.0.0.0    │
│   :51821        │                │   :51821        │
└────────┬────────┘                └────────▲────────┘
         │ DERP                             │ DERP
         │ (HTTPS)                          │ (HTTPS)
         └──────────────┬───────────────────┘
                        │
                        ▼
              ┌──────────────────┐
              │   DERP Server    │
              │ derp.example.com │
              │      :443        │
              └──────────────────┘
```

## Components Needed

### 1. UDP Listener
Listens for WireGuard packets on UDP port 51821 (or configurable).

### 2. DERP Client
Connects to DERP server, registers with public key.

### 3. Packet Router
Maps UDP packets ↔ DERP messages:
- Incoming UDP → Send via DERP to peer
- Incoming DERP → Send via UDP to local WireGuard

### 4. Peer Discovery
Know which DERP peer corresponds to which WireGuard endpoint.

## Implementation Sketch

### Gateway Component

```go
package main

import (
    "net"
    "log"
    "github.com/drio/spanza/playground/derp/client"
    "tailscale.com/types/key"
)

type UDPGateway struct {
    // DERP connection
    derpClient *client.DERPClient
    privateKey key.NodePrivate

    // UDP socket for WireGuard
    udpConn *net.UDPConn

    // Peer mapping: DERP public key → UDP address
    peerMap map[key.NodePublic]*net.UDPAddr

    // Reverse mapping: UDP address → DERP public key
    addrMap map[string]key.NodePublic
}

func (gw *UDPGateway) Start() error {
    // 1. Start UDP listener
    go gw.listenUDP()

    // 2. Connect to DERP
    go gw.listenDERP()

    return nil
}

// Receive UDP from WireGuard, send to DERP
func (gw *UDPGateway) listenUDP() {
    buf := make([]byte, 65535)
    for {
        n, addr, err := gw.udpConn.ReadFromUDP(buf)
        if err != nil {
            log.Printf("UDP read error: %v", err)
            continue
        }

        // Look up which DERP peer this UDP address maps to
        peerKey, ok := gw.addrMap[addr.String()]
        if !ok {
            log.Printf("Unknown UDP source: %s", addr)
            continue
        }

        // Send packet to DERP peer
        if err := gw.derpClient.Send(peerKey, buf[:n]); err != nil {
            log.Printf("DERP send error: %v", err)
        }
    }
}

// Receive from DERP, send to UDP (WireGuard)
func (gw *UDPGateway) listenDERP() {
    for {
        msg, err := gw.derpClient.Recv()
        if err != nil {
            log.Printf("DERP recv error: %v", err)
            continue
        }

        // Extract packet from DERP message
        pkt, ok := msg.(derp.ReceivedPacket)
        if !ok {
            continue
        }

        // Look up UDP address for this DERP peer
        udpAddr, ok := gw.peerMap[pkt.Source]
        if !ok {
            log.Printf("Unknown DERP source: %s", pkt.Source)
            continue
        }

        // Send to local WireGuard via UDP
        if _, err := gw.udpConn.WriteToUDP(pkt.Data, udpAddr); err != nil {
            log.Printf("UDP write error: %v", err)
        }
    }
}
```

## The Peer Discovery Problem

**Critical issue**: How does the gateway know:
- Which DERP peer to send to?
- Which UDP address maps to which DERP peer?

### Solution 1: Static Configuration

Pre-configure the mapping:

```yaml
# gateway-config.yaml
peers:
  - derp_pubkey: "nodekey:abc123..."
    udp_address: "192.168.1.5:51820"
    wireguard_pubkey: "wg_pubkey_here"

  - derp_pubkey: "nodekey:def456..."
    udp_address: "10.0.0.5:51820"
    wireguard_pubkey: "wg_pubkey_here"
```

Each gateway loads this config and knows how to route.

### Solution 2: Dynamic Discovery (Complex)

Build a coordination server (like Tailscale's control plane):
1. Gateways register: "I'm serving WireGuard peer X"
2. Coordination server builds routing table
3. Gateways query: "Where is WireGuard peer Y?"
4. Server responds: "Via DERP peer Z"

This is what Tailscale does, but it's complex.

### Solution 3: Protocol Extension

Embed metadata in the first packet:
1. WireGuard sends UDP to gateway
2. Gateway wraps packet with header: `[DERP_PEER_KEY][WG_PACKET]`
3. Remote gateway unwraps and learns mapping

Requires modifying WireGuard or intercepting/wrapping packets.

## WireGuard Configuration

With the gateway, WireGuard peers would be configured like:

```ini
# Peer A's WireGuard config
[Interface]
PrivateKey = <wireguard_private_key>
Address = 10.0.0.1/24
ListenPort = 51820

[Peer]
PublicKey = <peer_b_wireguard_pubkey>
# Point to local gateway instead of remote peer directly!
Endpoint = 127.0.0.1:51821
AllowedIPs = 10.0.0.2/32
PersistentKeepalive = 25
```

The gateway listens on `127.0.0.1:51821` and relays to DERP.

## Challenges

### 1. **NAT and Endpoint Confusion**

WireGuard learns endpoints from received packets. If gateway changes the source,
WireGuard gets confused.

**Solution**: Use static endpoints, disable roaming.

### 2. **MTU and Fragmentation**

DERP adds overhead. WireGuard packets + DERP framing might exceed MTU.

**Solution**:
- Lower WireGuard MTU (`MTU = 1280` in config)
- Or implement fragmentation in gateway

### 3. **Performance**

Double encryption + extra hop:
- WireGuard encryption
- DERP TLS
- Gateway processing

**Solution**: Accept the overhead (it's the price of relay).

### 4. **State Management**

Gateway must track:
- Active sessions
- Timeout idle connections
- Handle DERP reconnections

### 5. **Security**

Gateway has access to encrypted WireGuard packets but:
- Cannot decrypt (only WireGuard peers have keys)
- But knows traffic patterns, timing, packet sizes

**Mitigation**: Run your own gateway, don't trust third parties.

## Existing Tools That Do Similar Things

### 1. **wstunnel**
Tunnels TCP/UDP over WebSocket (which works over HTTPS).

```bash
# Server
wstunnel -s 0.0.0.0:443 -r 127.0.0.1:51820

# Client (makes UDP appear at 51821, relayed via HTTPS to server)
wstunnel -c wss://server:443 -L 127.0.0.1:51821:127.0.0.1:51820 -u
```

- https://github.com/erebe/wstunnel
- UDP over WebSocket over HTTPS
- Simple to use
- Doesn't use DERP though

### 2. **Chisel**
Fast TCP/UDP tunnel over HTTP.

```bash
# Server
chisel server --port 443 --reverse

# Client
chisel client https://server:443 51821:localhost:51820/udp
```

- https://github.com/jpillora/chisel
- Similar to wstunnel
- Not DERP-based

### 3. **frp** (Fast Reverse Proxy)
Comprehensive tunnel solution including UDP.

- https://github.com/fatedier/frp
- Supports UDP forwarding
- HTTPS transport

## Recommendation for Your Spanza Project

Instead of trying to tunnel standard WireGuard through DERP, consider:

### Option A: Use Existing UDP Tunneling

Use **wstunnel** or **chisel** for the UDP relay part:
- Simpler than building from scratch
- Production-ready
- Well-tested

Your focus: Build the control plane and coordination.

### Option B: Build DERP-Like Protocol from Scratch

Since you're learning, build your own relay protocol:
1. UDP listener ✓ (simpler than HTTP upgrade)
2. Custom framing protocol ✓ (you understand this now)
3. Peer discovery ✓ (coordination server)
4. NAT traversal ✓ (STUN, hole punching)

Skip DERP entirely and build something tailored for WireGuard relay.

### Option C: Build the UDP-DERP Gateway

Implement the gateway described in this document:
- Good learning experience
- Reuses DERP infrastructure
- Teaches protocol translation

This is a complete project in itself!

## Next Steps

If you want to build the UDP-DERP gateway:

1. **Start simple**: Two gateways, static peer config
2. **Test locally**: Both gateways on same machine, different ports
3. **Add DERP**: Connect gateways through local DERP server
4. **Test remote**: Deploy to different machines
5. **Add discovery**: Build coordination server
6. **Production**: Error handling, reconnection, metrics

Would you like me to help you build this gateway? It's a great next project
after understanding DERP!

## Summary

**Direct answer**: No, there's no existing tool that does "WireGuard UDP → DERP relay".

**Why**: DERP was designed for Tailscale's modified WireGuard, not standard WireGuard.

**Alternatives**:
1. Use wstunnel/chisel for UDP-over-HTTPS tunneling (not DERP)
2. Build your own UDP-DERP gateway (learning project)
3. Build a custom relay protocol (better fit for WireGuard)

The UDP-DERP gateway would be an excellent learning project that combines
everything you've learned about DERP, networking, and protocol design!
