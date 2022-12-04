package inbound

import (
	C "github.com/Dreamacro/clash/constant"
	"github.com/Dreamacro/clash/transport/socks5"
)

// PacketAdapter is a UDP Packet adapter for socks/redir/tun
type PacketAdapter struct {
	C.UDPPacket
	metadata *C.Metadata
}

// Metadata returns destination metadata
func (s *PacketAdapter) Metadata() *C.Metadata {
	return s.metadata
}

func NewPacketWithInfos(target socks5.Addr, packet C.UDPPacket, source C.Type, inName, specialRules string) C.PacketAdapter {
	metadata := parseSocksAddr(target)
	metadata.NetWork = C.UDP
	metadata.Type = source
	metadata.InName = inName
	metadata.SpecialRules = specialRules
	if ip, port, err := parseAddr(packet.LocalAddr().String()); err == nil {
		metadata.SrcIP = ip
		metadata.SrcPort = port
	}
	if p, ok := packet.(C.UDPPacketInAddr); ok {
		if ip, port, err := parseAddr(p.InAddr().String()); err == nil {
			metadata.InIP = ip
			metadata.InPort = port
		}
	}

	return &PacketAdapter{
		packet,
		metadata,
	}
}

// NewPacket is PacketAdapter generator
func NewPacket(target socks5.Addr, packet C.UDPPacket, source C.Type) C.PacketAdapter {
	return NewPacketWithInfos(target, packet, source, "", "")
}
