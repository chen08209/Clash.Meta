package outbound

import (
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

type discardWriteConn struct{ net.Conn }

func (discardWriteConn) Write(b []byte) (int, error) { return len(b), nil }

func udpAddrOf(ip netip.Addr, port int) *net.UDPAddr {
	return &net.UDPAddr{IP: net.IP(ip.AsSlice()), Port: port}
}

func TestVmessPacketWriteToAddressMatch(t *testing.T) {
	v4 := netip.MustParseAddr("1.2.3.4")
	mapped := netip.MustParseAddr("::ffff:1.2.3.4")
	v6 := netip.MustParseAddr("2001:db8::1")

	for _, testCase := range []struct {
		name    string
		allowed net.Addr
		dest    net.Addr
		wantErr bool
	}{
		{name: "same v4", allowed: udpAddrOf(v4, 443), dest: udpAddrOf(v4, 443)},
		{name: "v4 against its mapped form", allowed: udpAddrOf(mapped, 443), dest: udpAddrOf(v4, 443)},
		{name: "same v6", allowed: udpAddrOf(v6, 443), dest: udpAddrOf(v6, 443)},
		{name: "different port", allowed: udpAddrOf(v4, 443), dest: udpAddrOf(v4, 80), wantErr: true},
		{name: "different host", allowed: udpAddrOf(v4, 443), dest: udpAddrOf(netip.MustParseAddr("1.2.3.5"), 443), wantErr: true},
		{name: "different family", allowed: udpAddrOf(v4, 443), dest: udpAddrOf(v6, 443), wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			conn := &vmessPacketConn{Conn: discardWriteConn{}, rAddr: testCase.allowed}
			_, err := conn.WriteTo([]byte("ping"), testCase.dest)
			if testCase.wantErr {
				require.ErrorIs(t, err, ErrUDPRemoteAddrMismatch)
				return
			}
			require.NoError(t, err)
		})
	}
}

// The destination guard runs on every outgoing UDP packet, so it must not
// format either address into a string.
func TestVmessPacketWriteToAllocatesNothing(t *testing.T) {
	addr := udpAddrOf(netip.MustParseAddr("1.2.3.4"), 443)
	conn := &vmessPacketConn{Conn: discardWriteConn{}, rAddr: addr}
	payload := []byte("ping")

	allocations := testing.AllocsPerRun(1000, func() {
		if _, err := conn.WriteTo(payload, addr); err != nil {
			t.Fatal(err)
		}
	})
	require.Zero(t, allocations)
}

func BenchmarkVmessPacketWriteTo(b *testing.B) {
	addr := udpAddrOf(netip.MustParseAddr("1.2.3.4"), 443)
	conn := &vmessPacketConn{Conn: discardWriteConn{}, rAddr: addr}
	payload := make([]byte, 1350)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := conn.WriteTo(payload, addr); err != nil {
			b.Fatal(err)
		}
	}
}
