package packet

import (
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/common/pool"
)

type withBufferPacketConn interface {
	net.PacketConn
	ReadFromWithBuffer(func(sizeHint int) []byte) (int, net.Addr, error)
}

type enhanceWithBufferPacketConn struct {
	withBufferPacketConn
}

func (c *enhanceWithBufferPacketConn) WaitReadFrom() (data []byte, put func(), addr netip.AddrPort, err error) {
	var readBuf []byte
	getBuffer := func(sizeHint int) []byte {
		readBuf = pool.Get(sizeHint)
		put = func() {
			_ = pool.Put(readBuf)
		}
		return readBuf
	}
	var (
		readN   int
		rawAddr net.Addr
	)
	readN, rawAddr, err = c.ReadFromWithBuffer(getBuffer)
	addr = AddrPortFromNetAddr(rawAddr)
	if readN > 0 && readBuf != nil {
		data = readBuf[:readN]
	} else if put != nil {
		put()
		put = nil
	}
	return
}

func (c *enhanceWithBufferPacketConn) Upstream() any {
	return c.withBufferPacketConn
}

func (c *enhanceWithBufferPacketConn) WriterReplaceable() bool {
	return true
}

func (c *enhanceWithBufferPacketConn) ReaderReplaceable() bool {
	return true
}
