package net

import (
	"net"
	"testing"
)

func BenchmarkNewBufferedConn(b *testing.B) {
	conn, remote := net.Pipe()
	b.Cleanup(func() {
		_ = conn.Close()
		_ = remote.Close()
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewBufferedConn(conn)
	}
}
