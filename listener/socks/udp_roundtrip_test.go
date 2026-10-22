package socks

import (
	"net"
	"testing"
	"time"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"
	"github.com/stretchr/testify/require"
)

// relayTunnel forwards each datagram straight to the destination the client
// asked for and writes the answer back through the packet, which is what the
// tunnel's direct outbound does.
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

// A datagram has to survive the whole way: the socks5 framing the client sent,
// the listener's flow handling, and the tunnel's write-back answering the same
// client. Nothing else in this package covers that path end to end.
func TestSocksUDPListenerAnswersItsClient(t *testing.T) {
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

	listener, err := NewUDP("127.0.0.1:0", relayTunnel{})
	require.NoError(t, err)
	defer listener.Close()

	client, err := net.Dial("udp4", listener.Address())
	require.NoError(t, err)
	defer client.Close()

	echoAddr := echo.LocalAddr().(*net.UDPAddr).AddrPort()
	request, err := socks5.EncodeUDPPacket(socks5.AddrFromStdAddrPort(echoAddr), []byte("ping"))
	require.NoError(t, err)
	_, err = client.Write(request)
	require.NoError(t, err)

	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	reply := make([]byte, 64*1024)
	n, err := client.Read(reply)
	require.NoError(t, err)

	from, payload, err := socks5.DecodeUDPPacket(reply[:n])
	require.NoError(t, err)
	require.Equal(t, []byte("ping"), payload)
	require.Equal(t, echoAddr, from.AddrPort())
}
