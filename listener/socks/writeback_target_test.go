package socks

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/component/nat"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type writeBackRecordingConn struct {
	writes int
	dest   net.Addr
}

func (c *writeBackRecordingConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, nil, net.ErrClosed
}

func (c *writeBackRecordingConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	c.writes++
	c.dest = addr
	return len(b), nil
}

func (c *writeBackRecordingConn) Close() error        { return nil }
func (c *writeBackRecordingConn) LocalAddr() net.Addr { return nil }
func (c *writeBackRecordingConn) SetDeadline(time.Time) error {
	return nil
}

func (c *writeBackRecordingConn) SetReadDeadline(time.Time) error {
	return nil
}

func (c *writeBackRecordingConn) SetWriteDeadline(time.Time) error {
	return nil
}

// A flow's write-back proxy keeps the last packet the sender handed it, so a
// reply can be written back to that client, and the sender drops that packet
// as soon as it processed it. Recycling the wrapper must not move the flow's
// write-back target onto whichever datagram claims the wrapper next.
func TestFlowWriteBackKeepsItsClientAfterThePacketIsDropped(t *testing.T) {
	clientA := &writeBackRecordingConn{}
	clientB := &writeBackRecordingConn{}
	addrA := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.168.1.2:1111"))
	addrB := net.UDPAddrFromAddrPort(netip.MustParseAddrPort("192.168.1.3:2222"))
	replyTarget := netip.MustParseAddrPort("1.2.3.4:53")

	for round := 0; round < 100; round++ {
		packet := newSocksPacket(clientA, addrA, []byte("request"), []byte("request"))
		proxy := nat.NewWriteBackProxy(C.NewPacketAdapter(packet, &C.Metadata{}))
		packet.Drop() // what packetSender.processPacket does right after it sends

		// the next datagram on this listener gets its own wrapper
		_ = newSocksPacket(clientB, addrB, []byte("other"), []byte("other"))

		_, err := proxy.WriteBack([]byte("reply"), replyTarget)
		require.NoError(t, err)
		if clientA.writes != round+1 || clientB.writes != 0 {
			t.Fatalf("round %d: the flow's reply went to its own client %d times and to another client %d times",
				round, clientA.writes, clientB.writes)
		}
	}
}
