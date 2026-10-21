package tunnel

import (
	"net"
	"net/netip"
	"sync"

	"github.com/metacubex/mihomo/common/pool"
)

// A wrapper is returned only by Drop, the tunnel contract's single release point.
var tunnelPacketPool = sync.Pool{New: func() interface{} { return new(packet) }}

func newTunnelPacket(pc net.PacketConn, rAddr net.Addr, payload []byte) *packet {
	tunnelPacket := tunnelPacketPool.Get().(*packet)
	*tunnelPacket = packet{
		pc:      pc,
		rAddr:   rAddr,
		payload: payload,
	}
	return tunnelPacket
}

type packet struct {
	pc      net.PacketConn
	rAddr   net.Addr
	payload []byte
}

func (c *packet) Data() []byte {
	return c.payload
}

// WriteBack write UDP packet with source(ip, port) = `addr`
func (c *packet) WriteBack(b []byte, addr netip.AddrPort) (n int, err error) {
	return c.pc.WriteTo(b, c.rAddr)
}

// LocalAddr returns the source IP/Port of UDP Packet
func (c *packet) LocalAddr() net.Addr {
	return c.rAddr
}

func (c *packet) Drop() {
	if c.payload == nil {
		return
	}
	_ = pool.Put(c.payload)
	c.payload = nil
	tunnelPacketPool.Put(c)
}

func (c *packet) InAddr() net.Addr {
	return c.pc.LocalAddr()
}
