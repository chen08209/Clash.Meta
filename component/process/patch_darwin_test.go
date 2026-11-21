package process

import (
	"net"
	"net/netip"
	"os/exec"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindProcessByFdFindsAnotherProcessBySourceAddress(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	nc := exec.Command("/usr/bin/nc", "127.0.0.1", strconv.Itoa(port))
	stdin, err := nc.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, nc.Start())
	defer func() {
		_ = stdin.Close()
		_ = nc.Process.Kill()
		_ = nc.Wait()
	}()

	conn, err := listener.Accept()
	require.NoError(t, err)
	defer conn.Close()
	source := conn.RemoteAddr().(*net.TCPAddr).AddrPort()

	for i := 0; i < 2; i++ {
		_, path, err := findProcessByFd(TCP, source.Addr(), int(source.Port()))
		require.NoError(t, err)
		require.Equal(t, "/usr/bin/nc", path)
	}

	_, _, err = findProcessByFd(TCP, netip.MustParseAddr("127.0.0.2"), int(source.Port()))
	require.ErrorIs(t, err, ErrNotFound)
	_, _, err = findProcessByFd(UDP, source.Addr(), int(source.Port()))
	require.ErrorIs(t, err, ErrNotFound)
}

func BenchmarkFindProcessByFd(b *testing.B) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(b, err)
	defer listener.Close()
	nc := exec.Command("/usr/bin/nc", "127.0.0.1", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	stdin, err := nc.StdinPipe()
	require.NoError(b, err)
	require.NoError(b, nc.Start())
	defer func() {
		_ = stdin.Close()
		_ = nc.Process.Kill()
		_ = nc.Wait()
	}()
	conn, err := listener.Accept()
	require.NoError(b, err)
	defer conn.Close()
	source := conn.RemoteAddr().(*net.TCPAddr).AddrPort()

	b.Run("recent", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, _ = findProcessByFd(TCP, source.Addr(), int(source.Port()))
		}
	})
	b.Run("unknown", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, _ = findProcessByFd(TCP, source.Addr(), 1)
		}
	})
}
