package vless

import (
	"bytes"
	"io"
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeConn struct {
	net.Conn
	r      io.Reader
	w      io.Writer
	writes int
}

func (c *fakeConn) Read(b []byte) (int, error) { return c.r.Read(b) }

func (c *fakeConn) Write(b []byte) (int, error) {
	c.writes++
	if c.w != nil {
		return c.w.Write(b)
	}
	return len(b), nil
}

func TestPacketConnWriteToFraming(t *testing.T) {
	var out bytes.Buffer
	conn := &fakeConn{w: &out}
	pc := &PacketConn{Conn: conn, rAddr: netip.MustParseAddrPort("203.0.113.7:443")}

	n, err := pc.WriteTo([]byte("hello"), nil)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, []byte{0x00, 0x05, 'h', 'e', 'l', 'l', 'o'}, out.Bytes())
	require.Equal(t, 1, conn.writes, "the length and the payload share one stream write")
}

func TestPacketConnReadFromFramedPayload(t *testing.T) {
	conn := &fakeConn{r: bytes.NewReader([]byte{0x00, 0x05, 'h', 'e', 'l', 'l', 'o'})}
	pc := &PacketConn{Conn: conn, rAddr: netip.MustParseAddrPort("203.0.113.7:443")}

	buf := make([]byte, 64)
	n, addr, err := pc.ReadFrom(buf)
	require.NoError(t, err)
	require.Equal(t, "hello", string(buf[:n]))
	require.Equal(t, netip.MustParseAddrPort("203.0.113.7:443"), addr.(*net.UDPAddr).AddrPort())
}
