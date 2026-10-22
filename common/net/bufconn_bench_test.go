package net

import (
	"net"
	"testing"

	singbufio "github.com/metacubex/sing/common/bufio"
)

var bufferedConnWriterSink ExtendedWriter

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

func BenchmarkBufferedConnExtendedWriter(b *testing.B) {
	conn, remote := net.Pipe()
	buffered := NewBufferedConn(conn)
	b.Cleanup(func() {
		_ = conn.Close()
		_ = remote.Close()
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bufferedConnWriterSink = singbufio.NewExtendedWriter(UnwrapWriter(buffered))
	}
}
