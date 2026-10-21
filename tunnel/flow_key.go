package tunnel

import (
	"hash/maphash"
	"net/netip"

	C "github.com/metacubex/mihomo/constant"
)

var flowHashSeed = maphash.MakeSeed()

func packetFlowKey(packet C.PacketAdapter) (netip.AddrPort, bool) {
	if flowPacket, ok := packet.(interface {
		FlowKey() netip.AddrPort
	}); ok {
		key := flowPacket.FlowKey()
		if key.IsValid() {
			return key, true
		}
		return netip.AddrPort{}, false
	}

	key, err := netip.ParseAddrPort(packet.Key())
	if err != nil || !key.IsValid() {
		return netip.AddrPort{}, false
	}
	return key, true
}
