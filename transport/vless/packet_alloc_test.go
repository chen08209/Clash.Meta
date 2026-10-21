//go:build !race

package vless

import (
	"bytes"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

// 0.5 leaves room for the one buffer a sync.Pool refills after a GC while still
// failing the per-call allocation this replaced.
func TestPacketConnWriteToAllocatesNothing(t *testing.T) {
	pc := &PacketConn{Conn: &fakeConn{}, rAddr: netip.MustParseAddrPort("203.0.113.7:443")}
	payload := []byte("ping")

	allocations := testing.AllocsPerRun(1000, func() {
		if _, err := pc.WriteTo(payload, nil); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkPacketConnWriteTo(b *testing.B) {
	pc := &PacketConn{Conn: &fakeConn{}, rAddr: netip.MustParseAddrPort("203.0.113.7:443")}
	payload := make([]byte, 1350)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := pc.WriteTo(payload, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPacketConnReadFrom(b *testing.B) {
	raw := append([]byte{0x05, 0x46}, make([]byte, 1350)...)
	reader := bytes.NewReader(raw)
	pc := &PacketConn{Conn: &fakeConn{r: reader}, rAddr: netip.MustParseAddrPort("203.0.113.7:443")}
	buf := make([]byte, 2048)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.Reset(raw)
		if _, _, err := pc.ReadFrom(buf); err != nil {
			b.Fatal(err)
		}
	}
}
