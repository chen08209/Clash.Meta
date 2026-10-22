package sing

import (
	"net"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/sing/common/buf"
	M "github.com/metacubex/sing/common/metadata"
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

func TestNewPacketAdapterOnSingPacketSkipsTheUDPAddr(t *testing.T) {
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
	require.Zero(t, allocations)
}

func TestPacketWriterStateClosesOnce(t *testing.T) {
	state := newSingPacketState(fakePacketWriter{})

	n, err := state.writeBack([]byte("ping"), netip.MustParseAddrPort("127.0.0.1:53"))
	require.NoError(t, err)
	require.Equal(t, 4, n)

	state.close()
	_, err = state.writeBack([]byte("ping"), netip.MustParseAddrPort("127.0.0.1:53"))
	require.EqualError(t, err, "writeBack to closed connection")
}

type fakePacketWriter struct{}

func (fakePacketWriter) WritePacket(_ *buf.Buffer, _ M.Socksaddr) error {
	return nil
}

func (fakePacketWriter) WriteTo(p []byte, _ net.Addr) (int, error) {
	return len(p), nil
}
