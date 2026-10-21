package socks

import (
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/metacubex/mihomo/transport/socks5"
)

// The wrapper is allocated per datagram on purpose: a flow keeps the packet it
// last handed the sender so replies can be written back through it, and Drop
// runs long before that flow ends. Recycling the object here would let the next
// datagram claim it and move those replies onto another client's socket.
func newSocksPacket(pc net.PacketConn, rAddr net.Addr, data, payload []byte) *packet {
	return &packet{
		pc:      pc,
		rAddr:   rAddr,
		data:    data,
		payload: payload,
	}
}

type packet struct {
	pc      net.PacketConn
	rAddr   net.Addr
	data    []byte
	payload []byte
}

func (c *packet) Data() []byte {
	return c.payload
}

// WriteBack write UDP packet with source(ip, port) = `addr`
func (c *packet) WriteBack(b []byte, addr netip.AddrPort) (n int, err error) {
	buf := pool.Get(socks5.UDPPacketHeadroom + len(b))
	defer pool.Put(buf)
	packet, err := socks5.EncodeUDPPacketInto(buf, addr, b)
	if err != nil {
		return
	}
	return c.pc.WriteTo(packet, c.rAddr)
}

// LocalAddr returns the source IP/Port of UDP Packet
func (c *packet) LocalAddr() net.Addr {
	return c.rAddr
}

func (c *packet) Drop() {
	if c.data == nil {
		return
	}
	_ = pool.Put(c.data)
	c.data = nil
	c.payload = nil
}

func (c *packet) InAddr() net.Addr {
	return c.pc.LocalAddr()
}
