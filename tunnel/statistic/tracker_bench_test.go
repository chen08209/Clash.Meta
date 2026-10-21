package statistic

import (
	"net"
	"testing"
)

func BenchmarkNewTCPTracker(b *testing.B) {
	manager := newStripedManager()
	local, remote := net.Pipe()
	b.Cleanup(func() { _ = remote.Close() })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker := NewTCPTracker(newStatisticTestConn(local), manager, benchmarkMetadata(), nil, 0, 0, true)
		manager.Leave(tracker)
	}
}
