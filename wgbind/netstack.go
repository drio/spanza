package wgbind

import (
	"log"
	"net"
	"net/netip"
	"sync"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
)

// NetstackBind implements conn.Bind for userspace UDP using gvisor's netstack.
// This allows WireGuard to work in WASM and other environments without kernel UDP access.
//
// Unlike StdNetBind which uses kernel UDP (net.ListenUDP), NetstackBind uses
// the userspace network stack (tnet.ListenUDP) from netstack.
type NetstackBind struct {
	mu       sync.Mutex
	tnet     *netstack.Net
	conn     *gonet.UDPConn
	localIP  netip.Addr      // Local IP address for this bind
	localPort uint16         // Local port for this bind
}

var _ conn.Bind = (*NetstackBind)(nil)

// NewNetstackBind creates a new Bind that uses userspace UDP from the provided
// netstack.Net. The tnet parameter comes from netstack.CreateNetTUN().
// The localIP parameter specifies the local IP address to use (e.g., "192.168.4.2").
func NewNetstackBind(tnet *netstack.Net, localIP string) conn.Bind {
	ip, _ := netip.ParseAddr(localIP)
	return &NetstackBind{
		tnet:    tnet,
		localIP: ip,
	}
}

// NetstackEndpoint represents a UDP endpoint for the netstack bind.
type NetstackEndpoint struct {
	dst netip.AddrPort // Destination address (remote peer)
	src netip.AddrPort // Source address (local interface)
}

var _ conn.Endpoint = (*NetstackEndpoint)(nil)

func (e *NetstackEndpoint) ClearSrc() {
	e.src = netip.AddrPort{}
}

func (e *NetstackEndpoint) DstIP() netip.Addr {
	return e.dst.Addr()
}

func (e *NetstackEndpoint) SrcIP() netip.Addr {
	return e.src.Addr()
}

func (e *NetstackEndpoint) SrcToString() string {
	return e.src.String()
}

func (e *NetstackEndpoint) DstToString() string {
	return e.dst.String()
}

func (e *NetstackEndpoint) DstToBytes() []byte {
	b, _ := e.dst.MarshalBinary()
	return b
}

// Open creates a UDP listener on the specified port using userspace networking.
func (b *NetstackBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.conn != nil {
		return nil, 0, conn.ErrBindAlreadyOpen
	}

	// Listen on all interfaces within the userspace network
	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: int(port),
	}

	udpConn, err := b.tnet.ListenUDP(addr)
	if err != nil {
		return nil, 0, err
	}

	b.conn = udpConn

	// Get the actual port we bound to and extract local address
	localAddr := udpConn.LocalAddr().(*net.UDPAddr)
	actualPort := uint16(localAddr.Port)
	b.localPort = actualPort

	log.Printf("[wgbind] Bound to %s:%d", b.localIP, actualPort)

	// Return a single receive function
	recvFn := func(bufs [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		return b.receive(bufs, sizes, eps)
	}

	return []conn.ReceiveFunc{recvFn}, actualPort, nil
}

// receive reads packets from the UDP connection.
func (b *NetstackBind) receive(bufs [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
	b.mu.Lock()
	udpConn := b.conn
	b.mu.Unlock()

	if udpConn == nil {
		return 0, net.ErrClosed
	}

	// Simple implementation: read one packet at a time
	// WireGuard will call this repeatedly as needed
	n, addr, err := udpConn.ReadFrom(bufs[0])
	if err != nil {
		return 0, err
	}

	sizes[0] = n

	// Convert net.Addr to netip.AddrPort
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return 0, net.ErrClosed
	}

	// The address from ReadFrom is the SOURCE of the packet (where it came from)
	// This becomes the DESTINATION for our replies (dst)
	dstAddrPort := udpAddr.AddrPort()

	// For source, use our configured local address
	srcAddrPort := netip.AddrPortFrom(b.localIP, b.localPort)

	eps[0] = &NetstackEndpoint{
		dst: dstAddrPort,
		src: srcAddrPort,
	}

	log.Printf("[wgbind] Received %d bytes from %s", n, dstAddrPort)
	log.Printf("[wgbind] Endpoint - Src: %s, Dst: %s", srcAddrPort, dstAddrPort)

	return 1, nil
}

// Close closes the UDP connection.
func (b *NetstackBind) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.conn == nil {
		return nil
	}

	err := b.conn.Close()
	b.conn = nil
	return err
}

// Send writes packets to the specified endpoint.
func (b *NetstackBind) Send(bufs [][]byte, endpoint conn.Endpoint) error {
	b.mu.Lock()
	udpConn := b.conn
	b.mu.Unlock()

	if udpConn == nil {
		return net.ErrClosed
	}

	ep, ok := endpoint.(*NetstackEndpoint)
	if !ok {
		return conn.ErrWrongEndpointType
	}

	// Convert netip.AddrPort to *net.UDPAddr
	// Send to the destination (remote peer)
	addr := net.UDPAddrFromAddrPort(ep.dst)

	// Simple implementation: send packets one at a time
	for _, buf := range bufs {
		n, err := udpConn.WriteTo(buf, addr)
		if err != nil {
			return err
		}
		log.Printf("[wgbind] Sent %d bytes to %s", n, addr)
	}

	return nil
}

// ParseEndpoint parses a string into an endpoint.
// The parsed address becomes the destination (remote peer).
func (b *NetstackBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	addr, err := netip.ParseAddrPort(s)
	if err != nil {
		return nil, err
	}
	// Destination is the parsed address (remote peer)
	// Source will be set when we receive packets
	return &NetstackEndpoint{
		dst: addr,
	}, nil
}

// SetMark is a no-op for userspace networking.
// Socket marks are a kernel feature not applicable to userspace UDP.
func (b *NetstackBind) SetMark(mark uint32) error {
	return nil
}

// BatchSize returns 1 since we use simple single-packet operations.
// This could be optimized later if needed.
func (b *NetstackBind) BatchSize() int {
	return 1
}
