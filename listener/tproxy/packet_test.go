package tproxy

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func TestPacketFlowKeyReturnsTheLocalAddress(t *testing.T) {
	pkt := &packet{lAddr: netip.MustParseAddrPort("192.168.1.2:54321")}

	flowPacket, ok := any(pkt).(interface{ FlowKey() netip.AddrPort })
	require.True(t, ok)
	require.Equal(t, pkt.lAddr, flowPacket.FlowKey())
}

func TestNewPacketAdapterOnTProxyPacketSkipsTheUDPAddr(t *testing.T) {
	pkt := &packet{
		lAddr: netip.MustParseAddrPort("192.168.1.2:54321"),
		buf:   []byte("ping"),
	}
	metadata := &C.Metadata{}

	allocations := testing.AllocsPerRun(100, func() {
		adapter := C.NewPacketAdapter(pkt, metadata)
		if key := adapter.(interface{ FlowKey() netip.AddrPort }).FlowKey(); key != pkt.lAddr {
			t.Fatal("adapter lost the flow key")
		}
	})
	// the adapter struct itself; no net.UDPAddr is materialised for the flow key
	require.EqualValues(t, 1, allocations)
}
