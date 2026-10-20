package trojan

import (
	"net"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/transport/socks5"
	"github.com/stretchr/testify/require"
)

// serveTrojanWire consumes one query off the wire and answers with payload in
// the same trojan UDP framing, so the client side of the pipe completes.
func serveTrojanWire(t *testing.T, server net.Conn, payload []byte) {
	t.Helper()

	if _, _, _, err := ReadPacket(server, make([]byte, 512)); err != nil {
		t.Error(err)
		return
	}
	if _, err := writePacket(server, socks5.ParseAddr("1.2.3.4:53"), payload); err != nil {
		t.Error(err)
	}
}

func TestPacketConnWaitReadFrom(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})

	source := netip.MustParseAddrPort("1.2.3.4:53")
	pc := &PacketConn{Conn: client}
	go serveTrojanWire(t, server, []byte("hello"))

	go func() {
		if _, err := pc.WriteTo([]byte("hello"), net.UDPAddrFromAddrPort(source)); err != nil {
			t.Error(err)
		}
	}()

	data, put, addr, err := pc.WaitReadFrom()
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), data)
	require.Equal(t, source, addr)
	require.NotNil(t, put)
	put()
}

func TestPacketConnWaitReadFromZeroLength(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})

	source := netip.MustParseAddrPort("1.2.3.4:53")
	pc := &PacketConn{Conn: client}
	go serveTrojanWire(t, server, nil)

	go func() {
		if _, err := pc.WriteTo(nil, net.UDPAddrFromAddrPort(source)); err != nil {
			t.Error(err)
		}
	}()

	data, put, addr, err := pc.WaitReadFrom()
	require.NoError(t, err)
	require.Nil(t, data)
	require.Equal(t, source, addr)
	require.Nil(t, put, "a zero-length datagram owns no buffer")
}

func TestPacketConnReadFrom(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})

	source := netip.MustParseAddrPort("1.2.3.4:53")
	pc := &PacketConn{Conn: client}
	go serveTrojanWire(t, server, []byte("hello"))

	go func() {
		if _, err := pc.WriteTo([]byte("hello"), net.UDPAddrFromAddrPort(source)); err != nil {
			t.Error(err)
		}
	}()

	buf := make([]byte, 512)
	n, addr, err := pc.ReadFrom(buf)
	require.NoError(t, err)
	require.Equal(t, len("hello"), n)
	require.Equal(t, []byte("hello"), buf[:n])
	require.Equal(t, source, addr.(*net.UDPAddr).AddrPort())
}
