package trojan

import (
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func TestPacketFlowKeyReturnsTheSource(t *testing.T) {
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	pkt := &packet{source: source}

	flowPacket, ok := any(pkt).(interface{ FlowKey() netip.AddrPort })
	require.True(t, ok)
	require.Equal(t, source, flowPacket.FlowKey())
	require.Equal(t, source.String(), pkt.LocalAddr().String())
}

func TestNewPacketAdapterOnTrojanPacketSkipsTheUDPAddr(t *testing.T) {
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	pkt := &packet{source: source}
	metadata := &C.Metadata{}
	C.NewPacketAdapter(pkt, metadata).Drop()

	allocations := testing.AllocsPerRun(100, func() {
		adapter := C.NewPacketAdapter(pkt, metadata)
		if key := adapter.(interface{ FlowKey() netip.AddrPort }).FlowKey(); key != source {
			t.Fatal("adapter lost the flow key")
		}
		adapter.Drop()
	})
	// The adapter is the only allocation: it stays allocated because the flow
	// keeps it in its write-back proxy past Drop, so no UDPAddr is materialised.
	require.EqualValues(t, 1, allocations)
}

func BenchmarkTrojanPacketDrop(b *testing.B) {
	pc := &fixedLocalPacketConn{}
	source := netip.MustParseAddrPort("192.168.1.2:54321")

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pkt := newTrojanPacket(pc, source, pool.Get(64))
		pkt.Drop()
	}
}
