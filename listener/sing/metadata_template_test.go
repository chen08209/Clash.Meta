package sing

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"

	"github.com/metacubex/sing/common/buf"
	M "github.com/metacubex/sing/common/metadata"
	"github.com/stretchr/testify/require"
)

type capturedUDPPacket struct {
	metadata *C.Metadata
	data     []byte
}

type captureTunnel struct {
	C.Tunnel
	packets []capturedUDPPacket
}

func (c *captureTunnel) HandleUDPPacket(packet C.UDPPacket, metadata *C.Metadata) {
	c.packets = append(c.packets, capturedUDPPacket{
		metadata: metadata,
		data:     append([]byte(nil), packet.Data()...),
	})
}

type dropTunnel struct {
	C.Tunnel
}

func (dropTunnel) HandleUDPPacket(packet C.UDPPacket, _ *C.Metadata) {
	packet.Drop()
}

type recycleTunnel struct {
	C.Tunnel
	wrappers []*packet
	data     [][]byte
}

func (c *recycleTunnel) HandleUDPPacket(udpPacket C.UDPPacket, _ *C.Metadata) {
	if singPacket, ok := udpPacket.(*packet); ok {
		c.wrappers = append(c.wrappers, singPacket)
	}
	c.data = append(c.data, append([]byte(nil), udpPacket.Data()...))
	udpPacket.Drop()
}

type fakeDatagram struct {
	data []byte
	dest M.Socksaddr
}

type fakePacketConn struct {
	datagrams []fakeDatagram
	pos       int
}

func (f *fakePacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	if f.pos >= len(f.datagrams) {
		return M.Socksaddr{}, io.EOF
	}
	datagram := f.datagrams[f.pos]
	f.pos++
	_, _ = buffer.Write(datagram.data)
	return datagram.dest, nil
}

func (f *fakePacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	return nil
}

func (f *fakePacketConn) Close() error                     { return nil }
func (f *fakePacketConn) LocalAddr() net.Addr              { return nil }
func (f *fakePacketConn) SetDeadline(time.Time) error      { return nil }
func (f *fakePacketConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakePacketConn) SetWriteDeadline(time.Time) error { return nil }

func TestNewPacketConnectionReusesPerFlowMetadata(t *testing.T) {
	destA := M.SocksaddrFrom(netip.MustParseAddr("1.2.3.4"), 443)
	destB := M.SocksaddrFrom(netip.MustParseAddr("5.6.7.8"), 8443)
	source := M.SocksaddrFrom(netip.MustParseAddr("192.168.1.2"), 54321)
	payload := []byte("payload")

	tunnel := &captureTunnel{}
	conn := &fakePacketConn{datagrams: []fakeDatagram{
		{data: payload, dest: destA},
		{data: payload, dest: destA},
		{data: payload, dest: destA},
		{data: payload, dest: destB},
		{data: payload, dest: destA},
	}}
	handler := &ListenerHandler{ListenerConfig: ListenerConfig{Tunnel: tunnel, Type: C.TUN}}

	err := handler.NewPacketConnection(context.Background(), conn, M.Metadata{Source: source, Destination: destA})
	require.True(t, err == nil || errors.Is(err, io.EOF), "unexpected error: %v", err)

	require.Len(t, tunnel.packets, 5)
	// Repeating the destination shares one metadata object per flow.
	require.Same(t, tunnel.packets[0].metadata, tunnel.packets[1].metadata)
	require.Same(t, tunnel.packets[0].metadata, tunnel.packets[2].metadata)
	// A changed destination builds its own metadata.
	require.NotSame(t, tunnel.packets[0].metadata, tunnel.packets[3].metadata)
	// And the template follows the latest destination.
	require.NotSame(t, tunnel.packets[3].metadata, tunnel.packets[4].metadata)
	require.NotSame(t, tunnel.packets[0].metadata, tunnel.packets[4].metadata)

	require.Equal(t, "1.2.3.4", tunnel.packets[0].metadata.DstIP.String())
	require.EqualValues(t, 443, tunnel.packets[0].metadata.DstPort)
	require.Equal(t, "5.6.7.8", tunnel.packets[3].metadata.DstIP.String())
	require.EqualValues(t, 8443, tunnel.packets[3].metadata.DstPort)
	for _, captured := range tunnel.packets {
		require.Equal(t, payload, captured.data)
	}
}

// One run forwards a fixed batch through one flow and drops each packet, so
// the reported allocs are per batch with the same ownership tunnel uses.
func BenchmarkNewPacketConnectionDatagramBatch(b *testing.B) {
	dest := M.SocksaddrFrom(netip.MustParseAddr("1.2.3.4"), 443)
	source := M.SocksaddrFrom(netip.MustParseAddr("192.168.1.2"), 54321)
	payload := make([]byte, 512)
	tunnel := dropTunnel{}
	handler := &ListenerHandler{ListenerConfig: ListenerConfig{Tunnel: tunnel, Type: C.TUN}}
	const datagramsPerRun = 100
	datagrams := make([]fakeDatagram, datagramsPerRun)
	for index := range datagrams {
		datagrams[index] = fakeDatagram{data: payload, dest: dest}
	}
	metadata := M.Metadata{Source: source, Destination: dest}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn := &fakePacketConn{datagrams: datagrams}
		_ = handler.NewPacketConnection(context.Background(), conn, metadata)
	}
}

func TestNewPacketConnectionReleasesPacketBuffersAndKeepsWrappersApart(t *testing.T) {
	dest := M.SocksaddrFrom(netip.MustParseAddr("1.2.3.4"), 443)
	source := M.SocksaddrFrom(netip.MustParseAddr("192.168.1.2"), 54321)
	tunnel := &recycleTunnel{}
	conn := &fakePacketConn{datagrams: []fakeDatagram{
		{data: []byte("first"), dest: dest},
		{data: []byte("second"), dest: dest},
		{data: []byte("third"), dest: dest},
	}}
	handler := &ListenerHandler{ListenerConfig: ListenerConfig{Tunnel: tunnel, Type: C.TUN}}

	err := handler.NewPacketConnection(context.Background(), conn, M.Metadata{Source: source, Destination: dest})
	require.True(t, err == nil || errors.Is(err, io.EOF), "unexpected error: %v", err)

	require.Len(t, tunnel.wrappers, 3)
	require.Equal(t, []byte("first"), tunnel.data[0])
	require.Equal(t, []byte("second"), tunnel.data[1])
	require.Equal(t, []byte("third"), tunnel.data[2])
	// Each datagram gets its own wrapper: a flow keeps the packet it last sent
	// so it can answer replies through it, and a recycled wrapper would move
	// those replies onto whichever datagram claimed it.
	require.NotSame(t, tunnel.wrappers[0], tunnel.wrappers[1])
	require.NotSame(t, tunnel.wrappers[0], tunnel.wrappers[2])
	for _, wrapper := range tunnel.wrappers {
		require.Nil(t, wrapper.buff)
	}
}
