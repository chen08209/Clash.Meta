package socks

import (
	"context"
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/adapter/inbound"
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
			handleSocksUDP(l, tunnel, data, remoteAddr, additions...)
		}
	}()

	return sl, nil
}

func handleSocksUDP(pc net.PacketConn, tunnel C.Tunnel, buf []byte, addr netip.AddrPort, additions ...inbound.Addition) {
	target, payload, err := socks5.DecodeUDPPacket(buf)
	if err != nil {
		// Unresolved UDP packet, return buffer to the pool
		_ = pool.Put(buf)
		return
	}
	packet := &packet{
		pc:      pc,
		rAddr:   net.UDPAddrFromAddrPort(addr),
		data:    buf,
		payload: payload,
	}
	tunnel.HandleUDPPacket(inbound.NewPacket(target, packet, C.SOCKS5, additions...))
}
