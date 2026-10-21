package udp

import (
	"sync"
	"testing"
)

func BenchmarkUDPPacketPool(b *testing.B) {
	bytePool := sync.Pool{
		New: func() interface{} { return make([]byte, udpBufferSize) },
	}
	arrayPool := sync.Pool{
		New: func() interface{} { return new([udpBufferSize]byte) },
	}

	b.Run("byte-slice", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := bytePool.Get().([]byte)
			bytePool.Put(buf)
		}
	})

	b.Run("array-pointer", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := arrayPool.Get().(*[udpBufferSize]byte)
			arrayPool.Put(buf)
		}
	})
}
