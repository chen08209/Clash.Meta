package tunnel

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
)

func TestPreHandleMetadataEmptyHostDoesNotAllocate(t *testing.T) {
	metadata := &C.Metadata{DstIP: netip.MustParseAddr("127.0.0.1")}

	allocations := testing.AllocsPerRun(100, func() {
		if err := preHandleMetadata(metadata); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("empty-host prehandle allocations = %v, want 0", allocations)
	}
}

func BenchmarkPreHandleMetadataEmptyHost(b *testing.B) {
	metadata := &C.Metadata{DstIP: netip.MustParseAddr("127.0.0.1")}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := preHandleMetadata(metadata); err != nil {
			b.Fatal(err)
		}
	}
}
