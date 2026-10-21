package packet

import (
	"net"
	"net/netip"
	"syscall"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/require"
)

func newUDPTestPair(tb testing.TB, network string, ip net.IP) (*net.UDPConn, EnhancePacketConn) {
	tb.Helper()

	server, err := net.ListenUDP(network, &net.UDPAddr{IP: ip})
	if err != nil && network == "udp6" {
		tb.Skipf("no loopback for %s: %v", network, err)
	}
	require.NoError(tb, err)
	client, err := net.DialUDP(network, nil, server.LocalAddr().(*net.UDPAddr))
	require.NoError(tb, err)
	tb.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	return client, NewEnhancePacketConn(server)
}

func TestWaitReadFromReturnsAddrPort(t *testing.T) {
	for _, family := range []struct {
		network string
		ip      net.IP
	}{
		{network: "udp4", ip: net.IPv4(127, 0, 0, 1)},
		{network: "udp6", ip: net.IPv6loopback},
	} {
		t.Run(family.network, func(t *testing.T) {
			client, conn := newUDPTestPair(t, family.network, family.ip)
			payload := []byte("ping")

			_, err := client.Write(payload)
			require.NoError(t, err)
			data, addr, err := conn.WaitReadFrom()
			require.NoError(t, err)
			defer pool.Put(data)

			require.Equal(t, payload, data)
			require.Equal(t, client.LocalAddr().(*net.UDPAddr).AddrPort(), addr)
		})
	}
}

func TestWaitReadFromReturnsIPv6AddrPort(t *testing.T) {
	server, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6loopback})
	require.NoError(t, err)
	client, err := net.DialUDP("udp6", nil, server.LocalAddr().(*net.UDPAddr))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	conn := NewEnhancePacketConn(server)
	payload := []byte("ping")
	_, err = client.Write(payload)
	require.NoError(t, err)
	data, addr, err := conn.WaitReadFrom()
	require.NoError(t, err)
	defer pool.Put(data)

	require.Equal(t, payload, data)
	require.Equal(t, client.LocalAddr().(*net.UDPAddr).AddrPort(), addr)
}

func TestWaitReadFromHandlesZeroLengthDatagram(t *testing.T) {
	client, conn := newUDPTestPair(t, "udp4", net.IPv4(127, 0, 0, 1))
	writeZeroLengthDatagram(t, client)
	data, addr, err := conn.WaitReadFrom()
	require.NoError(t, err)
	require.Nil(t, data)
	require.Equal(t, client.LocalAddr().(*net.UDPAddr).AddrPort(), addr)
}

func BenchmarkWaitReadFrom(b *testing.B) {
	for _, size := range []int{64, 1500} {
		b.Run(testPayloadName(size), func(b *testing.B) {
			client, conn := newUDPTestPair(b, "udp4", net.IPv4(127, 0, 0, 1))
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

func TestSockaddrConversion(t *testing.T) {
	// the kernel writes the port in wire order inside the sockaddr
	sa4 := &syscall.RawSockaddrInet4{
		Family: syscall.AF_INET,
		Port:   0x3500, // wire bytes 00 35 for port 53
		Addr:   [4]byte{1, 2, 3, 4},
	}
	require.Equal(t, netip.MustParseAddrPort("1.2.3.4:53"), sockaddrInet4ToAddrPort(sa4))

	sa6 := &syscall.RawSockaddrInet6{
		Family:   syscall.AF_INET6,
		Port:     0xe914, // wire bytes 14 e9 for port 5353
		Addr:     [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01},
		Scope_id: 0,
	}
	require.Equal(t, netip.MustParseAddrPort("[2001:db8::1]:5353"), sockaddrInet6ToAddrPort(sa6))

	sa6.Scope_id = 3
	got := sockaddrInet6ToAddrPort(sa6)
	require.Equal(t, netip.MustParseAddr("2001:db8::1%3").WithZone("3"), got.Addr())
}
