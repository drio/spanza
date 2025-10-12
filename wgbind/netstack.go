package wgbind

import (
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
	mu   sync.Mutex
	tnet *netstack.Net
	conn *gonet.UDPConn
}

var _ conn.Bind = (*NetstackBind)(nil)

// NewNetstackBind creates a new Bind that uses userspace UDP from the provided
// netstack.Net. The tnet parameter comes from netstack.CreateNetTUN().
func NewNetstackBind(tnet *netstack.Net) conn.Bind {
	return &NetstackBind{
		tnet: tnet,
	}
}

// NetstackEndpoint represents a UDP endpoint for the netstack bind.
type NetstackEndpoint struct {
	addr netip.AddrPort
}

var _ conn.Endpoint = (*NetstackEndpoint)(nil)

func (e *NetstackEndpoint) ClearSrc() {
	// No-op for simple implementation
}

func (e *NetstackEndpoint) DstIP() netip.Addr {
	return e.addr.Addr()
}

func (e *NetstackEndpoint) SrcIP() netip.Addr {
	// Return unspecified since we don't track source in simple implementation
	return netip.Addr{}
}

func (e *NetstackEndpoint) SrcToString() string {
	return ""
}

func (e *NetstackEndpoint) DstToString() string {
	return e.addr.String()
}

func (e *NetstackEndpoint) DstToBytes() []byte {
	b, _ := e.addr.MarshalBinary()
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

	// Get the actual port we bound to
	localAddr := udpConn.LocalAddr().(*net.UDPAddr)
	actualPort := uint16(localAddr.Port)

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
	addrPort := udpAddr.AddrPort()
	eps[0] = &NetstackEndpoint{addr: addrPort}

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
	addr := net.UDPAddrFromAddrPort(ep.addr)

	// Simple implementation: send packets one at a time
	for _, buf := range bufs {
		_, err := udpConn.WriteTo(buf, addr)
		if err != nil {
			return err
		}
	}

	return nil
}

// ParseEndpoint parses a string into an endpoint.
func (b *NetstackBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	addr, err := netip.ParseAddrPort(s)
	if err != nil {
		return nil, err
	}
	return &NetstackEndpoint{addr: addr}, nil
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
