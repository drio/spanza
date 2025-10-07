# Relay Package

Tracks WireGuard peers and determines packet forwarding destinations.

## Components

**Registry** - Thread-safe map from peer index to endpoint
- `Register(index, endpoint)` - Update peer location
- `Lookup(index)` - Find peer endpoint
- `Remove(index)` - Delete peer

**Processor** - Learns peer endpoints from packets
- `ProcessPacket(data, source)` - Parse packet, learn sender, return destination

**Endpoint** - Peer's network location (UDP address or WebSocket connection)

## How It Works

The relay learns peer locations by inspecting WireGuard packet indices:

**Initiation packet** - Contains sender index
- Learn: sender → source endpoint
- Forward: nowhere (first packet in handshake)

**Response packet** - Contains sender and receiver indices
- Learn: sender → source endpoint
- Forward: to receiver's endpoint

**Transport packet** - Contains receiver index
- Learn: receiver → source endpoint
- Forward: to receiver's endpoint

When a peer's location changes, the registry updates automatically on the next packet.
