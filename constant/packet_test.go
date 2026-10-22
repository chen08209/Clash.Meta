package constant_test

import (
	"net"
	"net/netip"
	"testing"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type fixedLocalPacket struct {
	C.UDPPacket
	localAddr net.Addr
}

func (packet fixedLocalPacket) LocalAddr() net.Addr {
	return packet.localAddr
}

func (fixedLocalPacket) Drop() {}

func TestPacketAdapterFlowKey(t *testing.T) {
	tests := []struct {
		name string
		addr net.Addr
		want netip.AddrPort
	}{
		{
			name: "ipv4",
			addr: &net.UDPAddr{IP: net.IPv4(192, 168, 1, 2), Port: 54321},
			want: netip.MustParseAddrPort("192.168.1.2:54321"),
		},
		{
			name: "ipv6",
			addr: &net.UDPAddr{IP: net.ParseIP("2001:db8::1"), Port: 54321},
			want: netip.MustParseAddrPort("[2001:db8::1]:54321"),
		},
		{
			name: "snat wrapped",
			addr: N.NewCustomAddr("TROJAN", "0f4b5a2c", &net.UDPAddr{IP: net.IPv4(192, 168, 1, 2), Port: 54321}),
			want: netip.MustParseAddrPort("192.168.1.2:54321"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := C.NewPacketAdapter(
				fixedLocalPacket{localAddr: test.addr},
				&C.Metadata{},
			)
			flowAdapter, ok := adapter.(interface{ FlowKey() netip.AddrPort })
			require.True(t, ok)
			require.Equal(t, test.want, flowAdapter.FlowKey())
		})
	}
}

func TestNewPacketAdapterDoesNotFormatKey(t *testing.T) {
	addr := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 2), Port: 54321}
	C.NewPacketAdapter(fixedLocalPacket{localAddr: addr}, &C.Metadata{}).Drop()

	allocations := testing.AllocsPerRun(100, func() {
		C.NewPacketAdapter(fixedLocalPacket{localAddr: addr}, &C.Metadata{}).Drop()
	})
	require.LessOrEqual(t, allocations, float64(3))
}

type flowKeyOnlyPacket struct {
	C.UDPPacket
	flowKey netip.AddrPort
}

func (packet flowKeyOnlyPacket) FlowKey() netip.AddrPort { return packet.flowKey }

func (packet flowKeyOnlyPacket) LocalAddr() net.Addr {
	panic("flow key packet materialised its local address")
}

func (*flowKeyOnlyPacket) Drop() {}

type countingDropPacket struct {
	drops int
}

func (*countingDropPacket) Data() []byte { return nil }
func (*countingDropPacket) WriteBack([]byte, netip.AddrPort) (int, error) {
	return 0, nil
}
func (packet *countingDropPacket) Drop() { packet.drops++ }
func (*countingDropPacket) LocalAddr() net.Addr {
	return net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.168.1.2:54321"))
}

func TestNewPacketAdapterPrefersThePacketsFlowKey(t *testing.T) {
	key := netip.MustParseAddrPort("192.168.1.2:54321")

	adapter := C.NewPacketAdapter(&flowKeyOnlyPacket{flowKey: key}, &C.Metadata{})
	flowAdapter, ok := adapter.(interface{ FlowKey() netip.AddrPort })
	require.True(t, ok)
	require.Equal(t, key, flowAdapter.FlowKey())
}

func TestNewPacketAdapterWithFlowKeyAllocatesOnlyTheAdapter(t *testing.T) {
	key := netip.MustParseAddrPort("192.168.1.2:54321")
	packet := &flowKeyOnlyPacket{flowKey: key}
	metadata := &C.Metadata{}
	C.NewPacketAdapter(packet, metadata).Drop()

	allocations := testing.AllocsPerRun(100, func() {
		C.NewPacketAdapter(packet, metadata).Drop()
	})
	require.EqualValues(t, 1, allocations)
}

func TestPacketAdapterDropIsSingleUse(t *testing.T) {
	packet := &countingDropPacket{}
	metadata := &C.Metadata{}
	adapter := C.NewPacketAdapter(packet, metadata)
	require.Equal(t, metadata, adapter.Metadata())
	require.Equal(t, "192.168.1.2:54321", adapter.Key())

	adapter.Drop()
	adapter.Drop()
	// The adapter stays whole: a flow keeps it in its write-back proxy past Drop.
	require.Equal(t, metadata, adapter.Metadata())
	require.Equal(t, 1, packet.drops)
}

func BenchmarkPacketAdapterLifecycle(b *testing.B) {
	packet := &flowKeyOnlyPacket{flowKey: netip.MustParseAddrPort("192.168.1.2:54321")}
	metadata := &C.Metadata{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		C.NewPacketAdapter(packet, metadata).Drop()
	}
}
