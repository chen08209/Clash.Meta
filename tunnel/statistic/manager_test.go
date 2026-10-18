package statistic

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagerTrafficAccounting(t *testing.T) {
	m := &Manager{}

	m.PushUploaded(0, false, 100) // proxied traffic counts globally and per proxy
	m.PushUploaded(1, true, 50)   // direct traffic counts globally only
	m.PushDownloaded(2, false, 200)

	up, down := m.Total()
	require.Equal(t, int64(150), up)
	require.Equal(t, int64(200), down)

	upProxy, downProxy := m.TotalTraffic(true)
	require.Equal(t, int64(100), upProxy)
	require.Equal(t, int64(200), downProxy)

	// the per-second rotation moves the temps into the blips
	m.rotateBlips()
	upNow, downNow := m.Now()
	require.Equal(t, int64(150), upNow)
	require.Equal(t, int64(200), downNow)
	m.rotateBlips()
	upNow, _ = m.Now()
	require.Zero(t, upNow)

	m.ResetStatistic()
	up, down = m.Total()
	require.Zero(t, up)
	require.Zero(t, down)
	upProxy, _ = m.TotalTraffic(true)
	require.Zero(t, upProxy)
}

func TestPickStripeRoundRobin(t *testing.T) {
	m := &Manager{}
	seen := map[int]bool{}
	for i := 0; i < managerStripeCount; i++ {
		seen[m.pickStripe()] = true
	}
	require.Len(t, seen, managerStripeCount)
}
