package outbound

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/transport/socks5"
	"github.com/stretchr/testify/require"
)

type recordingPacketConn struct {
	written  []byte
	destAddr net.Addr
	writes   int
}

func (c *recordingPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, nil, net.ErrClosed
}

func (c *recordingPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	c.written = append(c.written[:0], p...)
	c.destAddr = addr
	c.writes++
	return len(p), nil
}

func (*recordingPacketConn) Close() error                     { return nil }
func (*recordingPacketConn) LocalAddr() net.Addr              { return &net.UDPAddr{Port: 1080} }
func (*recordingPacketConn) SetDeadline(time.Time) error      { return nil }
func (*recordingPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (*recordingPacketConn) SetWriteDeadline(time.Time) error { return nil }

func wantUDPPacket(t *testing.T, source net.Addr, payload []byte) []byte {
	t.Helper()
	packet, err := socks5.EncodeUDPPacket(socks5.ParseAddrToSocksAddr(source), payload)
	require.NoError(t, err)
	return packet
}

func TestSocksPacketConnWriteToMatchesWireFormat(t *testing.T) {
	conn := &recordingPacketConn{}
	uc := &socksPacketConn{PacketConn: conn, rAddr: net.UDPAddrFromAddrPort(netip.MustParseAddrPort("10.0.0.1:1080"))}

	source := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("1.2.3.4:53"))
	n, err := uc.WriteTo([]byte("hello"), source)
	require.NoError(t, err)
	require.Equal(t, len("hello"), n)
	require.Equal(t, 1, conn.writes)
	require.Equal(t, wantUDPPacket(t, source, []byte("hello")), conn.written)
	require.Equal(t, "10.0.0.1:1080", conn.destAddr.String())
}

func TestSocksPacketConnWriteToStaysAllocationFree(t *testing.T) {
	conn := &recordingPacketConn{}
	uc := &socksPacketConn{PacketConn: conn}
	source := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("1.2.3.4:53"))

	allocations := testing.AllocsPerRun(100, func() {
		if _, err := uc.WriteTo([]byte("hello"), source); err != nil {
			t.Fatal(err)
		}
	})
	require.EqualValues(t, 0, allocations)
}
