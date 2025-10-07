# Packet Package

This package provides parsing functionality for WireGuard protocol messages.

## WireGuard Message Types

WireGuard uses 4 types of messages:

1. **Handshake Initiation (type 1)** - First message of the handshake
   - Contains: sender index
   - Size: 148 bytes

2. **Handshake Response (type 2)** - Second message of the handshake
   - Contains: sender index, receiver index
   - Size: 92 bytes

3. **Cookie Reply (type 3)** - DoS mitigation message
   - Contains: receiver index
   - Size: 64 bytes

4. **Transport Data (type 4)** - Encrypted data packets
   - Contains: receiver index
   - Size: minimum 32 bytes (16 byte header + 16 byte auth tag)

## Usage

```go
package main

import (
	"fmt"
	"github.com/drio/spanza/packet"
)

func main() {
	// Parse a WireGuard packet
	data := []byte{ /* raw packet data */ }
	msg, err := packet.Parse(data)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Message type: %s\n", msg.Type)

	if msg.Sender != nil {
		fmt.Printf("Sender index: 0x%x\n", *msg.Sender)
	}

	if msg.Receiver != nil {
		fmt.Printf("Receiver index: 0x%x\n", *msg.Receiver)
	}
}
```

## Peer Index Tracking

For relaying WireGuard packets, the key information is:

- **Handshake Initiation**: Extract sender index to know who initiated
- **Handshake Response**: Extract both indices to map sender → receiver
- **Transport Data**: Extract receiver index to know where to forward

This allows the relay to maintain a mapping of peer indices to endpoints without
needing to decrypt or understand the cryptographic content of the packets.
