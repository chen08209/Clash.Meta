package dns

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type poolConn struct {
	closed bool
}

func (c *poolConn) Read(b []byte) (int, error)       { return 0, nil }
func (c *poolConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *poolConn) Close() error                     { c.closed = true; return nil }
func (c *poolConn) LocalAddr() net.Addr              { return nil }
func (c *poolConn) RemoteAddr() net.Addr             { return nil }
func (c *poolConn) SetDeadline(time.Time) error      { return nil }
func (c *poolConn) SetReadDeadline(time.Time) error  { return nil }
func (c *poolConn) SetWriteDeadline(time.Time) error { return nil }

func TestClientUDPPoolReusesIdleConnsLifo(t *testing.T) {
	c := &client{schema: "udp"}
	first := &poolConn{}
	second := &poolConn{}
	c.keepConn("udp", first)
	c.keepConn("udp", second)

	got, err := c.takeConn(context.Background(), "udp", "192.0.2.53:53")
	require.NoError(t, err)
	require.Same(t, second, got)
	require.False(t, second.closed)

	c.keepConn("udp", got)
	got, err = c.takeConn(context.Background(), "udp", "192.0.2.53:53")
	require.NoError(t, err)
	require.Same(t, second, got)

	// A tcp exchange is never pooled: the handed-back conn closes instead.
	c.keepConn("tcp", first)
	require.True(t, first.closed)
}

func TestClientUDPPoolCapsIdleAndResetDrains(t *testing.T) {
	c := &client{schema: "udp"}
	conns := make([]*poolConn, 0, maxIdleDNSConns+1)
	for i := 0; i < maxIdleDNSConns+1; i++ {
		conn := &poolConn{}
		conns = append(conns, conn)
		c.keepConn("udp", conn)
	}
	require.Len(t, c.idle, maxIdleDNSConns)
	require.True(t, conns[maxIdleDNSConns].closed, "the conn beyond the cap must close")

	c.ResetConnection()
	for _, conn := range conns[:maxIdleDNSConns] {
		require.True(t, conn.closed)
	}
	require.Empty(t, c.idle)
}
