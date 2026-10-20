package deadline

import (
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/require"
)

type stubEnhancePacketConn struct {
	data       []byte
	addr       netip.AddrPort
	waitCalled int
}

func (c *stubEnhancePacketConn) WaitReadFrom() ([]byte, netip.AddrPort, error) {
	c.waitCalled++
	return c.data, c.addr, nil
}

func (*stubEnhancePacketConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, net.ErrClosed }
func (*stubEnhancePacketConn) WriteTo([]byte, net.Addr) (int, error)  { return 0, net.ErrClosed }
func (*stubEnhancePacketConn) Close() error                           { return nil }
func (*stubEnhancePacketConn) LocalAddr() net.Addr                    { return &net.UDPAddr{Port: 1080} }
func (*stubEnhancePacketConn) SetDeadline(time.Time) error            { return nil }
func (*stubEnhancePacketConn) SetReadDeadline(time.Time) error        { return nil }
func (*stubEnhancePacketConn) SetWriteDeadline(time.Time) error       { return nil }

func TestEnhanceWaitReadFromWithoutDeadline(t *testing.T) {
	source := netip.MustParseAddrPort("1.2.3.4:53")
	inner := &stubEnhancePacketConn{
		data: []byte("pong"),
		addr: source,
	}
	conn := NewEnhancePacketConn(inner)

	data, addr, err := conn.WaitReadFrom()
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), data)
	require.Equal(t, source, addr)
	_ = pool.Put(data)
}

func TestEnhanceWaitReadFromWithExpiredDeadline(t *testing.T) {
	inner := &stubEnhancePacketConn{
		data: []byte("pong"),
		addr: netip.MustParseAddrPort("1.2.3.4:53"),
	}
	conn := NewEnhancePacketConn(inner)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(-time.Second)))

	data, addr, err := conn.WaitReadFrom()
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
	require.Nil(t, data)
	require.Equal(t, netip.AddrPort{}, addr)
	require.Equal(t, 0, inner.waitCalled, "an expired deadline must not reach the inner conn")
}
