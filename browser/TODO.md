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

### 🚧 Phase 2: WASM WireGuard Device (IN PROGRESS)
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

**Phase 2: WASM WireGuard Device**

Now that we have a working server peer, we'll add WireGuard to the WASM
module. This will create a userspace WireGuard device that runs in the browser.

Key challenge: Making sure WireGuard compiles for WASM target and works with
netstack in the browser environment.
