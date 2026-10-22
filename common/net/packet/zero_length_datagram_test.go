//go:build !windows

package packet

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeZeroLengthDatagram(t *testing.T, client *net.UDPConn) {
	t.Helper()
	_, err := client.Write(nil)
	require.NoError(t, err)
}
