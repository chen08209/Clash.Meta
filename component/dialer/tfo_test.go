package dialer

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDialTFOPropagatesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	conn, err := dialTFO(ctx, net.Dialer{}, "tcp", "127.0.0.1:1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	require.ErrorIs(t, conn.(*tfoConn).ctx.Err(), context.Canceled)
}

func TestDialTFOCloseCancelsTheDialContext(t *testing.T) {
	conn, err := dialTFO(context.Background(), net.Dialer{}, "tcp", "127.0.0.1:1")
	require.NoError(t, err)

	tfoConn := conn.(*tfoConn)
	require.NoError(t, conn.Close())
	require.ErrorIs(t, tfoConn.ctx.Err(), context.Canceled)
}
