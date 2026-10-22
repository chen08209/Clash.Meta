package dns

import (
	"context"
	"net"
	"testing"
	"time"

	D "github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func startUDPDNSServer(tb testing.TB) (string, <-chan struct{}) {
	tb.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = conn.Close() })

	received := make(chan struct{}, 1)
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, addr, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}
			var query D.Msg
			if err := query.Unpack(buffer[:n]); err != nil {
				continue
			}
			select {
			case received <- struct{}{}:
			default:
			}

			var response D.Msg
			response.SetReply(&query)
			if len(query.Question) != 0 {
				question := query.Question[0]
				response.Answer = append(response.Answer, &D.A{
					Hdr: D.RR_Header{
						Name:   question.Name,
						Rrtype: D.TypeA,
						Class:  D.ClassINET,
						Ttl:    60,
					},
					A: net.IPv4(192, 0, 2, 10),
				})
			}
			packed, err := response.Pack()
			if err != nil {
				continue
			}
			_, _ = conn.WriteTo(packed, addr)
		}
	}()

	_, port, err := net.SplitHostPort(conn.LocalAddr().String())
	require.NoError(tb, err)
	return port, received
}

func startTruncatedDNSServers(tb testing.TB) string {
	tb.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = listener.Close() })

	address := listener.Addr().String()
	packetConn, err := net.ListenPacket("udp", address)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = packetConn.Close() })

	go func() {
		buffer := make([]byte, 4096)
		for {
			n, addr, err := packetConn.ReadFrom(buffer)
			if err != nil {
				return
			}
			var query D.Msg
			if err := query.Unpack(buffer[:n]); err != nil {
				continue
			}
			var response D.Msg
			response.SetReply(&query)
			response.Truncated = true
			packed, err := response.Pack()
			if err != nil {
				continue
			}
			_, _ = packetConn.WriteTo(packed, addr)
		}
	}()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				dnsConn := &D.Conn{Conn: conn}
				query, err := dnsConn.ReadMsg()
				if err != nil {
					return
				}
				var response D.Msg
				response.SetReply(query)
				if len(query.Question) != 0 {
					question := query.Question[0]
					response.Answer = append(response.Answer, &D.A{
						Hdr: D.RR_Header{
							Name:   question.Name,
							Rrtype: D.TypeA,
							Class:  D.ClassINET,
							Ttl:    60,
						},
						A: net.IPv4(192, 0, 2, 11),
					})
				}
				_ = dnsConn.WriteMsg(&response)
			}()
		}
	}()

	_, port, err := net.SplitHostPort(address)
	require.NoError(tb, err)
	return port
}

func newLocalUDPClient(port string) *client {
	return &client{
		host:    "127.0.0.1",
		port:    port,
		dialer:  newDNSDialer(nil, nil, ""),
		network: "udp",
		addr:    net.JoinHostPort("127.0.0.1", port),
		dClient: D.Client{UDPSize: 4096, Timeout: 5 * time.Second},
	}
}

func newClientTestQuery() *D.Msg {
	query := &D.Msg{}
	query.SetQuestion("example.com.", D.TypeA)
	return query
}

func TestClientExchangeUDP(t *testing.T) {
	port, received := startUDPDNSServer(t)
	response, err := newLocalUDPClient(port).ExchangeContext(context.Background(), newClientTestQuery())
	require.NoError(t, err)
	require.Len(t, response.Answer, 1)
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("DNS server did not receive the query")
	}
}

func TestClientExchangeUDPDeadline(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer conn.Close()

	_, port, err := net.SplitHostPort(conn.LocalAddr().String())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err = newLocalUDPClient(port).ExchangeContext(ctx, newClientTestQuery())
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestClientExchangeRetriesTruncatedUDPOverTCP(t *testing.T) {
	port := startTruncatedDNSServers(t)
	response, err := newLocalUDPClient(port).ExchangeContext(context.Background(), newClientTestQuery())
	require.NoError(t, err)
	require.Len(t, response.Answer, 1)
	answer, ok := response.Answer[0].(*D.A)
	require.True(t, ok)
	require.Equal(t, net.IPv4(192, 0, 2, 11).To4(), answer.A.To4())
}

func BenchmarkClientExchangeUDP(b *testing.B) {
	port, _ := startUDPDNSServer(b)
	dnsClient := newLocalUDPClient(port)
	query := newClientTestQuery()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dnsClient.ExchangeContext(ctx, query); err != nil {
			b.Fatal(err)
		}
	}
}
