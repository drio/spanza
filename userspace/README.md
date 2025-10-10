# Userspace WireGuard + Spanza Testing

This directory contains a fully userspace implementation of WireGuard peers
communicating through the Spanza DERP relay. Everything runs in a single Go
process with no kernel involvement, no root privileges, and no containers
required.

## Thank you

To Jason [Jason Donenfeld](https://www.zx2c4.com/) for his contributions. Nothing
of what I did here is new, it is based on his amazing work:

From the good folks at [Fly.io](https://fly.io/blog/our-user-mode-wireguard-year/):

<pre>
"I said this within earshot of Jason Donenfeld. The next morning, he had a
working demo. As it turns out, the gVisor project had exactly the same
user-mode TCP/IP problem, and built a pretty excellent TCP stack in Golang.
Jason added bindings to wireguard-go, and an enterprising soul (Ben Burkert,
pay him all your moneys, he’s fantastic) offered to build the feature into
flyctl. We were off to the races: you could flyctl ssh console into any Fly.io
app, with no client configuration beyond just installing flyctl."
</pre>


## What's Different from Container Testing?

### Container Approach (`container/`)
- Requires Docker/Podman
- Uses kernel WireGuard (`wg` command)
- Needs privileged container capabilities (`NET_ADMIN`)
- Each peer runs in separate container
- Heavier resource usage
- Slower to start/stop

### Userspace Approach (`userspace/`)
- Pure Go, single process
- Uses userspace WireGuard implementation
- **No root privileges needed**
- **No containers needed**
- **No kernel modules needed**
- All peers in one binary
- Fast startup (< 5 seconds)
- Perfect for CI/CD and automated testing

## Architecture

```
Application Layer (HTTP client/server)
         ↓
tnet (userspace TCP/IP stack - gvisor netstack)
         ↓
WireGuard device (encryption/decryption)
         ↓
Spanza Gateway (UDP ↔ DERP relay)
         ↓
DERP Server (Tailscale public relay over HTTPS)
```

**Everything runs in userspace** - no kernel networking stack involved.

## How It Works

1. **Userspace Network Stack**: Uses gvisor's netstack to implement TCP/IP
   entirely in Go
2. **Userspace WireGuard**: golang.zx2c4.com/wireguard provides WireGuard
   protocol in Go
3. **Spanza Gateway**: Relays WireGuard's UDP packets through DERP over HTTPS
4. **Single Process**: Both peers, both gateways, all networking - one binary

## Quick Start

```bash
# Build
make build

# Run the test
make test
# or
./ustest
```

The test will:
1. Start two Spanza gateways (one per peer)
2. Create two userspace WireGuard interfaces
   (192.168.4.1 and 192.168.4.2)
3. Start HTTP server on peer1
4. Make HTTP request from peer2 to peer1
5. Show success message if traffic flows through DERP

## Why This Matters

This demonstrates that you can run WireGuard with DERP relay:
- Without any infrastructure (no VMs, no containers)
- Without any privileges (no root, no capabilities)
- In any environment (CI/CD, cloud functions, restricted environments)
- With full network stack simulation

Perfect for:
- **Testing**: Fast, reproducible integration tests
- **CI/CD**: No Docker daemon required
- **Development**: Quick iteration without container overhead
- **Restricted environments**: Where you can't modify kernel networking

## Files

- `ustest.go` - Combined test with both peers in one binary
- `Makefile` - Build and test targets
- `README.md` - This file

## Technical Details

**Ports:**
- Peer1 WireGuard: 51820 → Peer1 Gateway: 51821
- Peer2 WireGuard: 51822 → Peer2 Gateway: 51823

**Keys:**
- Each peer has separate WireGuard keys (for tunnel encryption)
- Each peer has separate DERP keys (for relay identity)
- Same keys as container setup for consistency

**Network:**
- Peer1: 192.168.4.1/24
- Peer2: 192.168.4.2/24
- All traffic routes through DERP (no direct UDP between peers)
