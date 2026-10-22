//go:build !race

package sing_vless

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 0.5 leaves room for the one buffer a sync.Pool refills after a GC while still
// failing the per-call allocation this replaced.
func TestServerPacketConnWriteToAllocatesNothing(t *testing.T) {
	pc := &serverPacketConn{ExtendedConn: &fakeExtendedConn{}}
	payload := []byte("ping")

	allocations := testing.AllocsPerRun(1000, func() {
		if _, err := pc.WriteTo(payload, nil); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkServerPacketConnWriteTo(b *testing.B) {
	pc := &serverPacketConn{ExtendedConn: &fakeExtendedConn{}}
	payload := make([]byte, 1350)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := pc.WriteTo(payload, nil); err != nil {
			b.Fatal(err)
		}
	}
}
