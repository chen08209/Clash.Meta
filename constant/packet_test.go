package constant_test

import (
	"net"
	"net/netip"
	"testing"

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

	allocations := testing.AllocsPerRun(100, func() {
		C.NewPacketAdapter(fixedLocalPacket{localAddr: addr}, &C.Metadata{})
	})
	require.LessOrEqual(t, allocations, float64(3))
}
