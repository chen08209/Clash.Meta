package packet

import (
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/common/pool"
)

type WaitReadFrom interface {
	WaitReadFrom() (data []byte, put func(), addr net.Addr, err error)
}

type AddrPortWaitReadFrom interface {
	WaitReadFromAddrPort() (data []byte, put func(), addr netip.AddrPort, rawAddr net.Addr, err error)
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

func WaitReadFromAddrPort(pc EnhancePacketConn) (data []byte, put func(), addr netip.AddrPort, rawAddr net.Addr, err error) {
	if fast, isFast := pc.(AddrPortWaitReadFrom); isFast {
		return fast.WaitReadFromAddrPort()
	}
	data, put, rawAddr, err = pc.WaitReadFrom()
	return data, put, AddrPortFromNetAddr(rawAddr), rawAddr, err
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

func (c *enhancePacketConn) WaitReadFrom() (data []byte, put func(), addr net.Addr, err error) {
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

func (c *enhanceUDPConn) Upstream() any {
	return c.UDPConn
}

func (c *enhanceUDPConn) WriterReplaceable() bool {
	return true
}

func (c *enhanceUDPConn) ReaderReplaceable() bool {
	return true
}

func waitReadFrom(pc net.PacketConn) (data []byte, put func(), addr net.Addr, err error) {
	readBuf := pool.Get(pool.UDPBufferSize)
	put = func() {
		_ = pool.Put(readBuf)
	}
	var readN int
	readN, addr, err = pc.ReadFrom(readBuf)
	if readN > 0 {
		data = readBuf[:readN]
	} else {
		put()
		put = nil
	}
	return
}
