package tunnel

import (
	"context"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
	"github.com/stretchr/testify/require"
)

func TestShouldStopRetryClassifiesTerminalDialErrors(t *testing.T) {
	_, refusedErr := (&net.Dialer{}).DialContext(context.Background(), "tcp", "127.0.0.1:1")
	require.Error(t, refusedErr)

	require.True(t, shouldStopRetry(refusedErr), "a refused dial must not be retried")
	require.False(t, shouldStopRetry(&net.OpError{Op: "dial", Err: context.DeadlineExceeded}),
		"a timeout may still succeed on retry")
	require.False(t, shouldStopRetry(nil))
}

type refusedDialProxy struct {
	directTestProxy
	attempts atomic.Int64
}

func (p *refusedDialProxy) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	p.attempts.Add(1)
	_, err := (&net.Dialer{}).DialContext(ctx, "tcp", "127.0.0.1:1")
	return nil, err
}

func TestHandleTCPConnDoesNotRetryARefusedDial(t *testing.T) {
	previousStatus := status.Load()
	status.Store(Running)
	t.Cleanup(func() { status.Store(previousStatus) })

	proxy := &refusedDialProxy{}
	UpdateProxies(map[string]C.Proxy{"PROXY": proxy}, nil)
	SetMode(Rule)
	UpdateRules([]C.Rule{rulesCommon.NewMatch("PROXY")}, nil, nil)
	t.Cleanup(func() {
		SetMode(Rule)
		UpdateProxies(nil, nil)
		UpdateRules(nil, nil, nil)
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
		DstIP:   netip.MustParseAddr("93.184.216.34"),
		DstPort: 443,
		Host:    "example.com",
	}
	go Tunnel.HandleTCPConn(<-acceptedCh, metadata)

	_, err = client.Write([]byte("ping"))
	require.NoError(t, err)

	readDone := make(chan error, 1)
	go func() {
		_, readErr := client.Read(make([]byte, 16))
		readDone <- readErr
	}()

	select {
	case <-readDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection was not given up in time")
	}

	require.EqualValues(t, 1, proxy.attempts.Load(), "a refused dial must be attempted exactly once")
}

func BenchmarkTCPConnRefusedProxy(b *testing.B) {
	previousStatus := status.Load()
	status.Store(Running)
	b.Cleanup(func() { status.Store(previousStatus) })

	proxy := &refusedDialProxy{}
	UpdateProxies(map[string]C.Proxy{"PROXY": proxy}, nil)
	SetMode(Rule)
	UpdateRules([]C.Rule{rulesCommon.NewMatch("PROXY")}, nil, nil)
	b.Cleanup(func() {
		SetMode(Rule)
		UpdateProxies(nil, nil)
		UpdateRules(nil, nil, nil)
		invalidateAllProxies()
	})

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

		metadata := &C.Metadata{
			NetWork: C.TCP,
			Type:    C.HTTP,
			SrcIP:   netip.MustParseAddr("127.0.0.1"),
			SrcPort: 12345,
			DstIP:   netip.MustParseAddr("93.184.216.34"),
			DstPort: 443,
			Host:    "example.com",
		}
		go Tunnel.HandleTCPConn(<-acceptedCh, metadata)
		if _, err := client.Write([]byte("ping")); err != nil {
			b.Fatal(err)
		}
		_, _ = client.Read(make([]byte, 16))
		_ = client.Close()
		_ = inbound.Close()
	}
}
