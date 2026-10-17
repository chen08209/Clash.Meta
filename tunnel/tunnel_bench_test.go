package tunnel

import (
	"fmt"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
	"github.com/metacubex/mihomo/rules/wrapper"
	"github.com/stretchr/testify/require"
)

func benchmarkMetadata() C.Metadata {
	return C.Metadata{
		NetWork: C.TCP,
		Type:    C.HTTP,
		Host:    "benchmark.example.com",
		DstPort: 443,
	}
}

func benchmarkResolveRuntime(tb testing.TB) {
	tb.Helper()

	oldRuntime := loadRuntime()

	configMux.Lock()
	currentRuntime.Store(
		&runtimeSnapshot{
			proxies: map[string]C.Proxy{"DIRECT": directTestProxy{}},
			rules: []C.Rule{
				wrapper.NewRuleWrapper(
					rulesCommon.NewDomain("benchmark.example.com", "DIRECT"),
				),
			},
			mode: Rule,
		},
	)
	configMux.Unlock()

	tb.Cleanup(func() {
		configMux.Lock()
		currentRuntime.Store(oldRuntime)
		invalidateAllProxies()
		configMux.Unlock()
	})
}

func TestResolveMetadataPureDomainRuleStaysAllocationLight(t *testing.T) {
	benchmarkResolveRuntime(t)

	var proxy C.Proxy
	var err error
	allocations := testing.AllocsPerRun(100, func() {
		metadata := benchmarkMetadata()
		proxy, _, err = resolveMetadata(&metadata)
	})
	require.NoError(t, err)
	require.NotNil(t, proxy)
	require.Equal(t, "DIRECT", proxy.Name())
	require.LessOrEqual(t, allocations, float64(1))
}

func BenchmarkResolveMetadata(b *testing.B) {
	benchmarkResolveRuntime(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metadata := benchmarkMetadata()
		proxy, _, err := resolveMetadata(&metadata)
		if err != nil {
			b.Fatal(err)
		}
		if proxy == nil || proxy.Name() != "DIRECT" {
			b.Fatalf("proxy = %v, want DIRECT", proxy)
		}
	}
}

func BenchmarkResolveMetadataFallback(b *testing.B) {
	benchmarkResolveRuntime(b)
	configMux.Lock()
	next := cloneRuntime(loadRuntime())
	next.rules = nil
	currentRuntime.Store(next)
	configMux.Unlock()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metadata := benchmarkMetadata()
		metadata.Host = fmt.Sprintf("miss-%d.example.com", i)
		proxy, _, err := resolveMetadata(&metadata)
		if err != nil {
			b.Fatal(err)
		}
		if proxy == nil || proxy.Name() != "DIRECT" {
			b.Fatalf("proxy = %v, want DIRECT", proxy)
		}
	}
}
