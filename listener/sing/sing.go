package sing

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter/inbound"
	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"

	"github.com/gofrs/uuid/v5"
	mux "github.com/metacubex/sing-mux"
	vmess "github.com/metacubex/sing-vmess"
	"github.com/metacubex/sing-vmess/packetaddr"
	"github.com/metacubex/sing/common"
	"github.com/metacubex/sing/common/buf"
	"github.com/metacubex/sing/common/bufio"
	"github.com/metacubex/sing/common/bufio/deadline"
	E "github.com/metacubex/sing/common/exceptions"
	M "github.com/metacubex/sing/common/metadata"
	"github.com/metacubex/sing/common/network"
	"github.com/metacubex/sing/common/uot"
)

const UDPTimeout = 5 * time.Minute
const ICMPTimeout = 10 * time.Second

// A wrapper is returned only by Drop, the tunnel contract's single release point.
var singPacketPool = sync.Pool{New: func() interface{} { return new(packet) }}
var singBufferPool sync.Pool

type ListenerConfig struct {
	Tunnel     C.Tunnel
	Type       C.Type
	Additions  []inbound.Addition
	UDPTimeout time.Duration
	MuxOption  MuxOption
}

type MuxOption struct {
	Padding bool          `yaml:"padding" json:"padding,omitempty"`
	Brutal  BrutalOptions `yaml:"brutal" json:"brutal,omitempty"`
}

type BrutalOptions struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Up      string `yaml:"up" json:"up,omitempty"`
	Down    string `yaml:"down" json:"down,omitempty"`
}

type ListenerHandler struct {
	ListenerConfig
	handlerId  uuid.UUID
	muxService *mux.Service
}

func UpstreamMetadata(metadata M.Metadata) M.Metadata {
	return M.Metadata{
		Source:      metadata.Source,
		Destination: metadata.Destination,
	}
}

func ConvertMetadata(metadata *C.Metadata) M.Metadata {
	return M.Metadata{
		Protocol:    metadata.Type.String(),
		Source:      M.SocksaddrFrom(metadata.SrcIP, metadata.SrcPort),
		Destination: M.ParseSocksaddrHostPort(metadata.String(), metadata.DstPort),
	}
}

func NewListenerHandler(lc ListenerConfig) (h *ListenerHandler, err error) {
	h = &ListenerHandler{ListenerConfig: lc}
	h.handlerId = utils.NewUUIDV4()
	h.muxService, err = mux.NewService(mux.ServiceOptions{
		NewStreamContext: func(ctx context.Context, conn net.Conn) context.Context {
			return ctx
		},
		Logger:  log.SingInfoToDebugLogger, // convert sing-mux info log to debug
		Handler: h,
		Padding: lc.MuxOption.Padding,
		Brutal: mux.BrutalOptions{
			Enabled:    lc.MuxOption.Brutal.Enabled,
			SendBPS:    utils.StringToBps(lc.MuxOption.Brutal.Up),
			ReceiveBPS: utils.StringToBps(lc.MuxOption.Brutal.Down),
		},
	})
	return
}

func (h *ListenerHandler) IsSpecialFqdn(fqdn string) bool {
	switch fqdn {
	case mux.Destination.Fqdn,
		vmess.MuxDestination.Fqdn,
		uot.MagicAddress,
		uot.LegacyMagicAddress:
		return true
	default:
		return false
	}
}

func (h *ListenerHandler) ParseSpecialFqdn(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	switch metadata.Destination.Fqdn {
	case mux.Destination.Fqdn:
		return h.muxService.NewConnection(ctx, conn, UpstreamMetadata(metadata))
	case vmess.MuxDestination.Fqdn:
		return vmess.HandleMuxConnection(ctx, conn, metadata, h)
	case uot.MagicAddress:
		request, err := uot.ReadRequest(conn)
		if err != nil {
			return E.Cause(err, "read UoT request")
		}
		metadata.Destination = request.Destination
		return h.NewPacketConnection(ctx, uot.NewConn(conn, *request), metadata)
	case uot.LegacyMagicAddress:
		metadata.Destination = M.Socksaddr{Addr: netip.IPv4Unspecified()}
		return h.NewPacketConnection(ctx, uot.NewConn(conn, uot.Request{}), metadata)
	}
	return errors.New("not special fqdn")
}

func (h *ListenerHandler) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	if h.IsSpecialFqdn(metadata.Destination.Fqdn) {
		return h.ParseSpecialFqdn(ctx, conn, metadata)
	}

	if deadline.NeedAdditionalReadDeadline(conn) {
		conn = N.NewDeadlineConn(conn) // conn from sing should check NeedAdditionalReadDeadline
	}

	cMetadata := &C.Metadata{
		NetWork: C.TCP,
		Type:    h.Type,
	}
	_ = cMetadata.SetRemoteAddr(metadata.Destination)
	inbound.SetSrcAddr(cMetadata, metadata.Source)
	inbound.SetInAddr(cMetadata, conn.LocalAddr())
	inbound.ApplyAdditions(cMetadata, h.Additions...)
	inbound.ApplyAdditions(cMetadata, getAdditions(ctx)...)

	h.Tunnel.HandleTCPConn(conn, cMetadata) // this goroutine must exit after conn unused
	return nil
}

func (h *ListenerHandler) NewPacketConnection(ctx context.Context, conn network.PacketConn, metadata M.Metadata) error {
	if metadata.Destination.Fqdn == packetaddr.SeqPacketMagicAddress {
		conn = packetaddr.NewConn(bufio.NewNetPacketConn(conn), M.Socksaddr{})
	}

	defer func() { _ = conn.Close() }()
	mutex := sync.Mutex{}
	writer := bufio.NewNetPacketWriter(conn)
	writerBox := &writer
	mutexBox := &mutex
	defer func() {
		// this goroutine must exit after all conn.WritePacket() is not running
		mutexBox.Lock()
		defer mutexBox.Unlock()
		writer = nil
	}()
	rwOptions := network.ReadWaitOptions{}
	readWaiter, isReadWaiter := bufio.CreatePacketReadWaiter(conn)
	if isReadWaiter {
		readWaiter.InitializeReadWaiter(rwOptions)
	}
	inAddr := conn.LocalAddr()
	if lAddr := getInAddr(ctx); lAddr != nil {
		inAddr = lAddr
	}
	// A UDP flow repeats its destination, so the metadata built from the first
	// datagram is reused until the destination changes. Only the tunnel's
	// worker goroutine touches the shared metadata after handoff, and its
	// in-place fixes are idempotent; datagrams that carry a different
	// destination still get their own metadata.
	var (
		metadataTemplate *C.Metadata
		templateDest     M.Socksaddr
	)
	for {
		var (
			buff *buf.Buffer
			dest M.Socksaddr
			err  error
		)
		if isReadWaiter {
			buff, dest, err = readWaiter.WaitReadPacket()
		} else {
			buff = newSingBuffer(rwOptions)
			dest, err = conn.ReadPacket(buff)
			if buff != nil {
				rwOptions.PostReturn(buff)
			}
		}
		if err != nil {
			buff.Release()
			if ShouldIgnorePacketError(err) {
				break
			}
			return err
		}
		cPacket := newSingPacket(writerBox, mutexBox, metadata.Source.AddrPort(), inAddr, buff)
		if metadataTemplate != nil && dest == templateDest {
			h.Tunnel.HandleUDPPacket(cPacket, metadataTemplate)
			continue
		}
		metadataTemplate = h.buildPacketMetadata(ctx, cPacket, metadata.Source, dest)
		templateDest = dest
		h.Tunnel.HandleUDPPacket(cPacket, metadataTemplate)
	}
	return nil
}

type localAddr interface {
	LocalAddr() net.Addr
}

func (h *ListenerHandler) NewPacket(ctx context.Context, key netip.AddrPort, buffer *buf.Buffer, metadata M.Metadata, init func(natConn network.PacketConn) network.PacketWriter) {
	writer := bufio.NewNetPacketWriter(init(nil))
	mutex := sync.Mutex{}
	cPacket := newSingPacket(&writer, &mutex, metadata.Source.AddrPort(), nil, buffer)
	if conn, ok := common.Cast[localAddr](writer); ok { // tun does not have real inAddr
		cPacket.lAddr = conn.LocalAddr()
	}
	h.handlePacket(ctx, cPacket, metadata.Source, metadata.Destination)
}

func (h *ListenerHandler) handlePacket(ctx context.Context, cPacket *packet, source M.Socksaddr, destination M.Socksaddr) {
	h.Tunnel.HandleUDPPacket(cPacket, h.buildPacketMetadata(ctx, cPacket, source, destination))
}

func (h *ListenerHandler) buildPacketMetadata(ctx context.Context, cPacket *packet, source M.Socksaddr, destination M.Socksaddr) *C.Metadata {
	cMetadata := &C.Metadata{
		NetWork: C.UDP,
		Type:    h.Type,
	}
	_ = cMetadata.SetRemoteAddr(destination)
	inbound.SetSrcAddr(cMetadata, source)
	inbound.SetInAddr(cMetadata, cPacket.InAddr())
	inbound.ApplyAdditions(cMetadata, h.Additions...)
	inbound.ApplyAdditions(cMetadata, getAdditions(ctx)...)
	return cMetadata
}

func (h *ListenerHandler) NewError(ctx context.Context, err error) {
	log.Warnln("%s listener get error: %+v", h.Type.String(), err)
}

func (h *ListenerHandler) TypeMutation(typ C.Type) *ListenerHandler {
	handler := *h
	handler.Type = typ
	return &handler
}

func ShouldIgnorePacketError(err error) bool {
	// ignore simple error
	if E.IsTimeout(err) || E.IsClosed(err) || E.IsCanceled(err) {
		return true
	}
	return false
}

func newSingPacket(writer *network.NetPacketWriter, mutex *sync.Mutex, source netip.AddrPort, lAddr net.Addr, buff *buf.Buffer) *packet {
	singPacket := singPacketPool.Get().(*packet)
	*singPacket = packet{
		writer: writer,
		mutex:  mutex,
		source: source,
		lAddr:  lAddr,
		buff:   buff,
	}
	return singPacket
}

func newSingBuffer(rwOptions network.ReadWaitOptions) *buf.Buffer {
	// Drop returns buffers here; reuse only when the pooled capacity fits the requested MTU/headroom.
	bufferSize := buf.UDPBufferSize
	if rwOptions.MTU > 0 {
		bufferSize = rwOptions.MTU + rwOptions.FrontHeadroom + rwOptions.RearHeadroom
	}
	if value := singBufferPool.Get(); value != nil {
		buffer := value.(*buf.Buffer)
		if buffer.Cap() >= bufferSize {
			buffer.Reset()
			if rwOptions.FrontHeadroom > 0 {
				buffer.Resize(rwOptions.FrontHeadroom, 0)
			}
			if rwOptions.RearHeadroom > 0 {
				buffer.Reserve(rwOptions.RearHeadroom)
			}
			return buffer
		}
		buffer.Release()
	}
	return rwOptions.NewPacketBuffer()
}

type packet struct {
	writer *network.NetPacketWriter
	mutex  *sync.Mutex
	source netip.AddrPort
	lAddr  net.Addr
	buff   *buf.Buffer
}

func (c *packet) Data() []byte {
	return c.buff.Bytes()
}

// FlowKey returns the source address the tunnel keys this packet's flow by.
func (c *packet) FlowKey() netip.AddrPort {
	return c.source
}

// WriteBack wirtes UDP packet with source(ip, port) = `addr`
func (c *packet) WriteBack(b []byte, addr netip.AddrPort) (n int, err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	conn := *c.writer
	if conn == nil {
		err = errors.New("writeBack to closed connection")
		return
	}

	// sing's writer speaks net.Addr, so the address crosses that one boundary
	return conn.WriteTo(b, net.UDPAddrFromAddrPort(addr))
}

// LocalAddr returns the source IP/Port of UDP Packet
func (c *packet) LocalAddr() net.Addr {
	return net.UDPAddrFromAddrPort(c.source)
}

func (c *packet) Drop() {
	if c.buff == nil {
		return
	}
	buff := c.buff
	c.buff = nil
	buff.Reset()
	singBufferPool.Put(buff)
	singPacketPool.Put(c)
}

func (c *packet) InAddr() net.Addr {
	return c.lAddr
}
