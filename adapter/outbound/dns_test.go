package outbound

import (
	"context"
	"net"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/require"
)

func TestDNSPacketConnWaitReadFromReturnsTheQueuedAddrPort(t *testing.T) {
	conn := &dnsPacketConn{response: make(chan dnsPacket, 1), ctx: context.Background()}
	addr := netip.MustParseAddrPort("1.2.3.4:53")
	conn.response <- dnsPacket{data: []byte("answer"), addr: addr}

	data, got, err := conn.WaitReadFrom()
	require.NoError(t, err)
	require.Equal(t, []byte("answer"), data)
	require.Equal(t, addr, got)
	_ = pool.Put(data)
}

func TestDNSPacketConnReadFromMaterializesTheQueuedAddrPort(t *testing.T) {
	conn := &dnsPacketConn{response: make(chan dnsPacket, 1), ctx: context.Background()}
	addr := netip.MustParseAddrPort("1.2.3.4:53")
	conn.response <- dnsPacket{data: []byte("answer"), addr: addr}

	buf := make([]byte, 16)
	n, got, err := conn.ReadFrom(buf)
	require.NoError(t, err)
	require.Equal(t, len("answer"), n)
	require.Equal(t, "1.2.3.4:53", got.String())
	require.Equal(t, addr, got.(*net.UDPAddr).AddrPort())
}

func TestDNSPacketConnClosedAnswersWithErrClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	conn := &dnsPacketConn{response: make(chan dnsPacket, 1), ctx: ctx, cancel: cancel}
	conn.Close()

	_, _, err := conn.WaitReadFrom()
	require.ErrorIs(t, err, net.ErrClosed)
}
