package sing_vless

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/metacubex/sing/common/buf"
	N "github.com/metacubex/sing/common/network"
	"github.com/stretchr/testify/require"
)

type fakeExtendedConn struct {
	net.Conn
	w      io.Writer
	writes int
}

func (c *fakeExtendedConn) Read([]byte) (int, error)     { return 0, io.EOF }
func (c *fakeExtendedConn) ReadBuffer(*buf.Buffer) error { return nil }
func (c *fakeExtendedConn) WriteBuffer(*buf.Buffer) error {
	return nil
}
func (c *fakeExtendedConn) Write(b []byte) (int, error) {
	c.writes++
	if c.w != nil {
		return c.w.Write(b)
	}
	return len(b), nil
}
func (c *fakeExtendedConn) Close() error                     { return nil }
func (c *fakeExtendedConn) LocalAddr() net.Addr              { return nil }
func (c *fakeExtendedConn) RemoteAddr() net.Addr             { return nil }
func (c *fakeExtendedConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeExtendedConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeExtendedConn) SetWriteDeadline(time.Time) error { return nil }

var _ N.ExtendedConn = (*fakeExtendedConn)(nil)

func TestServerPacketConnWriteToFraming(t *testing.T) {
	var out bytes.Buffer
	conn := &fakeExtendedConn{w: &out}
	pc := &serverPacketConn{ExtendedConn: conn}

	n, err := pc.WriteTo([]byte("hello"), nil)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, []byte{0x00, 0x05, 'h', 'e', 'l', 'l', 'o'}, out.Bytes())
	require.Equal(t, 1, conn.writes, "the length and the payload share one stream write")
}
