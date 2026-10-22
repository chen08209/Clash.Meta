package trojan

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter/inbound"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
	"github.com/stretchr/testify/require"
)

type fixedLocalPacketConn struct{}

var fixedLocalAddr = &net.UDPAddr{Port: 1080}

func (*fixedLocalPacketConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, net.ErrClosed }
func (*fixedLocalPacketConn) WriteTo([]byte, net.Addr) (int, error)  { return 0, net.ErrClosed }
func (*fixedLocalPacketConn) WriteToAddrPort([]byte, netip.AddrPort) (int, error) {
	return 0, net.ErrClosed
}
func (*fixedLocalPacketConn) Close() error                     { return nil }
func (*fixedLocalPacketConn) LocalAddr() net.Addr              { return fixedLocalAddr }
func (*fixedLocalPacketConn) SetDeadline(time.Time) error      { return nil }
func (*fixedLocalPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (*fixedLocalPacketConn) SetWriteDeadline(time.Time) error { return nil }

func TestInboundNewPacketAllocationCount(t *testing.T) {
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	pkt := &packet{pc: &fixedLocalPacketConn{}, source: source}
	target := socks5.AddrFromStdAddrPort(netip.MustParseAddrPort("8.8.8.8:53"))

	allocations := testing.AllocsPerRun(100, func() {
		inbound.NewPacket(target, pkt, C.TROJAN)
	})
	// the packet's own FlowKey must serve SetSrcAddr; materialising a
	// net.UDPAddr for it would add two allocations per datagram
	require.EqualValues(t, 4, allocations)
}

func TestInboundNewPacketReadsTheFlowKey(t *testing.T) {
	source := netip.MustParseAddrPort("192.168.1.2:54321")
	pkt := &packet{pc: &fixedLocalPacketConn{}, source: source}
	target := socks5.AddrFromStdAddrPort(netip.MustParseAddrPort("8.8.8.8:53"))

	adapter, metadata := inbound.NewPacket(target, pkt, C.TROJAN)
	require.Equal(t, source, metadata.SourceAddrPort())
	flow, ok := adapter.(interface{ FlowKey() netip.AddrPort })
	require.True(t, ok)
	require.Equal(t, source, flow.FlowKey())
}
