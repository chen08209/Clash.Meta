package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
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

	idleMu sync.Mutex
	idle   []net.Conn
}

var _ dnsClient = (*client)(nil)

// An exchange owns its conn exclusively, so a pooled conn never carries two
// in-flight queries; concurrent queries that find the pool empty simply dial.
const maxIdleDNSConns = 4

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
	conn, err := c.takeConn(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	dClient := &D.Client{
		UDPSize: 4096,
		Timeout: 5 * time.Second,
	}

	msg, err := exchangeWithConn(ctx, dClient, m, conn)

	// Resolvers MUST resend queries over TCP if they receive a truncated UDP response (with TC=1 set)!
	if msg != nil && msg.Truncated && network == "udp" {
		// Truncation is a message property, not a conn failure; the udp conn
		// returns to the pool before the tcp retry.
		c.keepConn(network, conn)
		network = "tcp"
		if log.DebugEnabled() {
			log.Debugln("[DNS] Truncated reply from %s:%s for %s over UDP, retrying over TCP", c.host, c.port, m.Question[0].String())
		}
		tcpConn, dialErr := c.dialer.DialContext(ctx, network, addr)
		if dialErr != nil {
			return msg, dialErr
		}
		msg, err = exchangeWithConn(ctx, dClient, m, tcpConn)
		_ = tcpConn.Close()
		return msg, err
	}

	if err != nil {
		_ = conn.Close()
		return msg, err
	}
	c.keepConn(network, conn)
	return msg, err
}

// takeConn hands out an idle udp conn when one exists; tcp conns are not
// pooled because their lifetime through proxies is not worth the reuse risk.
func (c *client) takeConn(ctx context.Context, network, addr string) (net.Conn, error) {
	if network == "udp" {
		c.idleMu.Lock()
		if n := len(c.idle); n > 0 {
			conn := c.idle[n-1]
			c.idle = c.idle[:n-1]
			c.idleMu.Unlock()
			return conn, nil
		}
		c.idleMu.Unlock()
	}
	return c.dialer.DialContext(ctx, network, addr)
}

func (c *client) keepConn(network string, conn net.Conn) {
	if network != "udp" {
		_ = conn.Close()
		return
	}
	c.idleMu.Lock()
	if len(c.idle) < maxIdleDNSConns {
		c.idle = append(c.idle, conn)
		c.idleMu.Unlock()
		return
	}
	c.idleMu.Unlock()
	_ = conn.Close()
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

func (c *client) ResetConnection() {
	c.idleMu.Lock()
	idle := c.idle
	c.idle = nil
	c.idleMu.Unlock()
	for _, conn := range idle {
		_ = conn.Close()
	}
}

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
