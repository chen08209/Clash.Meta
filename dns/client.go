package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/metacubex/mihomo/common/contextutils"
	"github.com/metacubex/mihomo/component/resolver"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"

	D "github.com/miekg/dns"
)

type client struct {
	port   string
	host   string
	dialer *dnsDialer
	schema string
}

var _ dnsClient = (*client)(nil)

// Address implements dnsClient
func (c *client) Address() string {
	return fmt.Sprintf("%s://%s", c.schema, net.JoinHostPort(c.host, c.port))
}

func (c *client) ExchangeContext(ctx context.Context, m *D.Msg) (*D.Msg, error) {
	network := "udp"
	if c.schema != "udp" {
		network = "tcp"
	}

	addr := net.JoinHostPort(c.host, c.port)
	conn, err := c.dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	dClient := &D.Client{
		UDPSize: 4096,
		Timeout: 5 * time.Second,
	}

	msg, err := exchangeWithConn(ctx, dClient, m, conn)

	// Resolvers MUST resend queries over TCP if they receive a truncated UDP response (with TC=1 set)!
	if msg != nil && msg.Truncated && network == "udp" {
		network = "tcp"
		if log.DebugEnabled() {
			log.Debugln("[DNS] Truncated reply from %s:%s for %s over UDP, retrying over TCP", c.host, c.port, m.Question[0].String())
		}
		tcpConn, dialErr := c.dialer.DialContext(ctx, network, addr)
		if dialErr != nil {
			return msg, dialErr
		}
		defer tcpConn.Close()
		msg, err = exchangeWithConn(ctx, dClient, m, tcpConn)
	}

	return msg, err
}

func exchangeWithConn(ctx context.Context, dClient *D.Client, m *D.Msg, conn net.Conn) (*D.Msg, error) {
	dConn := &D.Conn{
		Conn:    conn,
		UDPSize: dClient.UDPSize,
	}
	stop := contextutils.AfterFunc(ctx, func() {
		_ = conn.Close()
	})
	defer stop()

	msg, _, err := dClient.ExchangeWithConnContext(ctx, m, dConn)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		// the socket deadline taken from the context deadline can fire before
		// the context timer callback has run, and ctx.Err() would still be
		// nil; a passed deadline is the cause of a read timeout regardless of
		// which timer was observed first
		if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
			return nil, context.DeadlineExceeded
		}
	}
	return msg, err
}

func (c *client) ResetConnection() {}

func newClient(addr string, resolver resolver.Resolver, netType string, params map[string]string, proxyAdapter C.ProxyAdapter, proxyName string) *client {
	host, port, _ := net.SplitHostPort(addr)
	c := &client{
		port:   port,
		host:   host,
		dialer: newDNSDialer(resolver, proxyAdapter, proxyName),
		schema: "udp",
	}
	if strings.HasPrefix(netType, "tcp") {
		c.schema = "tcp"
	}
	return c
}
