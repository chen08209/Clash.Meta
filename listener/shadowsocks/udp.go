package shadowsocks

import (
	"bytes"
	"context"
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/adapter/inbound"
	"github.com/metacubex/mihomo/common/lru"
	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/pool"
	"github.com/metacubex/mihomo/common/sockopt"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/transport/shadowsocks/core"
	"github.com/metacubex/mihomo/transport/socks5"
)

type UDPListener struct {
	packetConn net.PacketConn
	closed     bool
	flowCache  *lru.LruCache[netip.AddrPort, *ssUDPFlow]
}

// A client flow repeats its destination, so the metadata parsed from the
// datagram header is cached per source address and reused while the target
// bytes match; only the read loop touches the cache.
const (
	ssUDPFlowCacheSize = 1024
	ssUDPFlowCacheAge  = 300
)

type ssUDPFlow struct {
	metadata *C.Metadata
	target   []byte
	rAddr    net.Addr
}

func NewUDP(addr string, lc C.InboundListenConfig, pickCipher core.Cipher, tunnel C.Tunnel, additions ...inbound.Addition) (*UDPListener, error) {
	l, err := lc.ListenPacket(context.Background(), "udp", addr)
	if err != nil {
		return nil, err
	}

	if err := sockopt.UDPReuseaddr(l); err != nil {
		log.Warnln("Failed to Reuse UDP Address: %s", err)
	}

	sl := &UDPListener{
		packetConn: l,
		flowCache: lru.New(
			lru.WithSize[netip.AddrPort, *ssUDPFlow](ssUDPFlowCacheSize),
			lru.WithAge[netip.AddrPort, *ssUDPFlow](ssUDPFlowCacheAge),
		),
	}
	conn := pickCipher.PacketConn(N.NewEnhancePacketConn(l))
	go func() {
		for {
			data, remoteAddr, err := conn.WaitReadFrom()
			if err != nil {
				if data != nil {
					_ = pool.Put(data)
				}
				if sl.closed {
					break
				}
				continue
			}
			sl.handleSocksUDP(conn, tunnel, data, remoteAddr, additions...)
		}
	}()

	return sl, nil
}

func (l *UDPListener) Close() error {
	l.closed = true
	return l.packetConn.Close()
}

func (l *UDPListener) LocalAddr() net.Addr {
	return l.packetConn.LocalAddr()
}

func (l *UDPListener) handleSocksUDP(pc net.PacketConn, tunnel C.Tunnel, buf []byte, addr netip.AddrPort, additions ...inbound.Addition) {
	target := socks5.SplitAddr(buf)
	if target == nil {
		// Unresolved UDP packet, return buffer to the pool
		_ = pool.Put(buf)
		return
	}
	payload := buf[len(target):]

	flow, hit := l.flowCache.Get(addr)
	if !hit || !bytes.Equal(target, flow.target) {
		src := newSSPacket(pc, net.UDPAddrFromAddrPort(addr), buf, payload)
		_, metadata := inbound.NewPacket(target, src, C.SHADOWSOCKS, additions...)
		flow = &ssUDPFlow{
			metadata: metadata,
			target:   bytes.Clone(target),
			rAddr:    src.rAddr,
		}
		l.flowCache.Set(addr, flow)
	}

	ssPacket := newSSPacket(pc, flow.rAddr, buf, payload)
	tunnel.HandleUDPPacket(ssPacket, flow.metadata)
}
