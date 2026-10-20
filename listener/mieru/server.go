package mieru

import (
	"errors"
	"io"
	"net"
	"net/netip"

	"github.com/metacubex/mihomo/adapter/inbound"
	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/socks5"

	mierucommon "github.com/enfein/mieru/v3/apis/common"
	mieruconstant "github.com/enfein/mieru/v3/apis/constant"
	mierumodel "github.com/enfein/mieru/v3/apis/model"
)

func Handle(conn net.Conn, tunnel C.Tunnel, request *mierumodel.Request, additions ...inbound.Addition) {
	// Return a fake response to the client.
	resp := &mierumodel.Response{
		Reply: mieruconstant.Socks5ReplySuccess,
		BindAddr: mierumodel.AddrSpec{
			IP:   net.IPv4zero,
			Port: 0,
		},
	}
	if err := resp.WriteToSocks5(conn); err != nil {
		conn.Close()
		return
	}
	user := conn.(mierucommon.UserContext).UserName()
	// Handle the connection with tunnel.
	switch request.Command {
	case mieruconstant.Socks5ConnectCmd: // TCP
		metadata := &C.Metadata{
			NetWork: C.TCP,
			Type:    C.MIERU,
			DstPort: uint16(request.DstAddr.Port),
		}
		if request.DstAddr.FQDN != "" {
			metadata.Host = request.DstAddr.FQDN
		} else if request.DstAddr.IP != nil {
			metadata.DstIP, _ = netip.AddrFromSlice(request.DstAddr.IP)
			metadata.DstIP = metadata.DstIP.Unmap()
		}
		inbound.ApplyAdditions(
			metadata,
			inbound.WithInUser(user),
			inbound.WithSrcAddr(conn.RemoteAddr()),
			inbound.WithInAddr(conn.LocalAddr()),
		)
		inbound.ApplyAdditions(metadata, additions...)
		tunnel.HandleTCPConn(conn, metadata)
	case mieruconstant.Socks5UDPAssociateCmd: // UDP
		additions = append(additions, inbound.WithInUser(user))
		pc := mierucommon.NewPacketOverStreamTunnel(conn)
		ep := N.NewEnhancePacketConn(pc)
		defer ep.Close()
		for {
			data, addr, err := ep.WaitReadFrom()
			if err != nil {
				if data != nil {
					// Unresolved UDP packet, return buffer to the pool.
					_ = pool.Put(data)
				}
				// mieru returns EOF or ErrUnexpectedEOF when a session is closed.
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe) {
					break
				}
				continue
			}
			target, payload, err := socks5.DecodeUDPPacket(data)
			if err != nil {
				return
			}
			packet := &packet{
				pc:      ep,
				udpAddr: net.UDPAddrFromAddrPort(addr),
				data:    data,
				payload: payload,
			}
			tunnel.HandleUDPPacket(inbound.NewPacket(target, packet, C.MIERU, additions...))
		}
	}
}

type packet struct {
	pc      net.PacketConn
	udpAddr *net.UDPAddr // source (i.e. remote) IP & Port of the packet
	data    []byte
	payload []byte
}

var _ C.UDPPacket = (*packet)(nil)
var _ C.UDPPacketInAddr = (*packet)(nil)

func (c *packet) Data() []byte {
	return c.payload
}

func (c *packet) WriteBack(b []byte, addr netip.AddrPort) (n int, err error) {
	buf := pool.Get(socks5.UDPPacketHeadroom + len(b))
	defer pool.Put(buf)
	packet, err := socks5.EncodeUDPPacketInto(buf, addr, b)
	if err != nil {
		return
	}
	return c.pc.WriteTo(packet, c.udpAddr)
}

func (c *packet) Drop() {
	if c.data != nil {
		_ = pool.Put(c.data)
		c.data = nil
	}
	c.payload = nil
}

func (c *packet) LocalAddr() net.Addr {
	return c.udpAddr
}

func (c *packet) InAddr() net.Addr {
	return c.pc.LocalAddr()
}
