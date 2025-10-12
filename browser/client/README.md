# Native Client Test Harness

## Purpose

This is a **debugging tool** to validate the WireGuard + DERP + Spanza gateway architecture outside of the browser/WASM environment.

## Why This Exists

The browser WASM client (`browser/wasm/`) uses a custom `derpBind` implementation because browsers don't support UDP sockets. This client was created to answer the question:

**"Is the issue with our architecture, or specifically with the WASM `derpBind` implementation?"**

## What It Does

This native Go client:
- Uses the **exact same keys and IP** as the browser peer (192.168.4.2)
- Connects to the same server peer (192.168.4.1)
- Uses the **standard architecture**: WireGuard → DefaultBind (UDP) → Spanza Gateway → DERP → Server
- Makes HTTP requests through the WireGuard tunnel to test connectivity

## Results

✅ **The native client works perfectly**, proving:
- The overall architecture is correct
- The keys are properly configured
- The server is functioning correctly
- The Spanza gateway UDP ↔ DERP proxy works as designed

❌ **The browser WASM client fails** with HTTP timeouts, proving:
- The issue is isolated to the `derpBind` implementation in `browser/wasm/main.go`
- The problem is WASM/browser-specific, not architectural

## Usage

```bash
# Make sure the server is running first
cd ../server
make run

# In another terminal, run the client
cd ../client
make run
```

You should see:
```
✅ SUCCESS! HTTP response received:
Status: 200 OK
Body: Hello from WireGuard server!
```

## Comparison

| Component | Browser (WASM) | Native Client |
|-----------|---------------|---------------|
| IP Address | 192.168.4.2 | 192.168.4.2 |
| Keys | Same | Same |
| WireGuard Bind | `derpBind` (custom) | `DefaultBind` (standard) |
| Transport | DERP direct | UDP → Gateway → DERP |
| HTTP Requests | ❌ Timeout | ✅ Works |

## Next Steps

Debug the `derpBind` implementation in `browser/wasm/main.go` to understand why:
1. WireGuard handshake completes successfully
2. Keepalive packets work
3. But HTTP data packets timeout

See `claude/context.md` for detailed investigation notes.
