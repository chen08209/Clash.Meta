package sniffer

import (
	"errors"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type surfaceUDPPacket struct {
	*fakeUDPPacket
	metadata *C.Metadata
}

func (s *surfaceUDPPacket) Metadata() *C.Metadata { return s.metadata }
func (s *surfaceUDPPacket) Key() string           { return "" }
func TestNeedAtLeastDataError(test *testing.T) {
	err := needAtLeast(42)
	assert.Contains(test, err.Error(), "42")
	assert.True(test, errors.Is(err, ErrNoClue))

	wrapped := errors.Join(errors.New("outer"), err)
	var need *errNeedAtLeastData
	require.True(test, errors.As(wrapped, &need))
	assert.Equal(test, 42, need.length)
}

func TestSnifferNetworkAndPortSupport(test *testing.T) {
	tests := []struct {
		name    string
		sniffer func() (interface{ SupportNetwork() C.NetWork }, error)
		network C.NetWork
		port    bool
	}{
		{"tls", func() (interface{ SupportNetwork() C.NetWork }, error) {
			s, err := NewTLSSniffer(SnifferConfig{})
			return s, err
		}, C.TCP, true},
		{"http", func() (interface{ SupportNetwork() C.NetWork }, error) {
			s, err := NewHTTPSniffer(SnifferConfig{})
			return s, err
		}, C.TCP, true},
		{"quic", func() (interface{ SupportNetwork() C.NetWork }, error) {
			s, err := NewQUICSniffer(SnifferConfig{})
			return s, err
		}, C.UDP, true},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			sniffer, err := testCase.sniffer()
			require.NoError(test, err)
			assert.Equal(test, testCase.network, sniffer.SupportNetwork())
		})
	}

	// the default port list is the protocol port
	tls, err := NewTLSSniffer(SnifferConfig{})
	require.NoError(test, err)
	assert.True(test, tls.SupportPort(443))
	assert.False(test, tls.SupportPort(80))
}

func TestTLSSnifferSniffData(test *testing.T) {
	tls, err := NewTLSSniffer(SnifferConfig{})
	require.NoError(test, err)

	record := makeTestTLSRecords(makeTestClientHello("sniffed.example", 0))
	domain, err := tls.SniffData(record)
	require.NoError(test, err)
	assert.Equal(test, "sniffed.example", domain)

	// a plain http request is not TLS and passes the error through
	_, err = tls.SniffData([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	require.Error(test, err)
}

func TestDispatcherEnable(test *testing.T) {
	var nilDispatcher *Dispatcher
	assert.False(test, nilDispatcher.Enable())

	disabled, err := NewDispatcher(&Config{})
	require.NoError(test, err)
	assert.False(test, disabled.Enable())

	enabled, err := NewDispatcher(&Config{Enable: true})
	require.NoError(test, err)
	assert.True(test, enabled.Enable())
}

func TestDispatcherDomainCanReplace(test *testing.T) {
	sd := &Dispatcher{
		skipDomain: []C.DomainMatcher{
			domainMatcherFunc(func(domain string) bool { return domain == "skip.me" }),
		},
	}
	assert.False(test, sd.domainCanReplace("skip.me"))
	assert.True(test, sd.domainCanReplace("ok.example"))
	assert.False(test, sd.domainCanReplace("."))
	assert.False(test, sd.domainCanReplace("not a domain"))
}

func TestDispatcherUDPSniffSinglePacket(test *testing.T) {
	newStub := func() *stubSniffer {
		return &stubSniffer{
			network: C.UDP,
			reply:   func([]byte) (string, error) { return "udp.example", nil },
		}
	}
	packetData := []byte("quic-initial")

	// without the override the sniffed name is recorded but the destination
	// stays as it was dialed
	stub := newStub()
	sd := &Dispatcher{
		sniffers:    []configuredSniffer{{Sniffer: stub, config: SnifferConfig{}}},
		parsePureIp: true,
	}
	metadata := &C.Metadata{
		NetWork: C.UDP,
		DstIP:   netip.MustParseAddr("192.0.2.1"),
		DstPort: 443,
	}
	sender := &fakeSender{}
	returned := sd.UDPSniff(&surfaceUDPPacket{
		fakeUDPPacket: &fakeUDPPacket{data: packetData, data2: packetData},
		metadata:      metadata,
	}, sender)

	assert.Same(test, sender, returned, "a single-packet sniff keeps the sender")
	assert.Equal(test, "udp.example", metadata.SniffHost)
	assert.Empty(test, metadata.Host, "the destination is not replaced without the override")
	assert.True(test, metadata.DstIP.IsValid())
	assert.Equal(test, C.DNSNormal, metadata.DNSMode)
	assert.Equal(test, []int{len(packetData)}, stub.seen)

	// with the override the sniffed name becomes the destination
	stub = newStub()
	sd = &Dispatcher{
		sniffers: []configuredSniffer{
			{Sniffer: stub, config: SnifferConfig{OverrideDest: true}},
		},
		parsePureIp: true,
	}
	metadata = &C.Metadata{
		NetWork: C.UDP,
		DstIP:   netip.MustParseAddr("192.0.2.1"),
		DstPort: 443,
	}
	returned = sd.UDPSniff(&surfaceUDPPacket{
		fakeUDPPacket: &fakeUDPPacket{data: packetData, data2: packetData},
		metadata:      metadata,
	}, sender)

	assert.Same(test, sender, returned)
	assert.Equal(test, "udp.example", metadata.SniffHost)
	assert.Equal(test, "udp.example", metadata.Host)
	assert.False(test, metadata.DstIP.IsValid())
	assert.Equal(test, C.DNSNormal, metadata.DNSMode)
}
func TestDispatcherUDPSniffSkipsWithoutOverride(test *testing.T) {
	stub := &stubSniffer{
		network: C.UDP,
		reply:   func([]byte) (string, error) { return "udp.example", nil },
	}
	sd := &Dispatcher{
		sniffers:    []configuredSniffer{{Sniffer: stub}},
		parsePureIp: false,
	}

	metadata := &C.Metadata{NetWork: C.UDP, DstPort: 443}
	packet := &surfaceUDPPacket{
		fakeUDPPacket: &fakeUDPPacket{data: []byte("quic-initial"), data2: []byte("quic-initial")},
		metadata:      metadata,
	}
	sender := &fakeSender{}

	returned := sd.UDPSniff(packet, sender)
	assert.Same(test, sender, returned)
	assert.Empty(test, metadata.SniffHost, "no override policy means no sniff")
}
