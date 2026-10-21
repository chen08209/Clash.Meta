package shadowsocks

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

type ssCaptureTunnel struct {
	C.Tunnel
	metadatas []*C.Metadata
}

func (c *ssCaptureTunnel) HandleUDPPacket(udpPacket C.UDPPacket, metadata *C.Metadata) {
	c.metadatas = append(c.metadatas, metadata)
	udpPacket.Drop()
}

type ssDropTunnel struct {
	C.Tunnel
}

func (ssDropTunnel) HandleUDPPacket(udpPacket C.UDPPacket, _ *C.Metadata) {
	udpPacket.Drop()
}

type ssRecycleTunnel struct {
	C.Tunnel
	wrappers []*packet
	payloads [][]byte
}

func (c *ssRecycleTunnel) HandleUDPPacket(udpPacket C.UDPPacket, _ *C.Metadata) {
	if ssPacket, ok := udpPacket.(*packet); ok {
		c.wrappers = append(c.wrappers, ssPacket)
	}
	c.payloads = append(c.payloads, append([]byte(nil), udpPacket.Data()...))
	udpPacket.Drop()
}

type ssFakePacketConn struct{}

func (ssFakePacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	return 0, nil, nil
}

func (ssFakePacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	return len(p), nil
}

func (ssFakePacketConn) Close() error        { return nil }
func (ssFakePacketConn) LocalAddr() net.Addr { return nil }
func (ssFakePacketConn) SetDeadline(time.Time) error {
	return nil
}

func (ssFakePacketConn) SetReadDeadline(time.Time) error {
	return nil
}

func (ssFakePacketConn) SetWriteDeadline(time.Time) error {
	return nil
}

func newShadowSocksTestListener() *UDPListener {
	return &UDPListener{
		packetConn: ssFakePacketConn{},
		flowCache: lru.New(
			lru.WithSize[netip.AddrPort, *ssUDPFlow](ssUDPFlowCacheSize),
			lru.WithAge[netip.AddrPort, *ssUDPFlow](ssUDPFlowCacheAge),
		),
	}
}

func newShadowSocksDatagram(ip netip.Addr, port uint16, payload string) []byte {
	dgram := []byte{1} // ATYP, then the address, the port and the payload
	dgram = append(dgram, ip.AsSlice()...)
	var portBytes [2]byte
	binary.BigEndian.PutUint16(portBytes[:], port)
	dgram = append(dgram, portBytes[:]...)
	return append(dgram, payload...)
}

func TestShadowSocksUDPFlowCacheReusesMetadata(t *testing.T) {
	tunnel := &ssCaptureTunnel{}
	listener := newShadowSocksTestListener()
	conn := ssFakePacketConn{}
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	dgram := newShadowSocksDatagram(netip.MustParseAddr("1.2.3.4"), 443, "payload")

	listener.handleSocksUDP(conn, tunnel, dgram, source)
	listener.handleSocksUDP(conn, tunnel, dgram, source)
	listener.handleSocksUDP(conn, tunnel, dgram, source)

	require.Len(t, tunnel.metadatas, 3)
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[2])
	require.Equal(t, "1.2.3.4", tunnel.metadatas[0].DstIP.String())
	require.EqualValues(t, 443, tunnel.metadatas[0].DstPort)
	require.EqualValues(t, 54321, tunnel.metadatas[0].SrcPort)
}

func TestShadowSocksUDPFlowCacheSplitsTargets(t *testing.T) {
	tunnel := &ssCaptureTunnel{}
	listener := newShadowSocksTestListener()
	conn := ssFakePacketConn{}
	source := netip.MustParseAddrPort("192.168.1.2:54321")

	listener.handleSocksUDP(conn, tunnel, newShadowSocksDatagram(netip.MustParseAddr("1.2.3.4"), 443, "payload"), source)
	listener.handleSocksUDP(conn, tunnel, newShadowSocksDatagram(netip.MustParseAddr("5.6.7.8"), 8443, "payload"), source)

	require.Len(t, tunnel.metadatas, 2)
	require.NotSame(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Equal(t, "5.6.7.8", tunnel.metadatas[1].DstIP.String())
	require.EqualValues(t, 8443, tunnel.metadatas[1].DstPort)
}

func TestShadowSocksUDPPacketWrappersAreRecycled(t *testing.T) {
	tunnel := &ssRecycleTunnel{}
	listener := newShadowSocksTestListener()
	conn := ssFakePacketConn{}
	source := netip.MustParseAddrPort("192.168.1.2:54321")

	listener.handleSocksUDP(conn, tunnel, newShadowSocksDatagram(netip.MustParseAddr("1.2.3.4"), 443, "first"), source)
	listener.handleSocksUDP(conn, tunnel, newShadowSocksDatagram(netip.MustParseAddr("1.2.3.4"), 443, "second"), source)
	listener.handleSocksUDP(conn, tunnel, newShadowSocksDatagram(netip.MustParseAddr("1.2.3.4"), 443, "third"), source)

	require.Len(t, tunnel.wrappers, 3)
	require.Equal(t, []byte("first"), tunnel.payloads[0])
	require.Equal(t, []byte("second"), tunnel.payloads[1])
	require.Equal(t, []byte("third"), tunnel.payloads[2])
	for _, wrapper := range tunnel.wrappers {
		require.Nil(t, wrapper.data)
		require.Nil(t, wrapper.payload)
	}
}

func TestShadowSocksUDPDropsUnresolvedDatagram(t *testing.T) {
	tunnel := &ssCaptureTunnel{}
	listener := newShadowSocksTestListener()
	conn := ssFakePacketConn{}
	source := netip.MustParseAddrPort("192.168.1.2:54321")

	// An unknown address type makes the header unresolvable, so the datagram
	// never reaches the tunnel.
	listener.handleSocksUDP(conn, tunnel, []byte{0x05, 0x01, 0x02}, source)

	require.Empty(t, tunnel.metadatas)
}

// One run forwards one datagram through a warm flow and drops it, so the
// reported allocs are the steady-state cost with tunnel ownership.
func BenchmarkShadowSocksUDPHandleCached(b *testing.B) {
	tunnel := ssDropTunnel{}
	listener := newShadowSocksTestListener()
	conn := ssFakePacketConn{}
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	dgram := newShadowSocksDatagram(netip.MustParseAddr("1.2.3.4"), 443, "payload")
	listener.handleSocksUDP(conn, tunnel, dgram, source)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		listener.handleSocksUDP(conn, tunnel, dgram, source)
	}
}
