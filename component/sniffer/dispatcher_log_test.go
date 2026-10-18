package sniffer

import (
	"net/netip"
	"testing"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTCPSniffOutcomeIsLevelIndependent(t *testing.T) {
	oldLevel := log.Level()
	defer log.SetLevel(oldLevel)

	for _, level := range []log.LogLevel{log.DEBUG, log.INFO, log.SILENT} {
		t.Run(level.String(), func(t *testing.T) {
			log.SetLevel(level)

			successful := &stubSniffer{reply: func([]byte) (string, error) {
				return "example.com", nil
			}}
			sd, err := NewDispatcher(&Config{Enable: true, ParsePureIp: true})
			require.NoError(t, err)
			sd.sniffers = []configuredSniffer{
				{Sniffer: successful, config: SnifferConfig{OverrideDest: true}},
			}
			raw := &chunkedConn{t: t, chunks: [][]byte{[]byte("0123456789")}}
			t.Cleanup(raw.stopTimer)
			metadata := &C.Metadata{
				NetWork: C.TCP,
				Process: "curl",
				Uid:     1000,
				SrcIP:   netip.MustParseAddr("192.168.1.100"),
				SrcPort: 54321,
				DstIP:   netip.MustParseAddr("192.0.2.1"),
				DstPort: 80,
			}

			sniffed := sd.TCPSniff(N.NewBufferedConn(raw), metadata)

			require.True(t, sniffed)
			assert.Equal(t, "example.com", metadata.SniffHost)
			assert.Equal(t, "example.com", metadata.Host)
			assert.False(t, metadata.DstIP.IsValid())
		})
	}
}

func benchmarkSniffMetadata() *C.Metadata {
	return &C.Metadata{
		NetWork: C.TCP,
		Process: "curl",
		Uid:     1000,
		SrcIP:   netip.MustParseAddr("192.168.1.100"),
		SrcPort: 54321,
		DstIP:   netip.MustParseAddr("93.184.216.34"),
		DstPort: 443,
		Host:    "example.com",
	}
}

var sniffLogSink string

func BenchmarkSniffSuccessLogArguments(b *testing.B) {
	metadata := benchmarkSniffMetadata()

	b.Run("Unconditional", func(b *testing.B) {
		var sink string
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			sink = metadata.SourceDetail() + metadata.RemoteAddress() + metadata.String()
		}
		sniffLogSink = sink
	})
	b.Run("Guarded", func(b *testing.B) {
		var sink string
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if log.DebugEnabled() {
				sink = metadata.SourceDetail() + metadata.RemoteAddress() + metadata.String()
			}
		}
		sniffLogSink = sink
	})
}

func BenchmarkSniffFailureLogArguments(b *testing.B) {
	metadata := benchmarkSniffMetadata()

	b.Run("Unconditional", func(b *testing.B) {
		var sink string
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			sink = metadata.String()
		}
		sniffLogSink = sink
	})
	b.Run("Guarded", func(b *testing.B) {
		var sink string
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if log.DebugEnabled() {
				sink = metadata.String()
			}
		}
		sniffLogSink = sink
	})
}
