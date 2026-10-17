package statistic

import (
	"io"
	"net"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
)

func benchmarkMetadata() *C.Metadata {
	return &C.Metadata{
		NetWork: C.TCP,
		Type:    C.HTTP,
		SrcIP:   netip.MustParseAddr("127.0.0.1"),
		SrcPort: 12345,
		DstIP:   netip.MustParseAddr("127.0.0.1"),
		DstPort: 443,
		Host:    "benchmark.example.com",
	}
}

func newBenchmarkTracker(b *testing.B) (*tcpTracker, net.Conn) {
	b.Helper()

	local, remote := net.Pipe()
	tracker := NewTCPTracker(
		newStatisticTestConn(local),
		&Manager{},
		benchmarkMetadata(),
		nil,
		0,
		0,
		false,
	)
	b.Cleanup(func() {
		_ = tracker.Close()
		_ = remote.Close()
	})
	return tracker, remote
}

func BenchmarkTrackerJoinLeave(b *testing.B) {
	tracker, _ := newBenchmarkTracker(b)
	manager := tracker.manager

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.Join(tracker)
		manager.Leave(tracker)
	}
}

func BenchmarkTrackerWrite(b *testing.B) {
	tracker, remote := newBenchmarkTracker(b)
	payload := make([]byte, 16*1024)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		_, _ = io.Copy(io.Discard, remote)
	}()

	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := tracker.Write(payload); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	_ = tracker.Close()
	<-readerDone
}

func BenchmarkTrackerRead(b *testing.B) {
	tracker, remote := newBenchmarkTracker(b)
	payload := make([]byte, 16*1024)
	stop := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := remote.Write(payload); err != nil {
				return
			}
		}
	}()

	buffer := make([]byte, len(payload))
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := tracker.Read(buffer); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	close(stop)
	_ = tracker.Close()
	<-writerDone
}
