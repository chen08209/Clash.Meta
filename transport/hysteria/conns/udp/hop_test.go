package udp

import (
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUDPPacketPoolRecyclesWrappers(t *testing.T) {
	conn := &ObfsUDPHopClientPacketConn{
		recvQueue: make(chan *udpPacket, 1),
		closeChan: make(chan struct{}),
		bufPool: sync.Pool{
			New: func() any { return new([udpBufferSize]byte) },
		},
	}
	addr := &net.UDPAddr{IP: net.IPv4(192, 168, 1, 2), Port: 443}
	buf := new([udpBufferSize]byte)
	copy(buf[:], []byte("hello"))
	packet := newUDPPacket(buf, len("hello"), addr)
	conn.recvQueue <- packet

	out := make([]byte, 8)
	n, gotAddr, err := conn.ReadFrom(out)
	require.NoError(t, err)
	require.Equal(t, len("hello"), n)
	require.Equal(t, "hello", string(out[:n]))
	require.Equal(t, conn.serverAddr, gotAddr)
	require.Nil(t, packet.buf)
	require.Zero(t, packet.n)
	require.Nil(t, packet.addr)

	if !raceEnabled {
		reused := newUDPPacket(buf, 1, addr)
		require.Same(t, packet, reused)
		releaseUDPPacket(reused, &conn.bufPool)
	}
}

func BenchmarkUDPPacketLifecycle(b *testing.B) {
	bufPool := sync.Pool{
		New: func() any { return new([udpBufferSize]byte) },
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packet := newUDPPacket(bufPool.Get().(*[udpBufferSize]byte), 1, nil)
		udpPacketSink = packet
		releaseUDPPacket(packet, &bufPool)
	}
}

func BenchmarkUDPPacketOwnedLifecycle(b *testing.B) {
	bufPool := sync.Pool{
		New: func() any { return new([udpBufferSize]byte) },
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packet := &udpPacket{
			buf: bufPool.Get().(*[udpBufferSize]byte),
			n:   1,
		}
		udpPacketSink = packet
		bufPool.Put(packet.buf)
	}
}

var udpPacketSink *udpPacket
