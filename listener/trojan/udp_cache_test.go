package trojan

import (
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type trojanCaptureTunnel struct {
	C.Tunnel
	metadatas []*C.Metadata
	wrappers  []*packet
}

func (c *trojanCaptureTunnel) HandleUDPPacket(udpPacket C.UDPPacket, metadata *C.Metadata) {
	c.metadatas = append(c.metadatas, metadata)
	if pkt, ok := udpPacket.(*packet); ok {
		c.wrappers = append(c.wrappers, pkt)
	}
	udpPacket.Drop()
}

type trojanDropTunnel struct {
	C.Tunnel
}

func (trojanDropTunnel) HandleUDPPacket(udpPacket C.UDPPacket, _ *C.Metadata) {
	udpPacket.Drop()
}

func TestTrojanUDPFlowCacheReusesMetadataAndWrappers(t *testing.T) {
	tunnel := &trojanCaptureTunnel{}
	session := newTrojanUDPSession(
		&fixedLocalPacketConn{},
		netip.MustParseAddrPort("192.168.1.2:54321"),
		tunnel,
		nil,
	)
	target := netip.MustParseAddrPort("1.2.3.4:53")

	session.handle(pool.Get(8), target)
	session.handle(pool.Get(8), target)
	session.handle(pool.Get(8), target)

	require.Len(t, tunnel.metadatas, 3)
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[2])
	require.Equal(t, "1.2.3.4", tunnel.metadatas[0].DstIP.String())
	require.EqualValues(t, 53, tunnel.metadatas[0].DstPort)
	require.EqualValues(t, 54321, tunnel.metadatas[0].SrcPort)

	for _, wrapper := range tunnel.wrappers {
		require.Nil(t, wrapper.data)
		require.Nil(t, wrapper.payload)
	}
}

func TestTrojanUDPFlowCacheSplitsTargets(t *testing.T) {
	tunnel := &trojanCaptureTunnel{}
	session := newTrojanUDPSession(
		&fixedLocalPacketConn{},
		netip.MustParseAddrPort("192.168.1.2:54321"),
		tunnel,
		nil,
	)

	session.handle(pool.Get(8), netip.MustParseAddrPort("1.2.3.4:53"))
	session.handle(pool.Get(8), netip.MustParseAddrPort("5.6.7.8:443"))

	require.Len(t, tunnel.metadatas, 2)
	require.NotSame(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Equal(t, "1.2.3.4", tunnel.metadatas[0].DstIP.String())
	require.Equal(t, "5.6.7.8", tunnel.metadatas[1].DstIP.String())
}

func BenchmarkTrojanUDPHandle(b *testing.B) {
	session := newTrojanUDPSession(
		&fixedLocalPacketConn{},
		netip.MustParseAddrPort("192.168.1.2:54321"),
		trojanDropTunnel{},
		nil,
	)
	target := netip.MustParseAddrPort("1.2.3.4:53")
	data := pool.Get(64)
	session.handle(data, target)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		session.handle(data, target)
	}
}

func BenchmarkTrojanPacketWriteBack(b *testing.B) {
	pkt := &packet{pc: &fixedLocalPacketConn{}}
	addr := netip.MustParseAddrPort("1.2.3.4:53")
	payload := []byte("payload")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pkt.WriteBack(payload, addr)
	}
}
