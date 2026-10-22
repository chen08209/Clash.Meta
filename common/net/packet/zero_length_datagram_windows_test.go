//go:build windows

package packet

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func writeZeroLengthDatagram(t *testing.T, client *net.UDPConn) {
	t.Helper()
	raw, err := client.SyscallConn()
	require.NoError(t, err)

	var writeErr error
	err = raw.Write(func(fd uintptr) bool {
		var sent uint32
		writeErr = windows.WSASend(windows.Handle(fd), &windows.WSABuf{Len: 0}, 1, &sent, 0, nil, nil)
		return true
	})
	require.NoError(t, err)
	require.NoError(t, writeErr)
}
