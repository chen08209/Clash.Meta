package packet

import (
	"net"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
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

func TestWaitReadFromReturnsAddrPort(t *testing.T) {
	client, conn := newUDPTestPair(t)
	payload := []byte("ping")

	_, err := client.Write(payload)
	require.NoError(t, err)
	data, addr, err := conn.WaitReadFrom()
	require.NoError(t, err)
	defer pool.Put(data)

	require.Equal(t, payload, data)
	require.Equal(t, client.LocalAddr().(*net.UDPAddr).AddrPort(), addr)
}

func BenchmarkWaitReadFrom(b *testing.B) {
	for _, size := range []int{64, 1500} {
		b.Run(testPayloadName(size), func(b *testing.B) {
			client, conn := newUDPTestPair(b)
			payload := make([]byte, size)

			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				if _, err := client.Write(payload); err != nil {
					b.Fatal(err)
				}
				data, _, err := conn.WaitReadFrom()
				if err != nil {
					b.Fatal(err)
				}
				pool.Put(data)
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
