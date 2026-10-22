package inbound

import (
	"net/netip"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
)

// NewPacket is PacketAdapter generator
func NewPacket(target socks5.Addr, packet C.UDPPacket, source C.Type, additions ...Addition) (C.UDPPacket, *C.Metadata) {
	return finishPacket(parseSocksAddr(target), packet, source, additions...)
}

// NewPacketFromAddrPort is NewPacket for a listener that already holds its
// destination as an AddrPort: encoding it into a socks5 header only for
// parseSocksAddr to decode it again allocated that header per datagram.
func NewPacketFromAddrPort(dst netip.AddrPort, packet C.UDPPacket, source C.Type, additions ...Addition) (C.UDPPacket, *C.Metadata) {
	return finishPacket(&C.Metadata{
		DstIP:   dst.Addr().Unmap(),
		DstPort: dst.Port(),
	}, packet, source, additions...)
}

func finishPacket(metadata *C.Metadata, packet C.UDPPacket, source C.Type, additions ...Addition) (C.UDPPacket, *C.Metadata) {
	metadata.NetWork = C.UDP
	metadata.Type = source
	setSrcAddr(metadata, packet)
	if p, ok := packet.(C.UDPPacketInAddr); ok {
		SetInAddr(metadata, p.InAddr())
	}
	ApplyAdditions(metadata, additions...)

	return packet, metadata
}

// setSrcAddr reads the source a packet already carries as an AddrPort, so it
// does not materialise a net.UDPAddr only for SetSrcAddr to parse it back.
func setSrcAddr(metadata *C.Metadata, packet C.UDPPacket) {
	if keyPacket, ok := packet.(interface{ FlowKey() netip.AddrPort }); ok {
		if key := keyPacket.FlowKey(); key.IsValid() {
			metadata.SrcIP = key.Addr().Unmap()
			metadata.SrcPort = key.Port()
			return
		}
	}
	SetSrcAddr(metadata, packet.LocalAddr())
}
