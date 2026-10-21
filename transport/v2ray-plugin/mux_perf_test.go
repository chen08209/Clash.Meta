//go:build !race

package obfs

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

type discardConn struct{ net.Conn }

func (discardConn) Write(b []byte) (int, error) { return len(b), nil }

func TestMuxWriteAllocatesNothing(t *testing.T) {
	mux := &Mux{Conn: discardConn{}, id: [2]byte{1, 2}}
	payload := make([]byte, 1024)

	allocations := testing.AllocsPerRun(1000, func() {
		if _, err := mux.Write(payload); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

func BenchmarkMuxWrite(b *testing.B) {
	mux := &Mux{Conn: discardConn{}, id: [2]byte{1, 2}}
	payload := make([]byte, 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := mux.Write(payload); err != nil {
			b.Fatal(err)
		}
	}
}
