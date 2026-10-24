package provider

import (
	"testing"
	"time"
)

func TestHealthCheckWithoutATimeoutWaitsTheDefault(t *testing.T) {
	t.Cleanup(func() { defaultTestTimeout.Store(0) })

	if got := NewHealthCheck(nil, "http://own", 0, 0, true, nil).testTimeout(); got != fallbackTestTimeout {
		t.Errorf("unset timeout before any default = %v, want %v", got, fallbackTestTimeout)
	}
	SetDefaultTestTimeout(8 * time.Second)
	SetDefaultTestTimeout(0)
	if got := NewHealthCheck(nil, "http://own", 0, 0, true, nil).testTimeout(); got != 8*time.Second {
		t.Errorf("unset timeout = %v, want the default 8s", got)
	}
	if got := NewHealthCheck(nil, "http://own", 3000, 0, true, nil).testTimeout(); got != 3*time.Second {
		t.Errorf("configured timeout = %v, want 3s over the default", got)
	}
}

func TestTestTimeoutCoversOnlyTheURLsTheCheckTests(t *testing.T) {
	hc := NewHealthCheck(nil, "http://own", 3000, 0, true, nil)
	hc.registerHealthCheckTask("http://group", nil, "", 300)
	pd := &baseProvider{healthCheck: hc}

	for url, tested := range map[string]bool{"http://own": true, " http://group ": true, "http://other": false, "": false} {
		timeout, ok := pd.TestTimeout(url)
		if ok != tested || ok && timeout != 3*time.Second {
			t.Errorf("TestTimeout(%q) = %v, %t; want 3s, %t", url, timeout, ok, tested)
		}
	}

	unset := &baseProvider{healthCheck: NewHealthCheck(nil, "http://own", 0, 0, true, nil)}
	if _, ok := unset.TestTimeout("http://own"); ok {
		t.Error("TestTimeout reported a timeout for a check whose config names none")
	}
}
