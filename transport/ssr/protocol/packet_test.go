package protocol

import (
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/stretchr/testify/require"
)

type stubPacketConn struct {
	data       []byte
	addr       netip.AddrPort
	waitErr    error
	waitCalled int
}

func (c *stubPacketConn) WaitReadFrom() ([]byte, netip.AddrPort, error) {
	c.waitCalled++
	if c.waitErr != nil {
		return nil, netip.AddrPort{}, c.waitErr
	}
	return c.data, c.addr, nil
}

func (*stubPacketConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, net.ErrClosed }
func (*stubPacketConn) WriteTo([]byte, net.Addr) (int, error)  { return 0, net.ErrClosed }
func (*stubPacketConn) Close() error                           { return nil }
func (*stubPacketConn) LocalAddr() net.Addr                    { return &net.UDPAddr{Port: 1080} }
func (*stubPacketConn) SetDeadline(time.Time) error            { return nil }
func (*stubPacketConn) SetReadDeadline(time.Time) error        { return nil }
func (*stubPacketConn) SetWriteDeadline(time.Time) error       { return nil }

func TestPacketConnWaitReadFromDecodes(t *testing.T) {
	source := netip.MustParseAddrPort("1.2.3.4:53")
	inner := &stubPacketConn{
		data: []byte("encoded"),
		addr: source,
	}
	pc := &PacketConn{EnhancePacketConn: inner, Protocol: &origin{}}

	data, addr, err := pc.WaitReadFrom()
	require.NoError(t, err)
	require.Equal(t, []byte("encoded"), data)
	require.Equal(t, source, addr)
	_ = pool.Put(data)
}

func TestPacketConnWaitReadFromDropsAnUndecodablePacket(t *testing.T) {
	source := netip.MustParseAddrPort("1.2.3.4:53")
	inner := &stubPacketConn{
		data: []byte("encoded"),
		addr: source,
	}
	pc := &PacketConn{
		EnhancePacketConn: inner,
		Protocol:          &failingDecodeProtocol{},
	}

	data, addr, err := pc.WaitReadFrom()
	require.Error(t, err)
	require.Nil(t, data)
	require.Equal(t, source, addr, "the error path returns the source address it read")
}

type failingDecodeProtocol struct {
	origin
}

func (p *failingDecodeProtocol) DecodePacket([]byte) ([]byte, error) {
	return nil, errors.New("decode fail")
}
