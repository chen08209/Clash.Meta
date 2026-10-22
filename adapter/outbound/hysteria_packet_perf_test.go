package outbound

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/pool"
	hyCore "github.com/metacubex/mihomo/transport/hysteria/core"
	M "github.com/metacubex/sing/common/metadata"
	"github.com/stretchr/testify/require"
)

type stubHyUDPConn struct {
	from M.Socksaddr
	data []byte
}

func (s *stubHyUDPConn) ReadFrom() ([]byte, M.Socksaddr, error) { return s.data, s.from, nil }
func (s *stubHyUDPConn) WriteTo([]byte, M.Socksaddr) error     { return nil }
func (s *stubHyUDPConn) Close() error                          { return nil }
func (s *stubHyUDPConn) LocalAddr() net.Addr                   { return nil }
func (s *stubHyUDPConn) SetDeadline(time.Time) error           { return nil }
func (s *stubHyUDPConn) SetReadDeadline(time.Time) error       { return nil }
func (s *stubHyUDPConn) SetWriteDeadline(time.Time) error      { return nil }

var _ hyCore.UDPConn = (*stubHyUDPConn)(nil)

func TestHyPacketConnReturnsStructuredAddresses(t *testing.T) {
	for _, addrPort := range []netip.AddrPort{
		netip.MustParseAddrPort("203.0.113.7:443"),
		netip.MustParseAddrPort("[2001:db8::1]:5353"),
	} {
		conn := &hyPacketConn{UDPConn: &stubHyUDPConn{
			from: M.SocksaddrFromNetIP(addrPort),
			data: []byte("ping"),
		}}

		buf := make([]byte, 64)
		n, addr, err := conn.ReadFrom(buf)
		require.NoError(t, err)
		require.Equal(t, "ping", string(buf[:n]))
		require.Equal(t, addrPort, addr.(*net.UDPAddr).AddrPort())

		data, got, err := conn.WaitReadFrom()
		require.NoError(t, err)
		require.Equal(t, "ping", string(data))
		require.Equal(t, addrPort, got)
		pool.Put(data)
	}
}

func TestHyPacketConnWriteToAllocatesNothing(t *testing.T) {
	conn := &hyPacketConn{UDPConn: &stubHyUDPConn{}}
	addr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	payload := make([]byte, 1350)

	allocations := testing.AllocsPerRun(1000, func() {
		if _, err := conn.WriteTo(payload, addr); err != nil {
			t.Fatal(err)
		}
	})
	require.Zero(t, allocations)
}

func BenchmarkHyPacketConnWriteTo(b *testing.B) {
	conn := &hyPacketConn{UDPConn: &stubHyUDPConn{}}
	addr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("203.0.113.7:443"))
	payload := make([]byte, 1350)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := conn.WriteTo(payload, addr); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHyPacketConnReadFrom(b *testing.B) {
	conn := &hyPacketConn{UDPConn: &stubHyUDPConn{
		from: M.SocksaddrFromNetIP(netip.MustParseAddrPort("203.0.113.7:443")),
		data: make([]byte, 1350),
	}}
	buf := make([]byte, 2048)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := conn.ReadFrom(buf); err != nil {
			b.Fatal(err)
		}
	}
}

