package packet

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func newUDPTestPair(tb testing.TB) (*net.UDPConn, EnhancePacketConn) {
	tb.Helper()

	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(tb, err)
	client, err := net.DialUDP("udp4", nil, server.LocalAddr().(*net.UDPAddr))
	require.NoError(tb, err)
	tb.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	return client, NewEnhancePacketConn(server)
}

func TestWaitReadFromReturnsUDPAddr(t *testing.T) {
	client, conn := newUDPTestPair(t)
	payload := []byte("ping")

	_, err := client.Write(payload)
	require.NoError(t, err)
	data, put, addr, err := conn.WaitReadFrom()
	require.NoError(t, err)
	require.NotNil(t, put)
	defer put()

	require.Equal(t, payload, data)
	require.Equal(t, client.LocalAddr().(*net.UDPAddr).AddrPort(), addr.(*net.UDPAddr).AddrPort())
}

func TestWaitReadFromAddrPortReturnsAddrPort(t *testing.T) {
	client, conn := newUDPTestPair(t)
	fast, ok := conn.(AddrPortWaitReadFrom)
	require.True(t, ok)
	payload := []byte("ping")

	_, err := client.Write(payload)
	require.NoError(t, err)
	data, put, addr, rawAddr, err := fast.WaitReadFromAddrPort()
	require.NoError(t, err)
	require.NotNil(t, put)
	defer put()

	require.Equal(t, payload, data)
	require.Equal(t, client.LocalAddr().(*net.UDPAddr).AddrPort(), addr)
	require.Nil(t, rawAddr)
}

func BenchmarkWaitReadFrom(b *testing.B) {
	benchmarkWaitReadFrom(b, false)
}

func BenchmarkWaitReadFromAddrPort(b *testing.B) {
	benchmarkWaitReadFrom(b, true)
}

func benchmarkWaitReadFrom(b *testing.B, useAddrPort bool) {
	for _, size := range []int{64, 1500} {
		b.Run(testPayloadName(size), func(b *testing.B) {
			client, conn := newUDPTestPair(b)
			payload := make([]byte, size)
			fast, _ := conn.(AddrPortWaitReadFrom)

			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				if _, err := client.Write(payload); err != nil {
					b.Fatal(err)
				}
				var put func()
				var err error
				if useAddrPort {
					_, put, _, _, err = fast.WaitReadFromAddrPort()
				} else {
					_, put, _, err = conn.WaitReadFrom()
				}
				if err != nil {
					b.Fatal(err)
				}
				put()
			}
		})
	}
}

func testPayloadName(size int) string {
	if size == 1500 {
		return "1500B"
	}
	return "small"
}
