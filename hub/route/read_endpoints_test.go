package route

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRouterProviderAndGroupReadEndpoints(test *testing.T) {
	base := newTestServer(test, "")
	for _, testCase := range []struct {
		path string
		key  string
	}{
		{"/providers/proxies", "providers"},
		{"/providers/rules", "providers"},
		{"/group", "proxies"},
	} {
		status, body := doGet(test, base, testCase.path, "")
		require.Equal(test, http.StatusOK, status, testCase.path)
		require.Contains(test, body, testCase.key, testCase.path)
	}
}

func TestRouterUnknownProxyAndProviderPaths(test *testing.T) {
	base := newTestServer(test, "")
	for _, path := range []string{
		"/proxies/absent-proxy",
		"/proxies/absent-proxy/delay?url=http://127.0.0.1",
		"/providers/proxies/absent-provider",
		"/providers/proxies/absent-provider/healthcheck",
		"/group/absent-group",
	} {
		status, body := doGet(test, base, path, "")
		require.Equal(test, http.StatusNotFound, status, path)
		require.Contains(test, body, "Resource not found", path)
	}
}

func TestRouterCloseUnknownConnection(test *testing.T) {
	base := newTestServer(test, "")
	req, err := http.NewRequest(http.MethodDelete, base+"/connections/absent-connection", nil)
	require.NoError(test, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(test, err)
	defer resp.Body.Close()
	require.Equal(test, http.StatusNoContent, resp.StatusCode)
}
