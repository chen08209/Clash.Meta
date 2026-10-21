package socks

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
	LC "github.com/metacubex/mihomo/listener/config"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/transport/socks5"
)

type UDPListener struct {
	packetConn net.PacketConn
	addr       string
	closed     bool
	flowCache  *lru.LruCache[netip.AddrPort, *socksUDPFlow]
}

// A client flow repeats its destination, so the metadata parsed from the
// datagram header is cached per source address and reused while the target
// bytes match; only the read loop touches the cache.
const (
	socksUDPFlowCacheSize = 1024
	socksUDPFlowCacheAge  = 300
)

type socksUDPFlow struct {
	metadata *C.Metadata
	target   []byte
	rAddr    net.Addr
}

// RawAddress implements C.Listener
func (l *UDPListener) RawAddress() string {
	return l.addr
}

// Address implements C.Listener
func (l *UDPListener) Address() string {
	return l.packetConn.LocalAddr().String()
}

// Close implements C.Listener
func (l *UDPListener) Close() error {
	l.closed = true
	return l.packetConn.Close()
}

func NewUDP(addr string, tunnel C.Tunnel, additions ...inbound.Addition) (*UDPListener, error) {
	return NewUDPWithConfig(defaultConfig(addr), inbound.NewListenConfig(), tunnel, additions...)
}

func NewUDPWithConfig(config LC.AuthServer, lc C.InboundListenConfig, tunnel C.Tunnel, additions ...inbound.Addition) (*UDPListener, error) {
	if len(additions) == 0 {
		additions = []inbound.Addition{
			inbound.WithInName("DEFAULT-SOCKS"),
			inbound.WithSpecialRules(""),
		}
	}
	l, err := lc.ListenPacket(context.Background(), "udp", config.Listen)
	if err != nil {
		return nil, err
	}

	if err := sockopt.UDPReuseaddr(l); err != nil {
		log.Warnln("Failed to Reuse UDP Address: %s", err)
	}

	sl := &UDPListener{
		packetConn: l,
		addr:       config.Listen,
		flowCache: lru.New(
			lru.WithSize[netip.AddrPort, *socksUDPFlow](socksUDPFlowCacheSize),
			lru.WithAge[netip.AddrPort, *socksUDPFlow](socksUDPFlowCacheAge),
		),
	}
	conn := N.NewEnhancePacketConn(l)
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
			sl.handleSocksUDP(tunnel, data, remoteAddr, additions...)
		}
	}()

	return sl, nil
}

func (l *UDPListener) handleSocksUDP(tunnel C.Tunnel, buf []byte, addr netip.AddrPort, additions ...inbound.Addition) {
	target, payload, err := socks5.DecodeUDPPacket(buf)
	if err != nil {
		// Unresolved UDP packet, return buffer to the pool
		_ = pool.Put(buf)
		return
	}
	flow, hit := l.flowCache.Get(addr)
	if !hit || !bytes.Equal(target, flow.target) {
		packet := &packet{
			pc:      l.packetConn,
			rAddr:   net.UDPAddrFromAddrPort(addr),
			data:    buf,
			payload: payload,
		}
		_, metadata := inbound.NewPacket(target, packet, C.SOCKS5, additions...)
		flow = &socksUDPFlow{
			metadata: metadata,
			target:   bytes.Clone(target),
			rAddr:    packet.rAddr,
		}
		l.flowCache.Set(addr, flow)
	}

	packet := &packet{
		pc:      l.packetConn,
		rAddr:   flow.rAddr,
		data:    buf,
		payload: payload,
	}
	tunnel.HandleUDPPacket(packet, flow.metadata)
}
