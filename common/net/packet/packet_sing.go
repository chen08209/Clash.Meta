package packet

import (
	"net/netip"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/metacubex/sing/common/buf"
	"github.com/metacubex/sing/common/bufio"
	M "github.com/metacubex/sing/common/metadata"
	N "github.com/metacubex/sing/common/network"
)

type SingPacketConn = N.NetPacketConn

type EnhanceSingPacketConn interface {
	SingPacketConn
	EnhancePacketConn
}

type enhanceSingPacketConn struct {
	SingPacketConn
	packetReadWaiter N.PacketReadWaiter
}

func (c *enhanceSingPacketConn) WaitReadFrom() (data []byte, addr netip.AddrPort, err error) {
	var buff *buf.Buffer
	var dest M.Socksaddr
	rwOptions := N.ReadWaitOptions{}
	if c.packetReadWaiter != nil {
		c.packetReadWaiter.InitializeReadWaiter(rwOptions)
		buff, dest, err = c.packetReadWaiter.WaitReadPacket()
	} else {
		buff = rwOptions.NewPacketBuffer()
		dest, err = c.SingPacketConn.ReadPacket(buff)
		if buff != nil {
			rwOptions.PostReturn(buff)
		}
	}
	if !dest.IsFqdn() {
		addr = dest.AddrPort()
	}
	if err != nil {
		buff.Release()
		return nil, addr, err
	}
	if buff == nil {
		return nil, addr, nil
	}
	if buff.IsEmpty() {
		buff.Release()
		return nil, addr, nil
	}
	// copy into mihomo-pool memory: the caller recycles it with pool.Put, and
	// the sing buffer goes back to its own pool right away
	data = pool.Get(buff.Len())
	copy(data, buff.Bytes())
	buff.Release()
	return data, addr, nil
}

func (c *enhanceSingPacketConn) Upstream() any {
	return c.SingPacketConn
}

func (c *enhanceSingPacketConn) WriterReplaceable() bool {
	return true
}

func (c *enhanceSingPacketConn) ReaderReplaceable() bool {
	return true
}

func newEnhanceSingPacketConn(conn SingPacketConn) *enhanceSingPacketConn {
	epc := &enhanceSingPacketConn{SingPacketConn: conn}
	if readWaiter, isReadWaiter := bufio.CreatePacketReadWaiter(conn); isReadWaiter {
		epc.packetReadWaiter = readWaiter
	}
	return epc
}
