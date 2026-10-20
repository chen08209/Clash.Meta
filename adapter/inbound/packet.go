package inbound

import (
	"net/netip"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
)

// NewPacket is PacketAdapter generator
func NewPacket(target socks5.Addr, packet C.UDPPacket, source C.Type, additions ...Addition) (C.UDPPacket, *C.Metadata) {
	metadata := parseSocksAddr(target)
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
