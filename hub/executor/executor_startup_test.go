package executor_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/metacubex/mihomo/config"
	"github.com/metacubex/mihomo/hub/executor"
)

// applyConfig used to skip the inbound listener creation entirely after the
// upstream refactor commented out updateListeners - the standalone binary came
// up with only the external controller and ignored every proxy port. This test
// pins the startup: the mixed listener from the config must serve a proxied
// request through the full tunnel-match-dial-relay chain.
func TestApplyConfigStartsInboundListeners(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	proxyURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}

	configYAML := fmt.Sprintf(`
mixed-port: %d
find-process-mode: "off"
log-level: "silent"
proxies:
  - name: DIRECT-A
    type: direct
proxy-groups:
  - name: PROXY
    type: select
    proxies: [DIRECT-A]
rules:
  - MATCH,PROXY
`, port)

	cfg, err := config.Parse([]byte(configYAML))
	if err != nil {
		t.Fatalf("config parse: %v", err)
	}
	executor.ApplyConfig(cfg, true)
	t.Cleanup(func() {
		empty, err := config.Parse([]byte("log-level: silent\n"))
		if err == nil {
			executor.ApplyConfig(empty, true)
		}
	})

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("the proxied request failed - the inbound listener did not start or the tunnel rejected it: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
