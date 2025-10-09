# DERP Playground

This playground tests sending data through Tailscale's DERP (Designated
Encrypted Relay for Packets) server to understand how it works before building
our own relay.

## What We're Testing

We want to verify that DERP can relay arbitrary packets between two
clients, which will help us understand if we can:

1. Use DERP as-is for our WireGuard relay
2. Adapt DERP's design patterns for our own implementation

## Architecture

```
[Client A] <---> [DERP Server] <---> [Client B]
```

- **DERP Server**: Runs locally, accepts connections from clients
- **Client A**: Connects to server, sends packets to Client B
- **Client B**: Connects to server, receives packets from Client A, can respond

## Components

### 1. Server (`server/main.go`)
- Runs a DERP server on localhost
- Binds to a port (e.g., 3340 for HTTP, could use HTTPS)
- Tracks connected clients by their public keys
- Relays packets between clients

### 2. Client A (`clientA/main.go`)
- Generates or loads a private key
- Connects to DERP server
- Sends test packets to Client B's public key
- Receives and displays responses

### 3. Client B (`clientB/main.go`)
- Generates or loads a different private key
- Connects to DERP server
- Receives packets from Client A
- Optionally sends responses back to Client A

## Tasks

- [ ] **Task 1**: Create DERP server program
  - Set up basic HTTP server
  - Initialize DERP server with key
  - Handle client connections
  - Add logging to see what's happening

- [ ] **Task 2**: Create Client A program
  - Generate/load private key
  - Connect to DERP server
  - Send test messages (simple strings initially)
  - Display received messages

- [ ] **Task 3**: Create Client B program
  - Generate/load private key (different from A)
  - Connect to DERP server
  - Receive and display messages
  - Send responses back

- [ ] **Task 4**: Test communication
  - Start server
  - Start Client B (receiver)
  - Start Client A (sender)
  - Verify packets flow: A → Server → B
  - Verify responses: B → Server → A

- [ ] **Task 5**: Test with binary data
  - Send arbitrary byte sequences
  - Verify data integrity
  - Test with WireGuard-sized packets (~1500 bytes)

## Key Concepts to Understand

1. **Key-based addressing**: Clients are identified by their curve25519
   public keys
2. **Frame protocol**: DERP uses a simple frame format
   (type + length + payload)
3. **Connection management**: Server maintains mappings of keys to
   connections
4. **Transport agnostic**: DERP doesn't care about packet contents

## Running the Tests

```bash
# Terminal 1: Start server
cd playground/derp/server
go run main.go

# Terminal 2: Start Client B (receiver)
cd playground/derp/clientB
go run main.go

# Terminal 3: Start Client A (sender)
cd playground/derp/clientA
go run main.go
```

## What We'll Learn

- How DERP handles client registration
- How packets are routed by public key
- Whether DERP can handle any packet type (including WireGuard)
- Performance characteristics
- How reconnection works
- How the frame protocol works in practice

## Next Steps

After validating DERP works for our use case, we'll decide:
1. Use DERP libraries directly in our relay
2. Build a simplified DERP-like relay
3. Extend DERP with WireGuard-specific optimizations
