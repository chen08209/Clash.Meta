package nat

import (
	"net"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func TestTableUsesBinaryFlowKeys(t *testing.T) {
	table := New()
	ipv4 := netip.MustParseAddrPort("192.168.1.2:54321")
	ipv6 := netip.MustParseAddrPort("[2001:db8::1]:54321")

	first, loaded := table.GetOrCreateForFlow(ipv4, func() C.PacketSender {
		return nil
	})
	require.Nil(t, first)
	require.False(t, loaded)

	second, loaded := table.GetOrCreateForFlow(ipv4, func() C.PacketSender {
		t.Fatal("existing IPv4 flow recreated its sender")
		return nil
	})
	require.Nil(t, second)
	require.True(t, loaded)

	other, loaded := table.GetOrCreateForFlow(ipv6, func() C.PacketSender {
		return nil
	})
	require.Nil(t, other)
	require.False(t, loaded)

	localConn := &net.UDPConn{}
	require.True(t, table.AddForLocalConn(ipv4.String(), "example.com:443", localConn))
	require.Equal(t, localConn, table.GetForLocalConn(ipv4.String(), "example.com:443"))
	var ranged int
	table.RangeForLocalConnForFlow(ipv4, func(key string, value *net.UDPConn) bool {
		ranged++
		require.Equal(t, "example.com:443", key)
		require.Equal(t, localConn, value)
		return false
	})
	require.Equal(t, 1, ranged)

	table.DeleteForFlow(ipv4)
	require.Nil(t, table.GetForLocalConn(ipv4.String(), "example.com:443"))
}

func BenchmarkTableGetOrCreateForFlowHit(b *testing.B) {
	table := New()
	key := netip.MustParseAddrPort("192.168.1.2:54321")
	sender := &noopPacketSender{}
	table.GetOrCreateForFlow(key, func() C.PacketSender {
		return sender
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, loaded := table.GetOrCreateForFlow(key, nil); !loaded {
			b.Fatal("existing flow was not loaded")
		}
	}
}

type noopPacketSender struct{}

func (*noopPacketSender) Send(C.PacketAdapter)                       {}
func (*noopPacketSender) Process(C.PacketConn, C.WriteBackProxy)     {}
func (*noopPacketSender) Close()                                     {}
func (*noopPacketSender) DoSniff(*C.Metadata) error                  { return nil }
func (*noopPacketSender) AddMapping(*C.Metadata, *C.Metadata)        {}
func (*noopPacketSender) RestoreReadFrom(addr netip.Addr) netip.Addr { return addr }
