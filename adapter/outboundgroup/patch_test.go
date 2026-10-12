package outboundgroup

import (
	"testing"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"

	"github.com/stretchr/testify/require"
)

type fakeProvider struct {
	P.ProxyProvider
	name    string
	proxies []C.Proxy
}

func (f *fakeProvider) Name() string               { return f.name }
func (f *fakeProvider) VehicleType() P.VehicleType { return P.File }
func (f *fakeProvider) Proxies() []C.Proxy         { return f.proxies }
func (f *fakeProvider) Version() uint32            { return 1 }
func (f *fakeProvider) Touch()                     {}
func (f *fakeProvider) HealthCheckURL() string     { return "" }

func namedProxy(name string) C.Proxy {
	return adapter.NewProxy(outbound.NewDirectWithOption(outbound.DirectOption{Name: name}))
}

func parseMembers(t *testing.T, group map[string]any) []string {
	t.Helper()
	proxyMap := map[string]C.Proxy{}
	for _, name := range []string{"COMPATIBLE", "A", "B"} {
		proxyMap[name] = namedProxy(name)
	}
	providersMap := map[string]P.ProxyProvider{
		"p1": &fakeProvider{name: "p1", proxies: []C.Proxy{namedProxy("p1-hk"), namedProxy("p1-us")}},
		"p2": &fakeProvider{name: "p2", proxies: []C.Proxy{namedProxy("p2-hk")}},
	}
	config := map[string]any{
		"name":    "Group",
		"type":    "select",
		"proxies": []any{"A", "B"},
		"use":     []any{"p1", "p2"},
	}
	for key, value := range group {
		config[key] = value
	}
	parsed, err := ParseProxyGroup(config, proxyMap, providersMap, nil, nil)
	require.NoError(t, err)
	var names []string
	for _, proxy := range parsed.Proxies() {
		names = append(names, proxy.Name())
	}
	return names
}

func TestUsePositionListsProvidersBetweenProxies(t *testing.T) {
	for _, tc := range []struct {
		name  string
		group map[string]any
		want  []string
	}{
		{
			name: "upstream order without use-position",
			want: []string{"A", "B", "p1-hk", "p1-us", "p2-hk"},
		},
		{
			name:  "each provider after as many proxies as it names",
			group: map[string]any{"use-position": map[string]any{"p1": 1, "p2": 0}},
			want:  []string{"p2-hk", "A", "p1-hk", "p1-us", "B"},
		},
		{
			name:  "providers sharing a position keep the order of use",
			group: map[string]any{"use-position": map[string]any{"p2": 1, "p1": 1}},
			want:  []string{"A", "p1-hk", "p1-us", "p2-hk", "B"},
		},
		{
			name:  "a missing, stale or out of range position goes last",
			group: map[string]any{"use-position": map[string]any{"p1": 9, "p3": 0}},
			want:  []string{"A", "B", "p1-hk", "p1-us", "p2-hk"},
		},
		{
			name:  "a negative position goes first",
			group: map[string]any{"use-position": map[string]any{"p2": -1}},
			want:  []string{"p2-hk", "A", "B", "p1-hk", "p1-us"},
		},
		{
			name: "a filter still reaches only the providers",
			group: map[string]any{
				"use-position": map[string]any{"p1": 1},
				"filter":       "hk",
			},
			want: []string{"A", "p1-hk", "B", "p2-hk"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, parseMembers(t, tc.group))
		})
	}
}
