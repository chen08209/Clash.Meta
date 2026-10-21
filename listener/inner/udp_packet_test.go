package inner

import (
	"testing"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/require"
)

func TestInnerUDPPacketDropRecyclesPayloadAndWrapper(t *testing.T) {
	conn := new(innerUDPPacketConn)

	allocations := testing.AllocsPerRun(100, func() {
		packet := newInnerUDPPacket(conn, pool.Get(64))
		packet.Drop()
	})
	require.Zero(t, allocations)
}

func BenchmarkInnerUDPPacketDrop(b *testing.B) {
	conn := new(innerUDPPacketConn)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		packet := newInnerUDPPacket(conn, pool.Get(64))
		packet.Drop()
	}
}
