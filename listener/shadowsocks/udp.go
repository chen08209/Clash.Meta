package shadowsocks

import (
	"context"
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/adapter/inbound"
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
}

func NewUDP(addr string, lc C.InboundListenConfig, pickCipher core.Cipher, tunnel C.Tunnel, additions ...inbound.Addition) (*UDPListener, error) {
	l, err := lc.ListenPacket(context.Background(), "udp", addr)
	if err != nil {
		return nil, err
	}

	if err := sockopt.UDPReuseaddr(l); err != nil {
		log.Warnln("Failed to Reuse UDP Address: %s", err)
	}

	sl := &UDPListener{l, false}
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
			handleSocksUDP(conn, tunnel, data, remoteAddr, additions...)
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

func handleSocksUDP(pc net.PacketConn, tunnel C.Tunnel, buf []byte, addr netip.AddrPort, additions ...inbound.Addition) {
	tgtAddr := socks5.SplitAddr(buf)
	if tgtAddr == nil {
		// Unresolved UDP packet, return buffer to the pool
		_ = pool.Put(buf)
		return
	}
	target := tgtAddr
	payload := buf[len(tgtAddr):]

	packet := &packet{
		pc:      pc,
		rAddr:   net.UDPAddrFromAddrPort(addr),
		data:   buf,
		payload: payload,
	}
	tunnel.HandleUDPPacket(inbound.NewPacket(target, packet, C.SHADOWSOCKS, additions...))
}
