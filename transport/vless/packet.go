package vless

import (
	"encoding/binary"
	"io"
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/common/pool"
)

type PacketConn struct {
	net.Conn
	rAddr netip.AddrPort
}

// WriteTo sends the length and the payload as one frame: writing the two-byte
// header on its own cost a second Write on the stream and an allocation.
func (c *PacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	buf := pool.Get(2 + len(b))
	defer pool.Put(buf)

	binary.BigEndian.PutUint16(buf, uint16(len(b)))
	copy(buf[2:], b)
	if _, err := c.Conn.Write(buf[:2+len(b)]); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *PacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	var length uint16
	err := binary.Read(c.Conn, binary.BigEndian, &length)
	if err != nil {
		return 0, nil, err
	}
	if len(b) < int(length) {
		return 0, nil, io.ErrShortBuffer
	}
	n, err := io.ReadFull(c.Conn, b[:length])
	return n, net.UDPAddrFromAddrPort(c.rAddr), err
}

func (c *PacketConn) WaitReadFrom() (data []byte, addr netip.AddrPort, err error) {
	var length uint16
	err = binary.Read(c.Conn, binary.BigEndian, &length)
	if err != nil {
		return
	}
	readBuf := pool.Get(int(length))
	n, err := io.ReadFull(c.Conn, readBuf)
	if err != nil {
		_ = pool.Put(readBuf)
		return
	}
	data = readBuf[:n]
	addr = c.rAddr
	return
}
