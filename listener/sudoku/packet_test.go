package sudoku

import (
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
	tsudoku "github.com/metacubex/mihomo/transport/sudoku"
	"github.com/stretchr/testify/require"
)

func TestUoTPacketFlowKeyReturnsTheSource(t *testing.T) {
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	pkt := &uotPacket{source: source}

	flowPacket, ok := any(pkt).(interface{ FlowKey() netip.AddrPort })
	require.True(t, ok)
	require.Equal(t, source, flowPacket.FlowKey())
	require.Equal(t, source.String(), pkt.LocalAddr().String())
}

func TestNewPacketAdapterOnUoTPacketSkipsTheUDPAddr(t *testing.T) {
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	pkt := &uotPacket{source: source}
	metadata := &C.Metadata{}

	allocations := testing.AllocsPerRun(100, func() {
		adapter := C.NewPacketAdapter(pkt, metadata)
		if key := adapter.(interface{ FlowKey() netip.AddrPort }).FlowKey(); key != source {
			t.Fatal("adapter lost the flow key")
		}
	})
	require.EqualValues(t, 1, allocations)
}

func BenchmarkUoTPacketDrop(b *testing.B) {
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	addr := socks5.ParseAddr("8.8.8.8:53")
	writer := new(tsudoku.UoTPacketConn)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pkt := &uotPacket{
			addr:    addr,
			payload: pool.Get(64),
			writer:  writer,
			source:  source,
		}
		pkt.Drop()
	}
}
