package snell

import (
	"net"
	"net/netip"
	"sync"
	"testing"

	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/snell"
	"github.com/stretchr/testify/require"
)

type snellFixedAddrConn struct {
	net.Conn
}

func (snellFixedAddrConn) LocalAddr() net.Addr {
	return net.TCPAddrFromAddrPort(netip.MustParseAddrPort("127.0.0.1:8080"))
}

func (snellFixedAddrConn) RemoteAddr() net.Addr {
	return net.TCPAddrFromAddrPort(netip.MustParseAddrPort("192.168.1.2:54321"))
}

type snellCaptureTunnel struct {
	C.Tunnel
	metadatas []*C.Metadata
	wrappers  []*udpPacket
}

func (c *snellCaptureTunnel) HandleUDPPacket(packet C.UDPPacket, metadata *C.Metadata) {
	c.metadatas = append(c.metadatas, metadata)
	if wrapper, ok := packet.(*udpPacket); ok {
		c.wrappers = append(c.wrappers, wrapper)
	}
	packet.Drop()
}

type snellDropTunnel struct {
	C.Tunnel
}

func (snellDropTunnel) HandleUDPPacket(packet C.UDPPacket, _ *C.Metadata) {
	packet.Drop()
}

func newSnellTestSession(tb testing.TB, tunnel C.Tunnel) *snellUDPSession {
	tb.Helper()
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	stream := snell.ServerStreamConn(snellFixedAddrConn{}, []byte("benchmark-psk"), snell.Version4)
	return &snellUDPSession{
		listener:  &Listener{},
		stream:    stream,
		source:    source,
		localAddr: N.NewCustomAddr(C.SNELL.String(), "test", stream.RemoteAddr()),
		writeMu:   &sync.Mutex{},
		tunnel:    tunnel,
	}
}

func TestSnellUDPFlowCacheReusesMetadataAndWrappers(t *testing.T) {
	tunnel := &snellCaptureTunnel{}
	session := newSnellTestSession(t, tunnel)
	session.clientID = "udp-client"
	request := snell.UDPRequest{
		Ip:      netip.MustParseAddr("1.2.3.4"),
		Port:    53,
		Payload: []byte("hello"),
	}

	for i := 0; i < 3; i++ {
		session.handle(request.Payload, request)
	}

	require.Len(t, tunnel.metadatas, 3)
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Same(t, tunnel.metadatas[0], tunnel.metadatas[2])
	require.Equal(t, "1.2.3.4", tunnel.metadatas[0].DstIP.String())
	require.EqualValues(t, 53, tunnel.metadatas[0].DstPort)
	require.EqualValues(t, 54321, tunnel.metadatas[0].SrcPort)
	require.Equal(t, "udp-client", tunnel.metadatas[0].InUser)

	for _, wrapper := range tunnel.wrappers {
		require.Nil(t, wrapper.data)
		require.Nil(t, wrapper.conn)
		require.Nil(t, wrapper.writeMu)
	}
}

func TestSnellUDPFlowCacheSplitsTargets(t *testing.T) {
	tunnel := &snellCaptureTunnel{}
	session := newSnellTestSession(t, tunnel)
	first := snell.UDPRequest{Ip: netip.MustParseAddr("1.2.3.4"), Port: 53, Payload: []byte("a")}
	second := snell.UDPRequest{Ip: netip.MustParseAddr("5.6.7.8"), Port: 443, Payload: []byte("b")}

	session.handle(first.Payload, first)
	session.handle(second.Payload, second)

	require.Len(t, tunnel.metadatas, 2)
	require.NotSame(t, tunnel.metadatas[0], tunnel.metadatas[1])
	require.Equal(t, "1.2.3.4", tunnel.metadatas[0].DstIP.String())
	require.Equal(t, "5.6.7.8", tunnel.metadatas[1].DstIP.String())
}

func TestSnellPacketFlowKeyReturnsTheSource(t *testing.T) {
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	pkt := &udpPacket{source: source}

	flowPacket, ok := any(pkt).(interface{ FlowKey() netip.AddrPort })
	require.True(t, ok)
	require.Equal(t, source, flowPacket.FlowKey())
}

func TestSnellPacketDropIsAllocationFree(t *testing.T) {
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	newSnellPacket(nil, nil, nil, source, pool.Get(64)).Drop()

	allocations := testing.AllocsPerRun(100, func() {
		newSnellPacket(nil, nil, nil, source, pool.Get(64)).Drop()
	})
	require.Zero(t, allocations)
}

func TestSnellUDPHandleIsAllocationFree(t *testing.T) {
	session := newSnellTestSession(t, snellDropTunnel{})
	request := snell.UDPRequest{
		Ip:      netip.MustParseAddr("1.2.3.4"),
		Port:    53,
		Payload: []byte("hello"),
	}
	session.handle(request.Payload, request)

	allocations := testing.AllocsPerRun(100, func() {
		session.handle(request.Payload, request)
	})
	require.Zero(t, allocations)
}

func BenchmarkSnellUDPHandle(b *testing.B) {
	session := newSnellTestSession(b, snellDropTunnel{})
	request := snell.UDPRequest{
		Ip:      netip.MustParseAddr("1.2.3.4"),
		Port:    53,
		Payload: []byte("hello"),
	}
	session.handle(request.Payload, request)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		session.handle(request.Payload, request)
	}
}
