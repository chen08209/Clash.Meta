//go:build !race

package gost

import (
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

// 0.5 leaves room for the one buffer a sync.Pool refills after a GC while still
// failing the per-call allocation this replaced.
func TestRelayPacketConnWriteToAllocatesNothing(t *testing.T) {
	raddr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	conn := &relayPacketConn{conn: fakeRelayConn{}, raddr: raddr}
	payload := []byte("ping")

	allocations := testing.AllocsPerRun(1000, func() {
		if _, err := conn.WriteTo(payload, raddr); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkRelayPacketConnWriteTo(b *testing.B) {
	raddr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	conn := &relayPacketConn{conn: fakeRelayConn{}, raddr: raddr}
	payload := make([]byte, 1350)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := conn.WriteTo(payload, raddr); err != nil {
			b.Fatal(err)
		}
	}
}
