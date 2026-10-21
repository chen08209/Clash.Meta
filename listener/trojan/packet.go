package trojan

import (
	"net"
	"net/netip"
	"sync"

	"github.com/metacubex/mihomo/adapter/inbound"
	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
)

// A wrapper is returned only by Drop, the tunnel contract's single release point.
var trojanPacketPool = sync.Pool{New: func() any { return new(packet) }}

func newTrojanPacket(pc net.PacketConn, source netip.AddrPort, data []byte) *packet {
	pkt := trojanPacketPool.Get().(*packet)
	*pkt = packet{
		pc:      pc,
		source:  source,
		data:    data,
		payload: data,
	}
	return pkt
}

type packet struct {
	pc      net.PacketConn
	source  netip.AddrPort
	data    []byte
	payload []byte
}

const trojanUDPFlowCacheSize = 256

type trojanUDPSession struct {
	pc        net.PacketConn
	source    netip.AddrPort
	tunnel    C.Tunnel
	additions []inbound.Addition
	flows     map[netip.AddrPort]*C.Metadata
}

func newTrojanUDPSession(
	pc net.PacketConn,
	source netip.AddrPort,
	tunnel C.Tunnel,
	additions []inbound.Addition,
) trojanUDPSession {
	return trojanUDPSession{
		pc:        pc,
		source:    source,
		tunnel:    tunnel,
		additions: additions,
	}
}

func (s *trojanUDPSession) handle(data []byte, addr netip.AddrPort) {
	metadata, ok := s.flows[addr]
	pkt := newTrojanPacket(s.pc, s.source, data)
	if !ok {
		if s.flows == nil {
			s.flows = make(map[netip.AddrPort]*C.Metadata, 8)
		} else if len(s.flows) >= trojanUDPFlowCacheSize {
			clear(s.flows)
		}
		_, metadata = inbound.NewPacketFromAddrPort(addr, pkt, C.TROJAN, s.additions...)
		s.flows[addr] = metadata
	}
	s.tunnel.HandleUDPPacket(pkt, metadata)
}

func (c *packet) Data() []byte {
	return c.payload
}

// FlowKey returns the source address the tunnel keys this packet's flow by.
func (c *packet) FlowKey() netip.AddrPort {
	return c.source
}

// WriteBack wirtes UDP packet with source(ip, port) = `addr`
func (c *packet) WriteBack(b []byte, addr netip.AddrPort) (n int, err error) {
	return c.pc.WriteTo(b, net.UDPAddrFromAddrPort(addr))
}

// LocalAddr returns the source IP/Port of UDP Packet
func (c *packet) LocalAddr() net.Addr {
	return net.UDPAddrFromAddrPort(c.source)
}

func (c *packet) Drop() {
	if c.data == nil {
		return
	}
	_ = pool.Put(c.data)
	c.data = nil
	c.payload = nil
	trojanPacketPool.Put(c)
}

func (c *packet) InAddr() net.Addr {
	return c.pc.LocalAddr()
}
