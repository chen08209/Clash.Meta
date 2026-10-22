package dns

import (
	"context"
	"net"
	"net/netip"
	"testing"

	D "github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingClient struct {
	calls   int
	queries []*D.Msg
	reply   *D.Msg
}

func (c *recordingClient) ExchangeContext(ctx context.Context, m *D.Msg) (*D.Msg, error) {
	c.calls++
	c.queries = append(c.queries, m)
	if c.reply != nil {
		return c.reply, nil
	}
	return m, nil
}

func (c *recordingClient) Address() string  { return "recording-client" }
func (c *recordingClient) ResetConnection() {}

func questionMsg(name string, qType uint16) *D.Msg {
	m := &D.Msg{}
	m.SetQuestion(name, qType)
	return m
}

func TestRCodeClientAnswersWithTheMappedCode(test *testing.T) {
	tests := []struct {
		addr  string
		rcode int
	}{
		{"success", D.RcodeSuccess},
		{"format_error", D.RcodeFormatError},
		{"server_failure", D.RcodeServerFailure},
		{"name_error", D.RcodeNameError},
		{"not_implemented", D.RcodeNotImplemented},
		{"refused", D.RcodeRefused},
	}
	for _, testCase := range tests {
		test.Run(testCase.addr, func(test *testing.T) {
			client := newRCodeClient(testCase.addr)
			assert.Equal(test, "rcode://"+testCase.addr, client.Address())

			query := questionMsg("example.com", D.TypeA)
			msg, err := client.ExchangeContext(context.Background(), query)
			require.NoError(test, err)
			assert.True(test, msg.Response)
			assert.Equal(test, testCase.rcode, msg.Rcode)
			client.ResetConnection()
		})
	}

	assert.Panics(test, func() { newRCodeClient("bogus") })
}

func TestWrapClientWithDisableTypesFilters(test *testing.T) {
	inner := &recordingClient{}
	same := wrapClientWithDisableTypes(inner, nil)
	assert.Equal(test, inner, same)

	wrapped := wrapClientWithDisableTypes(inner, map[string]string{
		"disable-ipv6":        "true",
		"disable-qtype-65":    "true",
		"disable-qtype-99999": "true",
		"disable-qtype-bad":   "true",
	})

	// the disabled question and the disabled qtype never reach the inner client
	msg, err := wrapped.ExchangeContext(context.Background(), questionMsg("example.com", D.TypeAAAA))
	require.NoError(test, err)
	assert.Equal(test, D.RcodeSuccess, msg.Rcode)
	assert.Empty(test, msg.Answer)
	assert.Equal(test, 0, inner.calls)

	msg, err = wrapped.ExchangeContext(context.Background(), questionMsg("example.com", D.TypeHTTPS))
	require.NoError(test, err)
	assert.Equal(test, D.RcodeSuccess, msg.Rcode)
	assert.Equal(test, 0, inner.calls)

	// an untouched question passes through
	msg, err = wrapped.ExchangeContext(context.Background(), questionMsg("example.com", D.TypeMX))
	require.NoError(test, err)
	assert.Equal(test, 1, inner.calls)

	// the response loses the disabled type in every section
	reply := &D.Msg{}
	reply.Answer = []D.RR{
		&D.A{Hdr: D.RR_Header{Rrtype: D.TypeA}, A: net.ParseIP("1.2.3.4")},
		&D.AAAA{Hdr: D.RR_Header{Rrtype: D.TypeAAAA}, AAAA: net.ParseIP("::1")},
		&D.MX{Hdr: D.RR_Header{Rrtype: D.TypeMX}},
	}
	reply.Ns = []D.RR{&D.AAAA{Hdr: D.RR_Header{Rrtype: D.TypeAAAA}, AAAA: net.ParseIP("::2")}}
	reply.Extra = []D.RR{&D.AAAA{Hdr: D.RR_Header{Rrtype: D.TypeAAAA}, AAAA: net.ParseIP("::3")}}
	inner.reply = reply

	_, err = wrapped.ExchangeContext(context.Background(), questionMsg("example.com", D.TypeMX))
	require.NoError(test, err)
	assert.Len(test, reply.Answer, 2)
	assert.Empty(test, reply.Ns)
	assert.Empty(test, reply.Extra)

	typed, ok := wrapped.(clientWithDisableTypes)
	require.True(test, ok)
	assert.Equal(test, inner, typed.Unwrap())
}
func TestDisableTypesParams(test *testing.T) {
	for _, key := range []string{"disable-ipv4", "disable-ipv6", "disable-qtype-65"} {
		assert.True(test, isDisableTypesParam(key), key)
		assert.True(test, isWrapperOnlyParam(key), key)
	}
	for _, key := range []string{"ecs", "ecs-override"} {
		assert.True(test, isEdns0SubnetParam(key), key)
		assert.True(test, isWrapperOnlyParam(key), key)
	}
	for _, key := range []string{"url", "timeout", "h3"} {
		assert.False(test, isWrapperOnlyParam(key), key)
	}
}

func TestSetEdns0Subnet(test *testing.T) {
	msg := questionMsg("example.com", D.TypeA)
	require.True(test, setEdns0Subnet(msg, netip.MustParsePrefix("1.2.3.0/24"), false))
	require.Len(test, msg.Extra, 1)
	opt, ok := msg.Extra[0].(*D.OPT)
	require.True(test, ok)
	require.Len(test, opt.Option, 1)
	subnet, ok := opt.Option[0].(*D.EDNS0_SUBNET)
	require.True(test, ok)
	assert.Equal(test, uint16(1), subnet.Family)
	assert.Equal(test, uint8(24), subnet.SourceNetmask)
	assert.Equal(test, net.ParseIP("1.2.3.0").To4(), subnet.Address)

	// an existing subnet option is left alone unless the override is set
	assert.False(test, setEdns0Subnet(msg, netip.MustParsePrefix("5.6.7.0/16"), false))
	assert.Equal(test, uint8(24), subnet.SourceNetmask)

	require.True(test, setEdns0Subnet(msg, netip.MustParsePrefix("5.6.7.0/16"), true))
	assert.Equal(test, uint8(16), subnet.SourceNetmask)
	assert.Equal(test, net.ParseIP("5.6.7.0").To4(), subnet.Address)

	v6 := questionMsg("example.com", D.TypeAAAA)
	require.True(test, setEdns0Subnet(v6, netip.MustParsePrefix("2001:db8::/48"), false))
	v6Subnet, ok := v6.Extra[0].(*D.OPT)
	require.True(test, ok)
	v6Option, ok := v6Subnet.Option[0].(*D.EDNS0_SUBNET)
	require.True(test, ok)
	assert.Equal(test, uint16(2), v6Option.Family)
	assert.Equal(test, uint8(48), v6Option.SourceNetmask)
}

func TestWrapClientWithEdns0Subnet(test *testing.T) {
	inner := &recordingClient{}
	assert.Equal(test, inner, wrapClientWithEdns0Subnet(inner, map[string]string{}))

	wrapped := wrapClientWithEdns0Subnet(inner, map[string]string{"ecs": "1.2.3.0/24"})
	typed, ok := wrapped.(clientWithEdns0Subnet)
	require.True(test, ok)
	assert.Equal(test, inner, typed.Unwrap())

	query := questionMsg("example.com", D.TypeA)
	_, err := wrapped.ExchangeContext(context.Background(), query)
	require.NoError(test, err)
	require.Len(test, inner.queries, 1)
	// the wrapper sends a copy, so the caller's message is untouched
	assert.Empty(test, query.Extra)

	sent := inner.queries[0]
	require.Len(test, sent.Extra, 1)
	opt, ok := sent.Extra[0].(*D.OPT)
	require.True(test, ok)
	subnet, ok := opt.Option[0].(*D.EDNS0_SUBNET)
	require.True(test, ok)
	assert.Equal(test, uint8(24), subnet.SourceNetmask)

	// a bare address is taken as a full-length prefix
	wrapped = wrapClientWithEdns0Subnet(inner, map[string]string{"ecs": "1.2.3.4"})
	_, err = wrapped.ExchangeContext(context.Background(), questionMsg("example.com", D.TypeA))
	require.NoError(test, err)
	sent = inner.queries[1]
	require.Len(test, sent.Extra, 1)
	opt, ok = sent.Extra[0].(*D.OPT)
	require.True(test, ok)
	subnet, ok = opt.Option[0].(*D.EDNS0_SUBNET)
	require.True(test, ok)
	assert.Equal(test, uint8(32), subnet.SourceNetmask)
}

func TestMsgToLogStringShapes(test *testing.T) {
	a := questionMsg("example.com", D.TypeA)
	a.Answer = []D.RR{
		&D.A{Hdr: D.RR_Header{Rrtype: D.TypeA}, A: net.ParseIP("1.2.3.4")},
		&D.A{Hdr: D.RR_Header{Rrtype: D.TypeA}, A: net.ParseIP("5.6.7.8")},
	}
	assert.Equal(test, "[1.2.3.4 5.6.7.8] A", msgToLogString(a))

	qType, qTypeStr := msgToQtype(a)
	assert.Equal(test, D.TypeA, qType)
	assert.Equal(test, "A", qTypeStr)

	https := questionMsg("example.com", D.TypeHTTPS)
	assert.Equal(test, "[] HTTPS", msgToLogString(https))

	https.Answer = []D.RR{
		&D.HTTPS{SVCB: D.SVCB{
			Hdr:   D.RR_Header{Rrtype: D.TypeHTTPS},
			Value: []D.SVCBKeyValue{&D.SVCBAlpn{Alpn: []string{"h2", "h3"}}, &D.SVCBIPv4Hint{Hint: []net.IP{net.ParseIP("1.2.3.4")}}},
		}},
	}
	assert.Equal(test, "[alpn:h2,h3;ipv4hint] HTTPS", msgToLogString(https))

	https.Answer = append(https.Answer, &D.HTTPS{SVCB: D.SVCB{
		Hdr:   D.RR_Header{Rrtype: D.TypeHTTPS},
		Value: []D.SVCBKeyValue{&D.SVCBAlpn{Alpn: []string{"h3"}}, &D.SVCBIPv6Hint{Hint: []net.IP{net.ParseIP("::1")}}},
	}})
	assert.Contains(test, msgToLogString(https), "ipv6hint")

	empty := &D.Msg{}
	qType, qTypeStr = msgToQtype(empty)
	assert.Equal(test, uint16(0), qType)
	assert.Empty(test, qTypeStr)
	assert.Equal(test, "[] ", msgToLogString(empty))
}
