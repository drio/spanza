# Container Testing for Spanza

Test Spanza UDP-to-DERP gateway with two WireGuard peers in containers.

## Architecture

```
peer1 container:                        peer2 container:
  WireGuard (192.168.4.1)                 WireGuard (192.168.4.2)
         ↓                                         ↓
  Spanza Gateway                          Spanza Gateway
         ↓                                         ↓
         └──────→ DERP Server (Tailscale) ←───────┘
```

## Usage

```bash
cd container

# Build image
make build

# Terminal 1: Start peer1
make peer1
# Inside container, note the DERP public key shown

# Terminal 2: Start peer2
make peer2
# Inside container, note the DERP public key shown

# In peer1, start Spanza gateway with peer2's DERP key:
./spanza --key-file /tmp/peer1-derp.key \
         --derp-url https://derp1.tailscale.com \
         --remote-peer nodekey:PEER2_PUBKEY \
         --listen :51821 \
         --wg-endpoint 127.0.0.1:51820 \
         --verbose

# In peer2, start Spanza gateway with peer1's DERP key:
./spanza --key-file /tmp/peer2-derp.key \
         --derp-url https://derp1.tailscale.com \
         --remote-peer nodekey:PEER1_PUBKEY \
         --listen :51821 \
         --wg-endpoint 127.0.0.1:51820 \
         --verbose

# Test connectivity:
ping 192.168.4.2  # from peer1
ping 192.168.4.1  # from peer2
```

## Available Tools

- WireGuard: `wg show`, `wg-quick`
- Network: `ping`, `tcpdump`, `netcat`, `iperf3`
- Debug: `vim`, `nano`, `curl`, `wget`

## Environment Variables

- `DERP_URL`: DERP server URL (default: https://derp1.tailscale.com)
