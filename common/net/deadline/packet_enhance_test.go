package deadline

import (
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/require"
)

type stubConnBase struct{}

func (*stubConnBase) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, net.ErrClosed }
func (*stubConnBase) WriteTo([]byte, net.Addr) (int, error)  { return 0, net.ErrClosed }
func (*stubConnBase) Close() error                           { return nil }
func (*stubConnBase) LocalAddr() net.Addr                    { return &net.UDPAddr{Port: 1080} }
func (*stubConnBase) SetDeadline(time.Time) error            { return nil }
func (*stubConnBase) SetReadDeadline(time.Time) error        { return nil }
func (*stubConnBase) SetWriteDeadline(time.Time) error       { return nil }

type immediateStub struct {
	stubConnBase
	data []byte
	addr netip.AddrPort
}

func (s *immediateStub) WaitReadFrom() ([]byte, netip.AddrPort, error) {
	return s.data, s.addr, nil
}

// blockingStub parks its reader until released: the deadline wrapper's select
// picks randomly among ready cases, so an expired deadline may start a pipe
// read, and a parked reader keeps that read from completing so the expired
// deadline is observed deterministically.
type blockingStub struct {
	stubConnBase
	release chan struct{}
}

func (s *blockingStub) WaitReadFrom() ([]byte, netip.AddrPort, error) {
	<-s.release
	return nil, netip.AddrPort{}, nil
}

func TestEnhanceWaitReadFromWithoutDeadline(t *testing.T) {
	source := netip.MustParseAddrPort("1.2.3.4:53")
	inner := &immediateStub{data: []byte("pong"), addr: source}
	conn := NewEnhancePacketConn(inner)

	data, addr, err := conn.WaitReadFrom()
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), data)
	require.Equal(t, source, addr)
	_ = pool.Put(data)
}

func TestEnhanceWaitReadFromWithExpiredDeadline(t *testing.T) {
	inner := &blockingStub{release: make(chan struct{})}
	conn := NewEnhancePacketConn(inner)
	t.Cleanup(func() { close(inner.release) })
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(-time.Second)))

	data, addr, err := conn.WaitReadFrom()
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
	require.Nil(t, data)
	require.Equal(t, netip.AddrPort{}, addr)
}
