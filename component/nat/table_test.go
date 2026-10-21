package nat

import (
	"net"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/maphash"
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
	missing := netip.MustParseAddrPort("192.168.1.3:54322")
	example := netip.MustParseAddrPort("93.184.216.34:443")
	require.False(t, table.AddForLocalConn(missing, example, localConn))
	cond, loaded := table.GetOrCreateLockForLocalConn(missing, example)
	require.Nil(t, cond)
	require.False(t, loaded)

	require.True(t, table.AddForLocalConn(ipv4, example, localConn))
	require.Equal(t, localConn, table.GetForLocalConn(ipv4, example))
	var ranged int
	table.RangeForLocalConnForFlow(ipv4, func(key netip.AddrPort, value *net.UDPConn) bool {
		ranged++
		require.Equal(t, example, key)
		require.Equal(t, localConn, value)
		return false
	})
	require.Equal(t, 1, ranged)

	table.DeleteForFlow(ipv4)
	require.Nil(t, table.GetForLocalConn(ipv4, example))
}

func TestLocalConnLookupStaysAllocationFree(t *testing.T) {
	table := New()
	ipv4 := netip.MustParseAddrPort("192.168.1.2:54321")
	example := netip.MustParseAddrPort("93.184.216.34:443")
	localConn := &net.UDPConn{}

	table.GetOrCreateForFlow(ipv4, func() C.PacketSender { return nil })
	require.True(t, table.AddForLocalConn(ipv4, example, localConn))

	allocations := testing.AllocsPerRun(100, func() {
		if got := table.GetForLocalConn(ipv4, example); got != localConn {
			t.Fatal("local conn lookup missed")
		}
	})
	if maphash.ComparableAllocationFree {
		require.EqualValues(t, 0, allocations)
	} else {
		// The pre-go1.24 hasher port escapes the key on every hash; the
		// lookup probes three tables, so three hash allocations are the
		// floor and the table itself must add nothing.
		require.LessOrEqual(t, int(allocations), 3)
	}
}

func BenchmarkGetForLocalConnHit(b *testing.B) {
	table := New()
	ipv4 := netip.MustParseAddrPort("192.168.1.2:54321")
	example := netip.MustParseAddrPort("93.184.216.34:443")
	localConn := &net.UDPConn{}

	table.GetOrCreateForFlow(ipv4, func() C.PacketSender { return nil })
	table.AddForLocalConn(ipv4, example, localConn)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := table.GetForLocalConn(ipv4, example); got != localConn {
			b.Fatal("local conn lookup missed")
		}
	}
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

func BenchmarkTableCreateUDPFlow(b *testing.B) {
	keys := make([]netip.AddrPort, b.N)
	for i := range keys {
		index := uint32(i)
		keys[i] = netip.AddrPortFrom(
			netip.AddrFrom4([4]byte{10, byte(index >> 24), byte(index >> 16), byte(index)}),
			54321,
		)
	}
	table := New()
	sender := &noopPacketSender{}

	b.ReportAllocs()
	b.ResetTimer()
	for _, key := range keys {
		table.GetOrCreateForFlow(key, func() C.PacketSender { return sender })
	}
}

type noopPacketSender struct{}

func (*noopPacketSender) Send(C.PacketAdapter)                       {}
func (*noopPacketSender) Process(C.PacketConn, C.WriteBackProxy)     {}
func (*noopPacketSender) Close()                                     {}
func (*noopPacketSender) DoSniff(*C.Metadata) error                  { return nil }
func (*noopPacketSender) AddMapping(*C.Metadata, *C.Metadata)        {}
func (*noopPacketSender) RestoreReadFrom(addr netip.Addr) netip.Addr { return addr }
