package tproxy

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/metacubex/mihomo/adapter/inbound"
	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
)

// A wrapper is returned only by Drop, the tunnel contract's single release point.
var tproxyPacketPool = sync.Pool{New: func() interface{} { return new(packet) }}

func newTproxyPacket(pc net.PacketConn, lAddr netip.AddrPort, buf []byte, tunnel C.Tunnel, additions []inbound.Addition) *packet {
	tproxyPacket := tproxyPacketPool.Get().(*packet)
	*tproxyPacket = packet{
		pc:        pc,
		lAddr:     lAddr,
		buf:       buf,
		tunnel:    tunnel,
		additions: additions,
	}
	return tproxyPacket
}

type packet struct {
	pc        net.PacketConn
	lAddr     netip.AddrPort
	buf       []byte
	tunnel    C.Tunnel
	additions []inbound.Addition
}

func (c *packet) Data() []byte {
	return c.buf
}

// FlowKey returns the flow key the tunnel keys this packet's flow by.
func (c *packet) FlowKey() netip.AddrPort {
	return c.lAddr
}

// WriteBack opens a new socket binding `addr` to write UDP packet back
func (c *packet) WriteBack(b []byte, rAddr netip.AddrPort) (n int, err error) {
	tc, err := createOrGetLocalConn(rAddr, c.lAddr, c.tunnel, c.additions...)
	if err != nil {
		return
	}
	n, err = tc.Write(b)
	return
}

// LocalAddr returns the source IP/Port of UDP Packet
func (c *packet) LocalAddr() net.Addr {
	return net.UDPAddrFromAddrPort(c.lAddr)
}

func (c *packet) Drop() {
	if c.buf == nil {
		return
	}
	_ = pool.Put(c.buf)
	c.buf = nil
	tproxyPacketPool.Put(c)
}

func (c *packet) InAddr() net.Addr {
	return c.pc.LocalAddr()
}

// this function listen at rAddr and write to lAddr
// for here, rAddr is the ip/port client want to access
// lAddr is the ip/port client opened
func createOrGetLocalConn(rAddr, lAddr netip.AddrPort, tunnel C.Tunnel, additions ...inbound.Addition) (*net.UDPConn, error) {
	natTable := tunnel.NatTable()
	localConn := natTable.GetForLocalConn(lAddr, rAddr)
	// localConn not exist
	if localConn == nil {
		cond, loaded := natTable.GetOrCreateLockForLocalConn(lAddr, rAddr)
		if loaded {
			cond.L.Lock()
			cond.Wait()
			// we should get localConn here
			localConn = natTable.GetForLocalConn(lAddr, rAddr)
			if localConn == nil {
				return nil, fmt.Errorf("localConn is nil, nat entry not exist")
			}
			cond.L.Unlock()
		} else {
			if cond == nil {
				return nil, fmt.Errorf("cond is nil, nat entry not exist")
			}
			defer func() {
				natTable.DeleteLockForLocalConn(lAddr, rAddr)
				cond.Broadcast()
			}()
			conn, err := listenLocalConn(rAddr, lAddr, tunnel, additions...)
			if err != nil {
				log.Errorln("listenLocalConn failed with error: %s, packet loss (rAddr[%T]=%s lAddr[%T]=%s)", err.Error(), rAddr, rAddr, lAddr, lAddr)
				return nil, err
			}
			natTable.AddForLocalConn(lAddr, rAddr, conn)
			localConn = conn
		}
	}
	return localConn, nil
}

// this function listen at rAddr
// and send what received to program itself, then send to real remote
func listenLocalConn(rAddr, lAddr netip.AddrPort, tunnel C.Tunnel, additions ...inbound.Addition) (*net.UDPConn, error) {
	lc, err := dialUDP("udp", rAddr, lAddr)
	if err != nil {
		return nil, err
	}
	go func() {
		log.Debugln("TProxy listenLocalConn rAddr=%s lAddr=%s", rAddr, lAddr)
		for {
			buf := pool.Get(pool.UDPBufferSize)
			br, err := lc.Read(buf)
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					log.Debugln("TProxy local conn listener exit.. rAddr=%s lAddr=%s", rAddr, lAddr)
					pool.Put(buf)
					return
				}
			}
			// since following localPackets are pass through this socket which listen rAddr
			// I choose current listener as packet's packet conn
			handlePacketConn(lc, tunnel, buf[:br], lAddr, rAddr, 0, additions...)
		}
	}()
	return lc, nil
}
