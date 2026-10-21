package shadowsocks

import (
	"net"
	"net/netip"
	"sync"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/metacubex/mihomo/transport/socks5"

	"github.com/metacubex/mhurl"
)

// A wrapper is returned only by Drop, the tunnel contract's single release point.
var ssPacketPool = sync.Pool{New: func() interface{} { return new(packet) }}

func newSSPacket(pc net.PacketConn, rAddr net.Addr, data, payload []byte) *packet {
	ssPacket := ssPacketPool.Get().(*packet)
	*ssPacket = packet{
		pc:      pc,
		rAddr:   rAddr,
		data:    data,
		payload: payload,
	}
	return ssPacket
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

// WriteBack wirtes UDP packet with source(ip, port) = `addr`
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
	ssPacketPool.Put(c)
}

func (c *packet) InAddr() net.Addr {
	return c.pc.LocalAddr()
}

func ParseSSURL(s string) (addr, cipher, password string, err error) {
	u, err := mhurl.Parse(s) // we need multiple hosts url supports
	if err != nil {
		return
	}

	addr = u.Host
	if u.User != nil {
		cipher = u.User.Username()
		password, _ = u.User.Password()
	}
	return
}
