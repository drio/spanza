# Browser WireGuard + DERP Implementation TODO

## Architecture Overview

```
Browser (WASM)                          Server (Native Go)
─────────────────                       ──────────────────

WireGuard device                        WireGuard device
    ↓                                       ↓
DERP client ──────────────────────→     UDP socket
(WebSocket)      DERP Server         ←──────┘
                      ↑                     ↓
                      └─────────────── Spanza Gateway
                                            ↓
                                        DERP client
```

**Key difference:**
- Browser peer: WireGuard → DERP directly (WebSocket, no UDP)
- Server peer: WireGuard → UDP → Spanza Gateway → DERP (like userspace/)

## Progress

### ✅ Phase 0: Foundation (COMPLETED)
- [x] Create basic WASM infrastructure
- [x] Test Go ↔ JavaScript communication
- [x] Document wasm_exec.js runtime bridge
- [x] Commit: "Add basic WASM infrastructure for browser-based WireGuard"

### ✅ Phase 1: Server Peer (COMPLETED)
Create the server peer that browser will connect to.

- [x] Create browser/server/ directory structure
- [x] Create server/main.go with:
  - [x] Userspace WireGuard device (192.168.4.1)
  - [x] Spanza gateway (UDP → DERP)
  - [x] Simple HTTP server (for testing connectivity)
- [x] Create server/Makefile (build, run, clean targets)
- [x] Generate/document WireGuard keys for both peers
- [x] Test server peer builds successfully
- [x] Commit: "Add WireGuard server peer for browser testing"

### ✅ Phase 2: WASM WireGuard Device (COMPLETED)
Add userspace WireGuard to WASM module.

- [x] Add WireGuard imports to browser/wasm/main.go
- [x] Create userspace WireGuard device with netstack
- [x] Expose createWireGuard() function to JavaScript
- [x] Test compilation for WASM target
- [x] Verify device creation works (no networking yet)

### ✅ Phase 3: WASM DERP Client (COMPLETED)
Add DERP client to WASM module.

- [x] Add DERP client code to browser/wasm/main.go
- [x] Connect to https://derp.tailscale.com/derp
- [x] Verify WebSocket connection is used automatically
- [x] Test DERP connection from browser console
- [x] Commit: "Add DERP client to WASM module"

### ✅ Phase 4: Connect WireGuard to DERP (COMPLETED)
Wire up WireGuard device to DERP client in WASM.

- [x] Analyze Tailscale's MagicSock WASM handling
- [x] Create custom derpBind implementing conn.Bind interface
- [x] Wire derpBind to WireGuard device
- [x] Route WireGuard packets directly to DERP client
- [x] Handle received DERP packets → WireGuard device
- [x] Configure WireGuard peer (server's public key)
- [x] Update HTML with UI buttons for testing
- [x] Commit: "Implement derpBind and wire WireGuard to DERP"

### Phase 5: HTTP Through Tunnel
Prove end-to-end connectivity.

- [ ] Use netstack dialer for HTTP requests
- [ ] Expose fetch(url) function to JavaScript
- [ ] Update HTML with UI for making requests
- [ ] Test: Browser → WireGuard → DERP → Server → HTTP response
- [ ] Display response in browser
- [ ] 🎉 Success!

### Phase 6: Polish & Documentation
- [ ] Add error handling and logging
- [ ] Create comprehensive README
- [ ] Add diagrams showing packet flow
- [ ] Document key management
- [ ] Clean up code and comments

### Phase 7: Refactoring
- [ ] Extract gateway logic into reusable package
  - Gateway is duplicated in main.go, userspace/ustest.go, browser/server/main.go
  - Create gateway/ package with clean API
  - Refactor all instances to use the package
- [ ] Review and consolidate other duplicated code

## Current Focus

**Phase 4: COMPLETED - derpBind Implementation**

We've successfully implemented a custom `conn.Bind` that routes WireGuard packets
through DERP instead of UDP. This is the critical piece that makes WireGuard work
in the browser where UDP sockets aren't available.

**Key implementation details:**
- `derpBind` implements `conn.Bind` interface
- Sends packets via `derpClient.Send()`
- Receives packets via goroutine reading from `derpClient.Recv()`
- Returns single receive function (DERP only, no UDP) when `runtime.GOOS == "js"`
- Learned from Tailscale's MagicSock but implemented simpler version (~200 lines vs 4000)

**Next: Phase 5 - HTTP Through Tunnel**

Test end-to-end connectivity by making HTTP requests through the tunnel.
