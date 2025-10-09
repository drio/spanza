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

## Quick Start

```bash
cd container

# Build and start both peers (auto-starts gateway and pings)
make build
make start

# You should see ping output showing connectivity between peers
```

## Firewall Testing

Test DERP relay under restrictive firewall conditions (all UDP blocked):

```bash
# Inside a running container:
./firewall-test.sh enable   # Block all UDP, force DERP over HTTPS
./firewall-test.sh status   # Check firewall status
./firewall-test.sh disable  # Remove firewall rules

# With firewall enabled, WireGuard traffic must relay through DERP over HTTPS (port 443)
# This simulates corporate firewalls that block UDP/peer-to-peer connections
```

## Available Tools

- WireGuard: `wg show`, `wg-quick`
- Network: `ping`, `tcpdump`, `netcat`, `iperf3`
- Firewall: `ufw`, `iptables`, `./firewall-test.sh`
- Debug: `vim`, `nano`, `curl`, `wget`

## Environment Variables

- `DERP_URL`: DERP server URL (default: https://derp1.tailscale.com)
