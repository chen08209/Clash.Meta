package dialer

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsConnectionRefused(t *testing.T) {
	_, refused := (&net.Dialer{}).DialContext(context.Background(), "tcp", "127.0.0.1:1")
	require.Error(t, refused)

	require.False(t, IsConnectionRefused(nil))
	require.True(t, IsConnectionRefused(refused))
	require.True(t, IsConnectionRefused(&net.OpError{Op: "dial", Net: "tcp", Err: refused}))
	require.True(t, IsConnectionRefused(errors.New("connectex: No connection could be made because the target machine actively refused it.")))
	require.False(t, IsConnectionRefused(errors.New("i/o timeout")))
	require.False(t, IsConnectionRefused(context.DeadlineExceeded))
}
