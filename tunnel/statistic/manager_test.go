package statistic

import (
	"net"
	"net/netip"
	"sync"
	"testing"

	"github.com/gofrs/uuid/v5"
	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type statisticTestConn struct {
	N.ExtendedConn
}

func newStatisticTestConn(conn net.Conn) C.Conn {
	return &statisticTestConn{ExtendedConn: N.NewExtendedConn(conn)}
}

func (c *statisticTestConn) Chains() C.Chain {
	return nil
}

func (c *statisticTestConn) ProviderChains() C.Chain {
	return nil
}

func (c *statisticTestConn) AppendToChains(C.ProxyAdapter) {}

func (c *statisticTestConn) RemoteDestination() string {
	return ""
}

func newTestTracker(t *testing.T) *tcpTracker {
	t.Helper()

	conn, remote := net.Pipe()
	t.Cleanup(func() {
		_ = conn.Close()
		_ = remote.Close()
	})
	return NewTCPTracker(
		newStatisticTestConn(conn),
		&Manager{},
		&C.Metadata{
			NetWork: C.TCP,
			SrcIP:   netip.MustParseAddr("127.0.0.1"),
			DstIP:   netip.MustParseAddr("127.0.0.1"),
			DstPort: 443,
		},
		nil,
		0,
		0,
		false,
	)
}

func TestManagerUsesFormattedUUIDCompatibilityLookup(t *testing.T) {
	manager := &Manager{}
	tracker := newTestTracker(t)
	manager.Join(tracker)

	require.Equal(t, tracker, manager.Get(tracker.ID()))
	_, err := uuid.FromString(tracker.ID())
	require.NoError(t, err)
	require.Nil(t, manager.Get("not-a-uuid"))

	manager.Leave(tracker)
	require.Nil(t, manager.Get(tracker.ID()))
}

func TestManagerRangesBinaryKeyedTrackers(t *testing.T) {
	manager := &Manager{}
	tracker := newTestTracker(t)
	manager.Join(tracker)
	defer manager.Leave(tracker)

	var ranged int
	manager.Range(func(candidate Tracker) bool {
		ranged++
		require.Equal(t, tracker, candidate)
		return false
	})
	require.Equal(t, 1, ranged)
}

func newStripedManager() *Manager {
	manager := &Manager{stripes: make([]counterStripe, counterStripeCount)}
	return manager
}

func TestManagerParallelPushSumsExactlyAcrossStripes(t *testing.T) {
	manager := newStripedManager()

	const workers = 8
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				manager.PushDownloaded(false, 3)
			}
		}()
	}
	wg.Wait()

	expected := int64(workers * 1000 * 3)
	up, down := manager.TotalTraffic(false)
	require.EqualValues(t, 0, up)
	require.EqualValues(t, expected, down)

	proxyUp, proxyDown := manager.TotalTraffic(true)
	require.EqualValues(t, 0, proxyUp)
	require.EqualValues(t, expected, proxyDown)

	manager.PushUploaded(true, 7)
	up, down = manager.TotalTraffic(false)
	require.EqualValues(t, 7, up)
	require.EqualValues(t, expected, down)
	proxyUp, _ = manager.TotalTraffic(true)
	require.EqualValues(t, 0, proxyUp) // direct traffic never reaches proxy counters
}

func TestManagerTrackerConstructionFeedsBoundStripe(t *testing.T) {
	manager := newStripedManager()
	conn, remote := net.Pipe()
	t.Cleanup(func() {
		_ = conn.Close()
		_ = remote.Close()
	})

	tracker := NewTCPTracker(
		newStatisticTestConn(conn),
		manager,
		&C.Metadata{NetWork: C.TCP, DstPort: 443},
		nil,
		100,
		100,
		true,
	)
	defer func() { _ = tracker.Close() }()

	up, down := manager.TotalTraffic(false)
	require.EqualValues(t, 100, up)
	require.EqualValues(t, 100, down)
	require.EqualValues(t, 100, tracker.UploadTotal.Load())
	require.EqualValues(t, 100, tracker.DownloadTotal.Load())
}

func TestManagerBlipSwapDrainsEveryStripe(t *testing.T) {
	manager := newStripedManager()

	first := manager.bindStripe()
	second := manager.bindStripe()
	require.NotSame(t, first, second)

	first.pushDownloaded(10, false)
	second.pushDownloaded(5, false)
	first.pushUploaded(2, true)
	second.pushUploaded(3, true)

	require.EqualValues(t, 15, manager.swapDownloadTemp())
	require.EqualValues(t, 5, manager.swapUploadTemp())
	require.EqualValues(t, 0, manager.swapDownloadTemp())
	require.EqualValues(t, 0, manager.swapUploadTemp())
}

func TestManagerBindStripeSpreadsRoundRobin(t *testing.T) {
	manager := newStripedManager()

	seen := make(map[*counterStripe]struct{}, counterStripeCount)
	for i := 0; i < counterStripeCount; i++ {
		seen[manager.bindStripe()] = struct{}{}
	}
	require.Len(t, seen, counterStripeCount)
}

func TestManagerResetZeroesAllStripes(t *testing.T) {
	manager := newStripedManager()
	for i := 0; i < counterStripeCount; i++ {
		manager.stripes[i].pushDownloaded(9, false)
	}

	manager.ResetStatistic()

	up, down := manager.TotalTraffic(false)
	require.EqualValues(t, 0, up)
	require.EqualValues(t, 0, down)
}

func TestManagerZeroValueHasNoStripePanic(t *testing.T) {
	manager := &Manager{}

	require.Nil(t, manager.bindStripe())
	require.Nil(t, manager.sharedStripe())

	manager.PushUploaded(false, 1)
	manager.PushDownloaded(false, 1)

	up, down := manager.TotalTraffic(false)
	require.EqualValues(t, 0, up)
	require.EqualValues(t, 0, down)

	tracker := newTestTracker(t)
	require.Nil(t, tracker.stripe)
}
