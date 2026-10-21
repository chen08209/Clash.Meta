package inner

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/adapter/inbound"
	"github.com/metacubex/mihomo/common/net/deadline"
	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
)

const innerUDPQueueSize = 128

var innerUDPConnID atomic.Uint64

type innerUDPAddr struct {
	network string
	address string
}

func (a innerUDPAddr) Network() string { return a.network }
func (a innerUDPAddr) String() string  { return a.address }

type innerUDPDatagram struct {
	data []byte
	addr netip.AddrPort
}

type innerUDPPacketConn struct {
	tunnel    C.Tunnel
	proxy     string
	target    socks5.Addr
	metadata  *C.Metadata
	localAddr net.Addr
	readChan  chan innerUDPDatagram
	done      chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
	closed    bool

	readDeadline  deadline.PipeDeadline
	writeDeadline deadline.PipeDeadline
}

func HandleUdp(tunnel C.Tunnel, network, address, proxy string) (net.PacketConn, net.Addr, error) {
	if tunnel == nil {
		return nil, nil, fmt.Errorf("tunnel uninitialized")
	}
	switch network {
	case "udp", "udp4", "udp6":
	default:
		return nil, nil, fmt.Errorf("unsupported network %s", network)
	}
	if socks5.ParseAddr(address) == nil {
		return nil, nil, fmt.Errorf("invalid target address %s", address)
	}
	target := socks5.ParseAddr(address)

	id := innerUDPConnID.Add(1)
	conn := &innerUDPPacketConn{
		tunnel:        tunnel,
		proxy:         proxy,
		target:        target,
		localAddr:     innerUDPAddr{network: network, address: fmt.Sprintf("inner-udp-%d", id)},
		readChan:      make(chan innerUDPDatagram, innerUDPQueueSize),
		done:          make(chan struct{}),
		readDeadline:  deadline.MakePipeDeadline(),
		writeDeadline: deadline.MakePipeDeadline(),
	}
	// The target is fixed per conn, so the metadata is parsed once and shared
	// by every datagram; only the tunnel's worker goroutine touches it after
	// handoff and its in-place fixes are idempotent.
	probe := &innerUDPPacket{conn: conn}
	_, metadata := inbound.NewPacket(target, probe, C.INNER)
	metadata.DNSMode = C.DNSNormal
	metadata.Process = C.MihomoName
	metadata.SpecialProxy = proxy
	conn.metadata = metadata
	return conn, innerUDPAddr{network: network, address: address}, nil
}

func (c *innerUDPPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	select {
	case <-c.done:
		return 0, nil, net.ErrClosed
	case <-c.readDeadline.Wait():
		return 0, nil, os.ErrDeadlineExceeded
	case datagram := <-c.readChan:
		n := copy(p, datagram.data)
		_ = pool.Put(datagram.data)
		if n < len(datagram.data) {
			return n, net.UDPAddrFromAddrPort(datagram.addr), io.ErrShortBuffer
		}
		return n, net.UDPAddrFromAddrPort(datagram.addr), nil
	}
}

func (c *innerUDPPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if addr == nil {
		return 0, errors.New("missing target address")
	}
	select {
	case <-c.done:
		return 0, net.ErrClosed
	case <-c.writeDeadline.Wait():
		return 0, os.ErrDeadlineExceeded
	default:
	}

	data := pool.Get(len(p))
	copy(data, p)
	udpPacket := newInnerUDPPacket(c, data)
	c.tunnel.HandleUDPPacket(udpPacket, c.metadata)
	return len(p), nil
}

func (c *innerUDPPacketConn) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		close(c.done)
		for {
			select {
			case datagram := <-c.readChan:
				_ = pool.Put(datagram.data)
			default:
				c.mu.Unlock()
				return
			}
		}
	})
	return nil
}

func (c *innerUDPPacketConn) LocalAddr() net.Addr { return c.localAddr }

func (c *innerUDPPacketConn) SetDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	c.writeDeadline.Set(t)
	return nil
}

func (c *innerUDPPacketConn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	return nil
}

func (c *innerUDPPacketConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline.Set(t)
	return nil
}

func (c *innerUDPPacketConn) writeBack(data []byte, addr netip.AddrPort) (int, error) {
	payload := pool.Get(len(data))
	copy(payload, data)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		_ = pool.Put(payload)
		return 0, net.ErrClosed
	}
	select {
	case c.readChan <- innerUDPDatagram{data: payload, addr: addr}:
	default:
		_ = pool.Put(payload)
	}
	return len(data), nil
}

var innerUDPPacketPool = sync.Pool{New: func() any { return new(innerUDPPacket) }}

func newInnerUDPPacket(conn *innerUDPPacketConn, data []byte) *innerUDPPacket {
	packet := innerUDPPacketPool.Get().(*innerUDPPacket)
	*packet = innerUDPPacket{
		conn: conn,
		data: data,
	}
	return packet
}

type innerUDPPacket struct {
	conn *innerUDPPacketConn
	data []byte
}

func (p *innerUDPPacket) Data() []byte { return p.data }

func (p *innerUDPPacket) WriteBack(data []byte, addr netip.AddrPort) (int, error) {
	return p.conn.writeBack(data, addr)
}

func (p *innerUDPPacket) LocalAddr() net.Addr { return p.conn.localAddr }
func (p *innerUDPPacket) InAddr() net.Addr    { return p.conn.localAddr }

func (p *innerUDPPacket) Drop() {
	if p.data == nil {
		return
	}
	_ = pool.Put(p.data)
	p.data = nil
	p.conn = nil
	innerUDPPacketPool.Put(p)
}

var _ net.PacketConn = (*innerUDPPacketConn)(nil)
var _ C.UDPPacketInAddr = (*innerUDPPacket)(nil)
