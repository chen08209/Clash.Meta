package context

import (
	"net"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func TestConnContextStoresTheOriginalConn(t *testing.T) {
	conn, remote := net.Pipe()
	t.Cleanup(func() {
		_ = conn.Close()
		_ = remote.Close()
	})

	connCtx := NewConnContext(conn, &C.Metadata{})
	require.Same(t, conn, connCtx.Conn())
}

func TestConnContextDoesNotAllocateAReadBufferEagerly(t *testing.T) {
	conn, remote := net.Pipe()
	t.Cleanup(func() {
		_ = conn.Close()
		_ = remote.Close()
	})

	allocations := testing.AllocsPerRun(100, func() {
		NewConnContext(conn, &C.Metadata{})
	})
	require.LessOrEqual(t, int(allocations), 2)
}
