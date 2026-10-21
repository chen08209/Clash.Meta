package socks

import (
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/lru"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type socksCaptureTunnel struct {
	C.Tunnel
	metadatas []*C.Metadata
}

func (c *socksCaptureTunnel) HandleUDPPacket(packet C.UDPPacket, metadata *C.Metadata) {
	c.metadatas = append(c.metadatas, metadata)
}

type socksDropTunnel struct {
	C.Tunnel
}

func (socksDropTunnel) HandleUDPPacket(C.UDPPacket, *C.Metadata) {}

type socksFakePacketConn struct{}

func (socksFakePacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	return 0, nil, nil
}

func (socksFakePacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	return len(p), nil
}

func (socksFakePacketConn) Close() error        { return nil }
func (socksFakePacketConn) LocalAddr() net.Addr { return nil }
func (socksFakePacketConn) SetDeadline(time.Time) error {
	return nil
}

func (socksFakePacketConn) SetReadDeadline(time.Time) error {
	return nil
}

func (socksFakePacketConn) SetWriteDeadline(time.Time) error {
	return nil
}

func newSocksTestListener() *UDPListener {
	return &UDPListener{
		packetConn: socksFakePacketConn{},
		flowCache: lru.New(
			lru.WithSize[netip.AddrPort, *socksUDPFlow](socksUDPFlowCacheSize),
			lru.WithAge[netip.AddrPort, *socksUDPFlow](socksUDPFlowCacheAge),
		),
	}
}

func newSocksDatagram(ip netip.Addr, port uint16, payload string) []byte {
	dgram := []byte{0, 0, 0, 1}
	dgram = append(dgram, ip.AsSlice()...)
	var portBytes [2]byte
	binary.BigEndian.PutUint16(portBytes[:], port)
	dgram = append(dgram, portBytes[:]...)
	return append(dgram, payload...)
}

func TestSocksUDPFlowCacheReusesMetadata(t *testing.T) {
	tunnel := &socksCaptureTunnel{}
	listener := newSocksTestListener()
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	dgram := newSocksDatagram(netip.MustParseAddr("1.2.3.4"), 443, "payload")

	listener.handleSocksUDP(tunnel, dgram, source)
	listener.handleSocksUDP(tunnel, dgram, source)
	listener.handleSocksUDP(tunnel, dgram, source)

	require.Len(t, tunnel.metadatas, 3)
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[2])
	require.Equal(t, "1.2.3.4", tunnel.metadatas[0].DstIP.String())
	require.EqualValues(t, 443, tunnel.metadatas[0].DstPort)
	require.EqualValues(t, 54321, tunnel.metadatas[0].SrcPort)
}

func TestSocksUDPFlowCacheSplitsTargets(t *testing.T) {
	tunnel := &socksCaptureTunnel{}
	listener := newSocksTestListener()
	source := netip.MustParseAddrPort("192.168.1.2:54321")

	listener.handleSocksUDP(tunnel, newSocksDatagram(netip.MustParseAddr("1.2.3.4"), 443, "payload"), source)
	listener.handleSocksUDP(tunnel, newSocksDatagram(netip.MustParseAddr("5.6.7.8"), 8443, "payload"), source)

	require.Len(t, tunnel.metadatas, 2)
	require.NotSame(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Equal(t, "5.6.7.8", tunnel.metadatas[1].DstIP.String())
	require.EqualValues(t, 8443, tunnel.metadatas[1].DstPort)
}

// One run forwards one datagram through a warm flow, so the reported allocs
// are the steady-state cost: the packet carrier only, no metadata rebuild.
func BenchmarkSocksUDPHandleCached(b *testing.B) {
	tunnel := socksDropTunnel{}
	listener := newSocksTestListener()
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	dgram := newSocksDatagram(netip.MustParseAddr("1.2.3.4"), 443, "payload")
	listener.handleSocksUDP(tunnel, dgram, source)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		listener.handleSocksUDP(tunnel, dgram, source)
	}
}
