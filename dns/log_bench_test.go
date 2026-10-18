package dns

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/metacubex/mihomo/log"
	D "github.com/miekg/dns"
)

var logSink string

func benchmarkDNSLogMsg() *D.Msg {
	msg := &D.Msg{}
	msg.SetQuestion(D.Fqdn("www.example.com"), D.TypeA)
	for _, ip := range []string{"192.0.2.10", "192.0.2.11", "198.51.100.5"} {
		msg.Answer = append(msg.Answer, &D.A{
			Hdr: D.RR_Header{Name: "www.example.com.", Rrtype: D.TypeA, Class: D.ClassINET, Ttl: 60},
			A:   net.ParseIP(ip),
		})
	}
	return msg
}

func BenchmarkDNSCacheHitLogArguments(b *testing.B) {
	msg := benchmarkDNSLogMsg()
	expire := time.Now().Add(time.Minute)

	b.Run("Unconditional", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = fmt.Sprintf("[DNS] cache hit %s --> %s, expire at %s", "www.example.com", msgToLogString(msg), expire.Format("2006-01-02 15:04:05"))
		}
	})
	b.Run("Guarded", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if log.DebugEnabled() {
				_ = fmt.Sprintf("[DNS] cache hit %s --> %s, expire at %s", "www.example.com", msgToLogString(msg), expire.Format("2006-01-02 15:04:05"))
			}
		}
	})
}

func BenchmarkDNSExchangeLogArguments(b *testing.B) {
	msg := benchmarkDNSLogMsg()

	b.Run("Unconditional", func(b *testing.B) {
		var sink string
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			sink = msgToDomain(msg)
			_, sink = msgToQtype(msg)
		}
		logSink = sink
	})
	b.Run("Guarded", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if log.DebugEnabled() {
				_ = msgToDomain(msg)
				_, _ = msgToQtype(msg)
			}
		}
	})
}
