package udp

import (
	"sync"
	"testing"
)

func BenchmarkUDPPacketPool(b *testing.B) {
	arrayPool := sync.Pool{
		New: func() interface{} { return new([udpBufferSize]byte) },
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := arrayPool.Get().(*[udpBufferSize]byte)
		arrayPool.Put(buf)
	}
}
