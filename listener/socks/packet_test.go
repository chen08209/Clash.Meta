package socks

import (
	"net"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPacketExposesItsPayloadAndDropsOnce(t *testing.T) {
	conn := &recordingPacketConn{}
	pkt := newWriteBackTestPacket(conn)

	payload := []byte("dns-answer")
	pkt.payload = payload
	buffer := pool.Get(1024)
	require.NotNil(t, buffer)
	pkt.data = buffer

	assert.Equal(t, payload, pkt.Data())

	// the drop returns the pooled buffer and clears the payload
	pkt.Drop()
	assert.Nil(t, pkt.Data())
	assert.Nil(t, pkt.data)

	// a second drop is a no-op: the buffer is not returned twice
	pkt.Drop()
	assert.Nil(t, pkt.Data())

	// the local and inbound addresses stay the connection's
	assert.Equal(t, net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.168.1.2:54321")), pkt.LocalAddr())
	assert.Equal(t, &net.UDPAddr{Port: 1080}, pkt.InAddr())
}
