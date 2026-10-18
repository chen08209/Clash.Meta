package tunnel

import (
	"context"
	"io"
	"net"
	"net/netip"
	"strconv"
	"testing"
	"time"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type peekRelayConn struct {
	N.ExtendedConn
}

func (c *peekRelayConn) Chains() C.Chain               { return C.Chain{"DIRECT"} }
func (c *peekRelayConn) ProviderChains() C.Chain       { return nil }
func (c *peekRelayConn) AppendToChains(C.ProxyAdapter) {}
func (c *peekRelayConn) RemoteDestination() string     { return "" }

type peekDirectProxy struct {
	directTestProxy
}

func (peekDirectProxy) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(metadata.Host, strconv.Itoa(int(metadata.DstPort))))
	if err != nil {
		return nil, err
	}
	return &peekRelayConn{ExtendedConn: N.NewExtendedConn(conn)}, nil
}

func startPeekEcho(tb testing.TB) int {
	tb.Helper()

	echo, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = echo.Close() })
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return echo.Addr().(*net.TCPAddr).Port
}

// A client that connects first and speaks later must survive: the pre-dial peek
// goroutine used to leave a 200ms read deadline armed on the shared connection,
// which killed the relay before the client's first byte arrived.
func TestDirectConnSurvivesLateFirstByte(t *testing.T) {
	previousStatus := status.Load()
	status.Store(Running)
	t.Cleanup(func() { status.Store(previousStatus) })

	UpdateProxies(map[string]C.Proxy{"DIRECT": peekDirectProxy{}}, nil)
	SetMode(Direct)
	t.Cleanup(func() {
		SetMode(Rule)
		UpdateProxies(nil, nil)
		invalidateAllProxies()
	})

	port := startPeekEcho(t)

	inbound, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer inbound.Close()

	acceptedCh := make(chan net.Conn, 1)
	go func() {
		c, err := inbound.Accept()
		require.NoError(t, err)
		acceptedCh <- c
	}()

	client, err := net.Dial("tcp", inbound.Addr().String())
	require.NoError(t, err)
	defer client.Close()

	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.HTTP,
		SrcIP:   netip.MustParseAddr("127.0.0.1"),
		SrcPort: 12345,
		DstIP:   netip.MustParseAddr("127.0.0.1"),
		DstPort: uint16(port),
		Host:    "localhost",
	}
	go Tunnel.HandleTCPConn(<-acceptedCh, metadata)

	time.Sleep(300 * time.Millisecond) // longer than the peek deadline
	_, err = client.Write([]byte("late-hello"))
	require.NoError(t, err)

	require.NoError(t, client.SetReadDeadline(time.Now().Add(3*time.Second)))
	buf := make([]byte, len("late-hello"))
	_, err = io.ReadFull(client, buf)
	require.NoError(t, err, "the relay dropped a connection whose first byte arrived late")
	require.Equal(t, "late-hello", string(buf))
}

type handshakeTestConn struct {
	N.ExtendedConn
	firstWrite chan []byte
}

func (c *handshakeTestConn) NeedHandshake() bool           { return true }
func (c *handshakeTestConn) Chains() C.Chain               { return C.Chain{"PROXY"} }
func (c *handshakeTestConn) ProviderChains() C.Chain       { return nil }
func (c *handshakeTestConn) AppendToChains(C.ProxyAdapter) {}
func (c *handshakeTestConn) RemoteDestination() string     { return "" }

func (c *handshakeTestConn) Write(b []byte) (int, error) {
	if len(b) > 0 {
		select {
		case c.firstWrite <- append([]byte(nil), b...):
		default:
		}
	}
	return c.ExtendedConn.Write(b)
}

type handshakeProxy struct {
	directTestProxy
	firstWrite chan []byte
}

func (p *handshakeProxy) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(metadata.Host, strconv.Itoa(int(metadata.DstPort))))
	if err != nil {
		return nil, err
	}
	return &handshakeTestConn{ExtendedConn: N.NewExtendedConn(conn), firstWrite: p.firstWrite}, nil
}

// A handshake conn must still receive the client's first bytes directly, even
// though the buffer is now filled after the dial instead of before it.
func TestHandshakeConnReceivesClientHello(t *testing.T) {
	previousStatus := status.Load()
	status.Store(Running)
	t.Cleanup(func() { status.Store(previousStatus) })

	sink, err := net.Listen("tcp", "127.0.0.1:443")
	if err != nil {
		t.Skip("port 443 unavailable")
	}
	defer sink.Close()
	go func() {
		for {
			c, err := sink.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()

	firstWrite := make(chan []byte, 1)
	UpdateProxies(map[string]C.Proxy{"DIRECT": &handshakeProxy{firstWrite: firstWrite}}, nil)
	SetMode(Direct)
	t.Cleanup(func() {
		SetMode(Rule)
		UpdateProxies(nil, nil)
		invalidateAllProxies()
	})

	inbound, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer inbound.Close()

	acceptedCh := make(chan net.Conn, 1)
	go func() {
		c, err := inbound.Accept()
		require.NoError(t, err)
		acceptedCh <- c
	}()

	client, err := net.Dial("tcp", inbound.Addr().String())
	require.NoError(t, err)
	defer client.Close()

	metadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    C.HTTP,
		SrcIP:   netip.MustParseAddr("127.0.0.1"),
		SrcPort: 12345,
		DstIP:   netip.MustParseAddr("127.0.0.1"),
		DstPort: 443,
		Host:    "localhost",
	}
	go Tunnel.HandleTCPConn(<-acceptedCh, metadata)

	_, err = client.Write([]byte("client-hello"))
	require.NoError(t, err)

	select {
	case written := <-firstWrite:
		require.Equal(t, "client-hello", string(written), "the handshake conn must be handed the client's first bytes")
	case <-time.After(3 * time.Second):
		t.Fatal("the handshake conn never received the client's first bytes")
	}
}

func BenchmarkTCPConnSetupDirect(b *testing.B) {
	previousStatus := status.Load()
	status.Store(Running)
	b.Cleanup(func() { status.Store(previousStatus) })

	UpdateProxies(map[string]C.Proxy{"DIRECT": peekDirectProxy{}}, nil)
	SetMode(Direct)
	b.Cleanup(func() {
		SetMode(Rule)
		UpdateProxies(nil, nil)
		invalidateAllProxies()
	})

	echo, err := net.Listen("tcp", "127.0.0.1:443")
	if err != nil {
		b.Skip("port 443 unavailable")
	}
	b.Cleanup(func() { _ = echo.Close() })
	accepts := make(chan struct{}, 64)
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			accepts <- struct{}{}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inbound, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			b.Fatal(err)
		}
		acceptedCh := make(chan net.Conn, 1)
		go func() {
			c, err := inbound.Accept()
			if err != nil {
				return
			}
			acceptedCh <- c
		}()
		client, err := net.Dial("tcp", inbound.Addr().String())
		if err != nil {
			b.Fatal(err)
		}
		server := <-acceptedCh
		metadata := &C.Metadata{
			NetWork: C.TCP,
			Type:    C.HTTP,
			SrcIP:   netip.MustParseAddr("127.0.0.1"),
			SrcPort: 12345,
			DstIP:   netip.MustParseAddr("127.0.0.1"),
			DstPort: 443,
			Host:    "localhost",
		}
		go Tunnel.HandleTCPConn(server, metadata)
		if _, err := client.Write([]byte("ping")); err != nil {
			b.Fatal(err)
		}
		<-accepts
		_, _ = client.Read(make([]byte, 4))
		_ = client.Close()
		_ = server.Close()
		_ = inbound.Close()
	}
}
