package packet

import (
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/common/pool"
)

type WaitReadFrom interface {
	WaitReadFrom() (data []byte, addr netip.AddrPort, err error)
}

type EnhancePacketConn interface {
	net.PacketConn
	WaitReadFrom
}

func AddrPortFromNetAddr(addr net.Addr) netip.AddrPort {
	if udpAddr, isUDPAddr := addr.(*net.UDPAddr); isUDPAddr && udpAddr != nil {
		return udpAddr.AddrPort()
	}
	return netip.AddrPort{}
}

func NewEnhancePacketConn(pc net.PacketConn) EnhancePacketConn {
	if udpConn, isUDPConn := pc.(*net.UDPConn); isUDPConn {
		return &enhanceUDPConn{UDPConn: udpConn}
	}
	if enhancePC, isEnhancePC := pc.(EnhancePacketConn); isEnhancePC {
		return enhancePC
	}
	if singPC, isSingPC := pc.(SingPacketConn); isSingPC {
		return newEnhanceSingPacketConn(singPC)
	}
	if withBufferPC, isWithBufferPC := pc.(withBufferPacketConn); isWithBufferPC {
		return &enhanceWithBufferPacketConn{withBufferPacketConn: withBufferPC}
	}
	return &enhancePacketConn{PacketConn: pc}
}

type enhancePacketConn struct {
	net.PacketConn
}

// WaitReadFrom hands the caller mihomo-pool memory: recycle it with pool.Put
// once the packet has been handled. A zero-length datagram returns no buffer.
func (c *enhancePacketConn) WaitReadFrom() (data []byte, addr netip.AddrPort, err error) {
	return waitReadFrom(c.PacketConn)
}

func (c *enhancePacketConn) Upstream() any {
	return c.PacketConn
}

func (c *enhancePacketConn) WriterReplaceable() bool {
	return true
}

func (c *enhancePacketConn) ReaderReplaceable() bool {
	return true
}

type enhanceUDPConn struct {
	*net.UDPConn
}

// WaitReadFrom recvfrom's into pooled memory through the udp conn's own
// poller operation: the AddrPort variant fills a stack sockaddr, so nothing on
// this path allocates, and a raw syscall beside the poller would instead heap
// allocate the callback frame once per datagram.
func (c *enhanceUDPConn) WaitReadFrom() (data []byte, addr netip.AddrPort, err error) {
	readBuf := pool.Get(pool.UDPBufferSize)
	readN, addr, err := c.ReadFromUDPAddrPort(readBuf)
	if err != nil {
		_ = pool.Put(readBuf)
		return nil, addr, err
	}
	if readN == 0 {
		// A zero-length datagram is a successful UDP read, not EOF.
		_ = pool.Put(readBuf)
		return nil, addr, nil
	}
	return readBuf[:readN], addr, nil
}

func (c *enhanceUDPConn) Upstream() any {
	return c.UDPConn
}

func (c *enhanceUDPConn) WriterReplaceable() bool {
	return true
}

func (c *enhanceUDPConn) ReaderReplaceable() bool {
	return true
}

func waitReadFrom(pc net.PacketConn) (data []byte, addr netip.AddrPort, err error) {
	readBuf := pool.Get(pool.UDPBufferSize)
	var readN int
	var rawAddr net.Addr
	readN, rawAddr, err = pc.ReadFrom(readBuf)
	if err != nil {
		_ = pool.Put(readBuf)
		return nil, AddrPortFromNetAddr(rawAddr), err
	}
	if readN == 0 {
		_ = pool.Put(readBuf)
		return nil, AddrPortFromNetAddr(rawAddr), nil
	}
	return readBuf[:readN], AddrPortFromNetAddr(rawAddr), nil
}
