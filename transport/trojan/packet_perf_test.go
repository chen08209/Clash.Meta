//go:build !race

package trojan

import (
	"io"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/transport/socks5"
	"github.com/stretchr/testify/require"
)

// 0.5 leaves room for the buffer pool refilling after a GC while still failing
// the per-call allocation this replaced.
func TestWritePacketAllocatesNothing(t *testing.T) {
	addr := socks5.AddrFromStdAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	payload := make([]byte, 1350)

	allocations := testing.AllocsPerRun(1000, func() {
		if _, err := writePacket(io.Discard, addr, payload); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkWritePacket(b *testing.B) {
	addr := socks5.AddrFromStdAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	payload := make([]byte, 1350)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := writePacket(io.Discard, addr, payload); err != nil {
			b.Fatal(err)
		}
	}
}
