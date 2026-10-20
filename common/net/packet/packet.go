package packet

import (
	"net"
	"net/netip"
	"strconv"
	"syscall"
	"unsafe"

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

// sockaddrInet4ToAddrPort converts a kernel AF_INET sockaddr laid out exactly
// as the platform ABI defines it; the port travels in wire order.
func sockaddrInet4ToAddrPort(pp *syscall.RawSockaddrInet4) netip.AddrPort {
	portBytes := (*[2]byte)(unsafe.Pointer(&pp.Port))
	port := uint16(portBytes[0])<<8 | uint16(portBytes[1])
	return netip.AddrPortFrom(netip.AddrFrom4(pp.Addr), port)
}

// sockaddrInet6ToAddrPort is sockaddrInet4ToAddrPort for AF_INET6, mapping the
// kernel scope id back into the address zone.
func sockaddrInet6ToAddrPort(pp *syscall.RawSockaddrInet6) netip.AddrPort {
	portBytes := (*[2]byte)(unsafe.Pointer(&pp.Port))
	port := uint16(portBytes[0])<<8 | uint16(portBytes[1])
	zone := ""
	if pp.Scope_id != 0 {
		zone = strconv.FormatUint(uint64(pp.Scope_id), 10)
	}
	return netip.AddrPortFrom(netip.AddrFrom16(pp.Addr).WithZone(zone).Unmap(), port)
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
