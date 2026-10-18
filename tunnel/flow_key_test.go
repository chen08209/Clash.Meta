package tunnel

import (
	"net"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type binaryFlowKeyPacket struct {
	C.PacketAdapter
	key netip.AddrPort
}

type fixedLocalAddrPacket struct {
	C.UDPPacket
	localAddr net.Addr
}

func (packet fixedLocalAddrPacket) LocalAddr() net.Addr {
	return packet.localAddr
}

func (packet binaryFlowKeyPacket) FlowKey() netip.AddrPort {
	return packet.key
}

func (packet binaryFlowKeyPacket) Key() string {
	panic("binary flow-key packet formatted its compatibility key")
}

func TestPacketFlowKeyPrefersBinaryKey(t *testing.T) {
	key := netip.MustParseAddrPort("192.168.1.2:54321")

	actual, ok := packetFlowKey(binaryFlowKeyPacket{key: key})
	require.True(t, ok)
	require.Equal(t, key, actual)
}

func TestPacketFlowKeyParsesCompatibilityKey(t *testing.T) {
	key := netip.MustParseAddrPort("[2001:db8::1]:54321")
	packet := compatibilityKeyPacket{key: key.String()}

	actual, ok := packetFlowKey(packet)
	require.True(t, ok)
	require.Equal(t, key, actual)
}

type compatibilityKeyPacket struct {
	C.PacketAdapter
	key string
}

func (packet compatibilityKeyPacket) Key() string {
	return packet.key
}
