package outboundgroup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"

	"github.com/stretchr/testify/require"
)

func TestURLTestLeavesAnUnmeasuredPickOnceAMemberIsMeasured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	first, measured := namedProxy("first"), namedProxy("measured")
	group, err := NewURLTest(
		GroupCommonOption{Name: "auto", URL: server.URL},
		URLTestOption{},
		namedProxy("COMPATIBLE"),
		[]P.ProxyProvider{&fakeProvider{name: "p", proxies: []C.Proxy{first, measured}}},
	)
	require.NoError(t, err)
	require.Equal(t, "first", group.Now())

	_, err = measured.URLTest(context.Background(), server.URL, nil)
	require.NoError(t, err)

	require.Equal(t, "first", group.Now())

	time.Sleep(unmeasuredPickHold)
	require.Equal(t, "measured", group.Now())
}
