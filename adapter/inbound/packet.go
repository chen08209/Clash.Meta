package inbound

import (
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
)

// NewPacket is PacketAdapter generator
func NewPacket(target socks5.Addr, packet C.UDPPacket, source C.Type, additions ...Addition) (C.UDPPacket, *C.Metadata) {
	metadata := parseSocksAddr(target)
	metadata.NetWork = C.UDP
	metadata.Type = source
	SetSrcAddr(metadata, packet.LocalAddr())
	if p, ok := packet.(C.UDPPacketInAddr); ok {
		SetInAddr(metadata, p.InAddr())
	}
	ApplyAdditions(metadata, additions...)

	return packet, metadata
}
