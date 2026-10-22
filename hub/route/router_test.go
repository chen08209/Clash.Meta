package route

import (
	"io"
	"net"
	"net/http"
	"testing"

	mhttp "github.com/metacubex/http"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T, secret string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &mhttp.Server{Handler: router(false, secret, "", Cors{})}
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() { _ = server.Close() })
	return "http://" + listener.Addr().String()
}

func doGet(t *testing.T, baseURL, path, authorization string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	require.NoError(t, err)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func TestRouterHelloAndVersion(t *testing.T) {
	base := newTestServer(t, "")

	status, body := doGet(t, base, "/", "")
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "hello")

	status, body = doGet(t, base, "/version", "")
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "version")
	require.Contains(t, body, "meta")
}

func TestRouterAuth(t *testing.T) {
	base := newTestServer(t, "test-secret")

	status, _ := doGet(t, base, "/version", "")
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = doGet(t, base, "/version", "Bearer wrong")
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = doGet(t, base, "/version", "Bearer test-secret")
	require.Equal(t, http.StatusOK, status)
}

func TestRouterReadEndpoints(t *testing.T) {
	base := newTestServer(t, "")
	for _, path := range []string{"/proxies", "/rules", "/connections"} {
		status, body := doGet(t, base, path, "")
		require.Equal(t, http.StatusOK, status, path)
		require.Contains(t, body, path[1:], path)
	}
}
