package socks

import (
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/metacubex/mihomo/transport/socks5"
)

type packet struct {
	pc      net.PacketConn
	rAddr   net.Addr
	payload []byte
	put     func()
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
	if c.put != nil {
		c.put()
		c.put = nil
	}
	c.payload = nil
}

func (c *packet) InAddr() net.Addr {
	return c.pc.LocalAddr()
}
