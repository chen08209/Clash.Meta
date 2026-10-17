package statistic

import (
	"net"
	"net/netip"
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
