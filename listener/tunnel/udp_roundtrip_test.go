package tunnel

import (
	"net"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter/inbound"
	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

// relayTunnel forwards each datagram to the destination it was handed and
// writes the answer back through the packet, which is what the tunnel's direct
// outbound does.
type relayTunnel struct {
	C.Tunnel
}

func (relayTunnel) HandleUDPPacket(packet C.UDPPacket, metadata *C.Metadata) {
	defer packet.Drop()

	remote, err := net.Dial("udp", metadata.RemoteAddress())
	if err != nil {
		return
	}
	defer remote.Close()

	if _, err = remote.Write(packet.Data()); err != nil {
		return
	}
	_ = remote.SetReadDeadline(time.Now().Add(5 * time.Second))
	reply := make([]byte, 64*1024)
	n, err := remote.Read(reply)
	if err != nil {
		return
	}

	_, _ = packet.WriteBack(reply[:n], N.AddrPortFromNetAddr(remote.RemoteAddr()))
}

// The listener's whole job is to carry a datagram to the target it was opened
// with and carry the answer back to whichever client sent it.
func TestTunnelUDPListenerForwardsToItsTarget(t *testing.T) {
	echo, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer echo.Close()
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, from, err := echo.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = echo.WriteToUDP(buf[:n], from)
		}
	}()

	listener, err := NewUDP("127.0.0.1:0", echo.LocalAddr().String(), "", inbound.NewListenConfig(), relayTunnel{})
	require.NoError(t, err)
	defer listener.Close()

	client, err := net.Dial("udp4", listener.Address())
	require.NoError(t, err)
	defer client.Close()

	_, err = client.Write([]byte("ping"))
	require.NoError(t, err)

	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	reply := make([]byte, 64*1024)
	n, err := client.Read(reply)
	require.NoError(t, err)
	require.Equal(t, "ping", string(reply[:n]))
}
