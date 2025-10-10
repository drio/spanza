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

### 🚧 Phase 1: Server Peer (IN PROGRESS)
Create the server peer that browser will connect to.

- [ ] Create browser/server/ directory structure
- [ ] Create server/main.go with:
  - [ ] Userspace WireGuard device (192.168.4.1)
  - [ ] Spanza gateway (UDP → DERP)
  - [ ] Simple HTTP server (for testing connectivity)
- [ ] Create server/Makefile (build, run, clean targets)
- [ ] Generate/document WireGuard keys for both peers
- [ ] Test server peer runs independently
- [ ] Document server peer configuration

### Phase 2: WASM WireGuard Device
Add userspace WireGuard to WASM module.

- [ ] Add WireGuard imports to browser/wasm/main.go
- [ ] Create userspace WireGuard device with netstack
- [ ] Expose createWireGuard() function to JavaScript
- [ ] Test compilation for WASM target
- [ ] Verify device creation works (no networking yet)

### Phase 3: WASM DERP Client
Add DERP client to WASM module.

- [ ] Add DERP client code to browser/wasm/main.go
- [ ] Connect to https://derp.tailscale.com/derp
- [ ] Verify WebSocket connection is used automatically
- [ ] Expose getDERPStatus() to JavaScript
- [ ] Test DERP connection from browser console

### Phase 4: Connect WireGuard to DERP
Wire up WireGuard device to DERP client in WASM.

- [ ] Route WireGuard packets directly to DERP client
- [ ] Handle received DERP packets → WireGuard device
- [ ] Configure WireGuard peer (server's public key, endpoint)
- [ ] Test WireGuard handshake completes
- [ ] Verify encrypted tunnel established

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

**Step 1.1**: Create server peer directory structure and basic main.go

We're building the server peer first because:
1. Gives us a clear target to connect to
2. Can test independently before WASM complexity
3. Reuses proven patterns from userspace/ustest.go
