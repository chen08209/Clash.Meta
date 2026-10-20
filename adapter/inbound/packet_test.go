package inbound

import (
	"net"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
	"github.com/stretchr/testify/require"
)

type packetTestUDPPacket struct {
	src net.Addr
	in  net.Addr
}

func (p *packetTestUDPPacket) LocalAddr() net.Addr { return p.src }
func (p *packetTestUDPPacket) InAddr() net.Addr    { return p.in }
func (p *packetTestUDPPacket) Drop()               {}
func (p *packetTestUDPPacket) Data() []byte        { return nil }

func (p *packetTestUDPPacket) WriteBack(b []byte, addr netip.AddrPort) (int, error) {
	return len(b), nil
}

func newPacketTestPacket() *packetTestUDPPacket {
	return &packetTestUDPPacket{
		src: net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.168.1.2:5353")),
		in:  net.UDPAddrFromAddrPort(netip.MustParseAddrPort("10.0.0.5:5354")),
	}
}

func TestNewPacketSetsSourceAndInboundAddresses(t *testing.T) {
	packet := newPacketTestPacket()
	_, metadata := NewPacket(socks5.ParseAddr("93.184.216.34:53"), packet, C.SOCKS5)

	require.Equal(t, C.UDP, metadata.NetWork)
	require.Equal(t, C.SOCKS5, metadata.Type)
	require.Equal(t, netip.MustParseAddr("93.184.216.34"), metadata.DstIP)
	require.EqualValues(t, 53, metadata.DstPort)
	require.Equal(t, netip.MustParseAddr("192.168.1.2"), metadata.SrcIP)
	require.EqualValues(t, 5353, metadata.SrcPort)
	require.Equal(t, netip.MustParseAddr("10.0.0.5"), metadata.InIP)
	require.EqualValues(t, 5354, metadata.InPort)
}

type invalidFlowKeyPacket struct {
	C.UDPPacket
	localAddr net.Addr
}

func (p *invalidFlowKeyPacket) FlowKey() netip.AddrPort { return netip.AddrPort{} }
func (p *invalidFlowKeyPacket) LocalAddr() net.Addr     { return p.localAddr }

func TestNewPacketFallsBackToLocalAddrOnAnInvalidFlowKey(t *testing.T) {
	addr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 5), Port: 5354}
	packet := &invalidFlowKeyPacket{localAddr: addr}

	_, metadata := NewPacket(socks5.ParseAddr("93.184.216.34:53"), packet, C.SOCKS5)
	require.Equal(t, netip.MustParseAddr("10.0.0.5"), metadata.SrcIP)
	require.EqualValues(t, 5354, metadata.SrcPort)
}

func TestSetSrcAddrMatchesWithSrcAddr(t *testing.T) {
	addr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.168.1.2:5353"))

	viaClosure := &C.Metadata{}
	ApplyAdditions(viaClosure, WithSrcAddr(addr))

	direct := &C.Metadata{}
	SetSrcAddr(direct, addr)

	require.Equal(t, viaClosure.SrcIP, direct.SrcIP)
	require.Equal(t, viaClosure.SrcPort, direct.SrcPort)
}

func TestSetInAddrMatchesWithInAddr(t *testing.T) {
	addr := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("10.0.0.5:5354"))

	viaClosure := &C.Metadata{}
	ApplyAdditions(viaClosure, WithInAddr(addr))

	direct := &C.Metadata{}
	SetInAddr(direct, addr)

	require.Equal(t, viaClosure.InIP, direct.InIP)
	require.Equal(t, viaClosure.InPort, direct.InPort)
}

// NewPacket must not allocate a closure or a destination address per packet.
func TestNewPacketAllocationCount(t *testing.T) {
	packet := newPacketTestPacket()
	target := socks5.ParseAddr("93.184.216.34:53")

	allocations := testing.AllocsPerRun(200, func() {
		_, _ = NewPacket(target, packet, C.SOCKS5)
	})
	require.EqualValues(t, 1, allocations, "only the metadata itself may be allocated per packet")
}
