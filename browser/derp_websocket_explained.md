# How DERP Client Automatically Uses WebSocket in Browser

## What is WebSocket?

WebSocket is a **protocol for full-duplex communication over a single TCP
connection**. Let's understand where it sits in the network stack and how it
differs from regular TCP.

### Network Stack Comparison

```
Application Layer    Your Code (DERP protocol, HTTP, etc.)
                     ─────────────────────────────────────
Transport Layer      WebSocket Protocol  |  Raw TCP
                     ─────────────────────────────────────
Network Layer        TCP (port 443 typically)
                     ─────────────────────────────────────
                     IP
                     ─────────────────────────────────────
                     Ethernet / WiFi
```

**Key insight**: WebSocket **runs on top of TCP**. It's not a replacement for
TCP, it's a layer above it.

### How WebSocket Differs from Raw TCP

#### Raw TCP Connection:
```
Client                          Server
   |                               |
   |--- TCP SYN ------------------>|
   |<-- TCP SYN-ACK --------------|
   |--- TCP ACK ------------------>|
   |                               |
   | Now connected, can send bytes |
   |                               |
   |--- binary data -------------->|
   |<-- binary data ---------------|
   |                               |
   |--- TCP FIN ------------------>|
```

- **No structure**: Just a stream of bytes
- **No framing**: You must handle message boundaries yourself
- **No browser API**: JavaScript cannot create raw TCP sockets (security)

#### WebSocket Connection:
```
Client (Browser)                Server
   |                               |
   |--- HTTP GET ------------------>|  "Upgrade: websocket"
   |    (Upgrade request)           |  "Connection: Upgrade"
   |<-- HTTP 101 ------------------|  "Switching Protocols"
   |                               |
   | Now WebSocket, still over TCP |
   |                               |
   |--- WebSocket frame ---------->|  Message 1
   |<-- WebSocket frame -----------|  Message 2
   |--- WebSocket frame ---------->|  Message 3
   |                               |
   |--- WebSocket close frame ---->|
```

- **Starts as HTTP**: Initial handshake uses HTTP Upgrade
- **Framed messages**: Each message has a header with length, type
- **Browser API**: JavaScript can create WebSocket connections
- **Still TCP underneath**: Uses the same TCP connection

### The HTTP Upgrade Handshake

WebSocket starts as an HTTP request:

```http
GET /derp HTTP/1.1
Host: derp.tailscale.com
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Protocol: derp
Sec-WebSocket-Version: 13
```

If the server supports WebSocket, it responds:

```http
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
Sec-WebSocket-Protocol: derp
```

After this "101 Switching Protocols" response, **the same TCP connection** is
now used for WebSocket frames instead of HTTP.

### WebSocket Frame Structure

Unlike raw TCP (just bytes), WebSocket wraps data in frames:

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-------+-+-------------+-------------------------------+
|F|R|R|R| opcode|M| Payload len |    Extended payload length    |
|I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
|N|V|V|V|       |S|             |   (if payload len==126/127)   |
| |1|2|3|       |K|             |                               |
+-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
|     Extended payload length continued, if payload len == 127  |
+ - - - - - - - - - - - - - - - +-------------------------------+
|                               |Masking-key, if MASK set to 1  |
+-------------------------------+-------------------------------+
| Masking-key (continued)       |          Payload Data         |
+-------------------------------- - - - - - - - - - - - - - - - +
:                     Payload Data continued ...                :
+ - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
|                     Payload Data continued ...                |
+---------------------------------------------------------------+
```

**What this gives us**:
- **Message boundaries**: Know where one message ends and next begins
- **Message types**: Text, binary, ping, pong, close
- **Automatic handling**: Browser and libraries handle this complexity

### Why Browsers Require WebSocket (Not Raw TCP)

**Security**: Browsers cannot allow JavaScript to create raw TCP connections
because:

1. **Port scanning**: Malicious sites could scan your internal network
2. **Protocol confusion**: Could speak non-HTTP protocols to servers expecting
   HTTP
3. **Firewall bypass**: Could tunnel arbitrary protocols through port 80/443

**WebSocket is safe because it adds an abstraction layer**:

```
Raw TCP (not allowed):
JavaScript → TCP socket (direct control) ❌

WebSocket (allowed):
JavaScript → WebSocket API → Browser → TCP socket ✓
            (abstraction layer)
```

**The key difference**: The **browser controls the TCP socket**, not your
JavaScript code. Your app only talks to the WebSocket API, which is a
higher-level abstraction. The browser acts as a gatekeeper:

- Still goes through HTTP first (server must opt-in with Upgrade response)
- Has origin headers (server can validate who's connecting)
- Starts on HTTP ports (443/80), servers expect it
- Browser enforces same-origin policy
- Browser validates all WebSocket frames
- Browser controls when the socket opens/closes

**What your JavaScript sees**: A clean API with `send()` and `onmessage()`

**What the browser does behind the scenes**: Manages the actual TCP socket,
enforces protocol rules, wraps/unwraps WebSocket frames, validates everything

### WebSocket vs TCP: Summary Table

| Feature              | Raw TCP                  | WebSocket              |
|----------------------|--------------------------|------------------------|
| **Protocol Layer**   | Transport (Layer 4)      | Application (Layer 7)  |
| **Runs on top of**   | IP                       | TCP                    |
| **Data format**      | Byte stream              | Framed messages        |
| **Handshake**        | TCP 3-way handshake      | HTTP Upgrade           |
| **Port**             | Any port                 | Usually 80 or 443      |
| **Browser API**      | ❌ No (security)         | ✅ Yes                 |
| **Message boundary** | ❌ You must handle       | ✅ Built-in            |
| **Bidirectional**    | ✅ Yes                   | ✅ Yes                 |
| **Used for**         | Most network protocols   | Browser real-time apps |

### For Our DERP Use Case

**Why this works perfectly**:

1. **Browser restriction**: WASM in browser can't do raw TCP, but **can** do
   WebSocket
2. **Same TCP connection**: WebSocket still uses TCP underneath, just adds
   framing
3. **DERP protocol unchanged**: DERP protocol messages become WebSocket frames
4. **Server supports both**: DERP server accepts both raw TCP and WebSocket

```
WASM in Browser              DERP Server
───────────────              ───────────

DERP protocol message        DERP protocol message
        ↓                            ↑
WebSocket frame              WebSocket frame
        ↓                            ↑
     TCP connection (port 443)
```

The DERP protocol messages are identical. Only difference: in browser they're
wrapped in WebSocket frames.

## The Problem

When Go code compiles to WebAssembly and runs in a browser:
- **Cannot create raw TCP sockets** - Browser security sandbox prevents this
- **Cannot create UDP sockets** - Same security restrictions
- **CAN use WebSocket** - Standard browser API, designed for this purpose

But our WireGuard code expects to send packets over a network connection. How
does this work?

## The Solution: Automatic WebSocket Detection

Tailscale's DERP client library **automatically detects when it's running in a
browser** and switches from TCP to WebSocket. The brilliant part: **the rest of
your code doesn't need to change at all.**

## How It Works

### 1. Build Tags Detect the Platform

**File**: `derp/derphttp/websocket.go`

```go
//go:build js || ((linux || darwin) && ts_debug_websockets)

package derphttp

// This file only compiles when:
// - Target is js (WebAssembly for browser), OR
// - Debug mode for testing WebSocket on native platforms

const canWebsockets = true

func init() {
    dialWebsocketFunc = dialWebsocket  // Set the WebSocket dialer
}
```

**Key insight**: When you compile with `GOOS=js GOARCH=wasm`, Go includes this
file and sets up WebSocket support. On native platforms (Linux, Mac, Windows),
this file is excluded and regular TCP is used.

### 2. Runtime Detection

**File**: `derp/derphttp/derphttp_client.go:318-329`

```go
func useWebsockets() bool {
    if !canWebsockets {
        return false
    }
    if runtime.GOOS == "js" {
        return true  // Always use WebSocket in browser
    }
    // ... other platform checks
    return false
}
```

When the DERP client needs to connect, it checks: "Am I running in a browser?"

### 3. WebSocket Dial Function

**File**: `derp/derphttp/websocket.go:23-34`

```go
func dialWebsocket(ctx context.Context, urlStr string) (net.Conn, error) {
    // Use github.com/coder/websocket to dial
    c, res, err := websocket.Dial(ctx, urlStr, &websocket.DialOptions{
        Subprotocols: []string{"derp"},  // Tell server we speak DERP
    })
    if err != nil {
        log.Printf("websocket Dial: %v, %+v", err, res)
        return nil, err
    }

    log.Printf("websocket: connected to %v", urlStr)

    // Convert WebSocket to net.Conn interface
    netConn := wsconn.NetConn(context.Background(), c, websocket.MessageBinary, urlStr)
    return netConn, nil
}
```

**The magic**: `wsconn.NetConn()` wraps the WebSocket connection to implement
Go's standard `net.Conn` interface. This means the WebSocket **looks exactly
like a TCP connection** to the rest of the code.

### 4. Connection Flow in WASM

**File**: `derp/derphttp/derphttp_client.go:392-424`

```go
func (c *Client) connect(ctx context.Context, caller string) (client *derp.Client, connGen int, err error) {
    // ... setup code ...

    switch {
    case canWebsockets && useWebsockets():
        // Running in browser, use WebSocket
        var urlStr string
        if c.url != nil {
            urlStr = c.url.String()
        } else {
            urlStr = c.urlString(reg.Nodes[0])
        }

        c.logf("%s: connecting websocket to %v", caller, urlStr)

        // This calls dialWebsocket() defined above
        conn, err := dialWebsocketFunc(ctx, urlStr)
        if err != nil {
            c.logf("%s: websocket to %v error: %v", caller, urlStr, err)
            return nil, 0, err
        }

        // Create DERP client with WebSocket connection
        brw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
        derpClient, err := derp.NewClient(c.privateKey, conn, brw, c.logf,
            derp.MeshKey(c.MeshKey),
            derp.CanAckPings(c.canAckPings),
            derp.IsProber(c.IsProber),
        )
        // ... rest works identically to TCP ...

    case c.url != nil:
        // Native platform, use regular TCP
        c.logf("%s: connecting to %v", caller, c.url)
        tcpConn, err = c.dialURL(ctx)
        // ... TCP connection handling ...
    }
}
```

## The Complete Picture

```
Your WASM Code                  DERP Client Library              Browser
──────────────                  ───────────────────              ───────

derpClient, err :=
  derphttp.NewClient(...)
                                Build tag: js detected
                                useWebsockets() returns true

derpClient.Send(data)
                                dialWebsocketFunc(url)
                                                                  WebSocket API
                                websocket.Dial()                  creates connection

                                wsconn.NetConn()
                                wraps WebSocket as net.Conn

                                derp.Client uses net.Conn
                                (doesn't know it's WebSocket!)

                                Writes data to connection
                                                                  Sends WebSocket
                                                                  message

                                                                  Over network →
                                                                  DERP server
```

## Key Takeaways

1. **Automatic**: You don't write different code for browser vs native. The
   library detects the platform.

2. **Transparent**: Once `wsconn.NetConn()` wraps the WebSocket, it implements
   `net.Conn`. The rest of the DERP client code doesn't know or care that it's
   WebSocket.

3. **Build tags**: Go's build system includes/excludes files based on target
   platform (`GOOS=js`).

4. **Same protocol**: DERP protocol is identical whether over TCP or WebSocket.
   Only the transport layer changes.

## What This Means For Our Code

When we write:

```go
// In browser/wasm/main.go
derpClient, err := derphttp.NewClient(privateKey, "https://derp.tailscale.com/derp", logf, netMon)
if err != nil {
    log.Fatal(err)
}

// Send WireGuard packet
err = derpClient.Send(peerPublicKey, packet)
```

**In the browser (WASM)**: Uses WebSocket automatically
**On native platform**: Uses TCP

**We write the code once, and it works everywhere.**

## DERP Server Support

The DERP server already supports both:
- Regular TCP connections on port 443 (HTTPS)
- WebSocket connections on the same port with `Upgrade: websocket` header

The server doesn't care which transport is used - it speaks the same DERP
protocol over both.

## Testing This

You can verify WebSocket is being used by:
1. Compiling your Go code with `GOOS=js GOARCH=wasm`
2. Loading it in the browser
3. Opening browser DevTools → Network tab
4. Filtering for "WS" (WebSocket)
5. You'll see a WebSocket connection to the DERP server

The connection will show:
- **Type**: websocket
- **Protocol**: derp (the subprotocol we specified)
- **Messages**: Binary frames containing DERP protocol data
