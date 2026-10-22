package trojan

import (
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/adapter/inbound"
	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
)

// The wrapper is allocated per datagram on purpose: a flow keeps the packet it
// last handed the sender so replies can be written back through it, and Drop
// runs long before that flow ends. Recycling the object here would let the next
// datagram claim it and move those replies onto another client's socket.
func newTrojanPacket(pc trojanUDPConn, source netip.AddrPort, data []byte) *packet {
	return &packet{
		pc:      pc,
		source:  source,
		data:    data,
		payload: data,
	}
}

type packet struct {
	pc      trojanUDPConn
	source  netip.AddrPort
	data    []byte
	payload []byte
}

type trojanUDPConn interface {
	net.PacketConn
	WriteToAddrPort([]byte, netip.AddrPort) (int, error)
}

const trojanUDPFlowCacheSize = 256

type trojanUDPSession struct {
	pc        trojanUDPConn
	source    netip.AddrPort
	tunnel    C.Tunnel
	additions []inbound.Addition
	flows     map[netip.AddrPort]*C.Metadata
}

func newTrojanUDPSession(
	pc trojanUDPConn,
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
	return c.pc.WriteToAddrPort(b, addr)
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
}

func (c *packet) InAddr() net.Addr {
	return c.pc.LocalAddr()
}
