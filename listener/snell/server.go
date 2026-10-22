package snell

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"

	"github.com/metacubex/mihomo/adapter/inbound"
	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/pool"
	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	LC "github.com/metacubex/mihomo/listener/config"
	"github.com/metacubex/mihomo/listener/jls"
	"github.com/metacubex/mihomo/listener/restls"
	"github.com/metacubex/mihomo/listener/shadowtls"
	"github.com/metacubex/mihomo/transport/shadowsocks/shadowaead"
	obfs "github.com/metacubex/mihomo/transport/simple-obfs"
	"github.com/metacubex/mihomo/transport/snell"
)

const maxPacketLength = 0x3fff
const snellUDPFlowCacheSize = 256

var snellPacketPool = sync.Pool{New: func() interface{} { return new(udpPacket) }}

type Listener struct {
	closed    bool
	config    LC.SnellServer
	listeners []net.Listener
}

func New(config LC.SnellServer, lc C.InboundListenConfig, tunnel C.Tunnel, additions ...inbound.Addition) (C.MultiAddrListener, error) {
	if config.Version == 0 {
		config.Version = snell.Version4
	}
	switch config.Version {
	case snell.Version1, snell.Version2, snell.Version3, snell.Version4, snell.Version5:
	default:
		return nil, fmt.Errorf("snell inbound version %d is not supported", config.Version)
	}
	if config.Psk == "" {
		return nil, errors.New("snell inbound requires psk")
	}
	switch config.ObfsMode {
	case "", "http", "tls":
	default:
		return nil, fmt.Errorf("snell inbound obfs mode error: %s", config.ObfsMode)
	}

	l := &Listener{config: config}

	securityModes := make([]string, 0, 3)
	if config.ShadowTLS.Enable {
		securityModes = append(securityModes, "shadow-tls")
	}
	if config.ResTLS.Enable {
		securityModes = append(securityModes, "res-tls")
	}
	if config.JLSConfig.Enable {
		securityModes = append(securityModes, "jls")
	}
	if len(securityModes) > 1 {
		return nil, errors.New("security modes are mutually exclusive: " + strings.Join(securityModes, ", "))
	}

	var shadowTLSBuilder *shadowtls.Builder
	if config.ShadowTLS.Enable {
		var err error
		shadowTLSBuilder, err = shadowtls.New(config.ShadowTLS, tunnel)
		if err != nil {
			return nil, err
		}
	}
	var restlsBuilder *restls.Builder
	if config.ResTLS.Enable {
		restlsBuilder = restls.New(config.ResTLS, tunnel)
	}
	var jlsBuilder *jls.Builder
	if config.JLSConfig.Enable {
		var err error
		jlsBuilder, err = jls.New(config.JLSConfig, tunnel)
		if err != nil {
			return nil, err
		}
	}
	for _, addr := range strings.Split(config.Listen, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		ln, err := lc.Listen(context.Background(), "tcp", addr)
		if err != nil {
			_ = l.Close()
			return nil, err
		}
		if shadowTLSBuilder != nil {
			ln = shadowTLSBuilder.NewListener(ln)
		} else if restlsBuilder != nil {
			ln = restlsBuilder.NewListener(ln)
		} else if jlsBuilder != nil {
			ln = jlsBuilder.NewListener(ln)
		}
		l.listeners = append(l.listeners, ln)
		go func(ln net.Listener) {
			for {
				conn, err := ln.Accept()
				if err != nil {
					if l.closed {
						return
					}
					continue
				}
				go l.HandleConn(conn, tunnel, additions...)
			}
		}(ln)
	}
	return l, nil
}

func (l *Listener) Close() error {
	l.closed = true
	var retErr error
	for _, ln := range l.listeners {
		if err := ln.Close(); err != nil {
			retErr = err
		}
	}
	return retErr
}

func (l *Listener) Config() string {
	return l.config.String()
}

func (l *Listener) AddrList() (addrList []net.Addr) {
	for _, ln := range l.listeners {
		addrList = append(addrList, ln.Addr())
	}
	return
}

func (l *Listener) HandleConn(rawConn net.Conn, tunnel C.Tunnel, additions ...inbound.Addition) {
	defer rawConn.Close()
	user, loaded := shadowtls.UserFromConn(rawConn)
	if jlsUser, jlsLoaded := jls.UserFromConn(rawConn); jlsLoaded {
		user, loaded = jlsUser, true
	}
	if loaded {
		additions = append(append([]inbound.Addition(nil), additions...), inbound.WithInUser(user))
	}
	conn := rawConn
	switch l.config.ObfsMode {
	case "http":
		conn = obfs.NewHTTPObfsServer(conn)
	case "tls":
		conn = obfs.NewTLSObfsServer(conn)
	case "":
	default:
		return
	}
	stream := snell.ServerStreamConn(conn, []byte(l.config.Psk), l.config.Version)
	for {
		reuse, err := l.handleRequest(stream, tunnel, additions...)
		if err != nil || !reuse {
			return
		}
	}
}

func (l *Listener) handleRequest(stream *snell.Snell, tunnel C.Tunnel, additions ...inbound.Addition) (bool, error) {
	br := bufio.NewReader(stream)
	version, err := br.ReadByte()
	if err != nil {
		return false, err
	}
	if version != snell.Version {
		return false, fmt.Errorf("snell invalid protocol version: %d", version)
	}

	command, err := br.ReadByte()
	if err != nil {
		return false, err
	}
	if command == snell.CommandPing {
		_, _ = stream.Write([]byte{snell.CommandPong})
		return false, nil
	}

	clientID, err := readClientID(br)
	if err != nil {
		return false, err
	}

	switch command {
	case snell.CommandConnect, snell.CommandConnectV2:
		return l.handleTCP(stream, br, command == snell.CommandConnectV2, clientID, tunnel, additions...)
	case snell.CommandUDP:
		if !l.config.UDP {
			return false, errors.New("snell UDP is disabled")
		}
		return false, l.handleUDP(stream, clientID, tunnel, additions...)
	default:
		return false, fmt.Errorf("snell unknown command: %d", command)
	}
}

func readClientID(r *bufio.Reader) (string, error) {
	length, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	if length == 0 {
		return "", nil
	}
	id := make([]byte, int(length))
	if _, err := io.ReadFull(r, id); err != nil {
		return "", err
	}
	return string(id), nil
}

func (l *Listener) handleTCP(stream *snell.Snell, br *bufio.Reader, reuse bool, clientID string, tunnel C.Tunnel, additions ...inbound.Addition) (bool, error) {
	hostLen, err := br.ReadByte()
	if err != nil {
		return false, err
	}
	if hostLen == 0 {
		return false, errors.New("snell connect host is empty")
	}

	hostBytes := make([]byte, int(hostLen))
	if _, err := io.ReadFull(br, hostBytes); err != nil {
		return false, err
	}
	var portBytes [2]byte
	if _, err := io.ReadFull(br, portBytes[:]); err != nil {
		return false, err
	}

	metadata := l.metadata(C.TCP, string(hostBytes), binary.BigEndian.Uint16(portBytes[:]), stream, clientID, additions...)
	conn := &tcpRequestConn{
		Conn:   stream,
		reader: br,
		reuse:  reuse,
	}
	tunnel.HandleTCPConn(conn, metadata)
	if !reuse {
		return false, nil
	}
	return true, nil
}

func (l *Listener) handleUDP(stream *snell.Snell, clientID string, tunnel C.Tunnel, additions ...inbound.Addition) error {
	if _, err := stream.Write([]byte{snell.CommandTunnel}); err != nil {
		return err
	}

	connID := utils.NewUUIDV4().String()
	localAddr := N.NewCustomAddr(C.SNELL.String(), connID, stream.RemoteAddr())
	source := N.AddrPortFromNetAddr(stream.RemoteAddr())
	writeMu := &sync.Mutex{}
	session := snellUDPSession{
		listener:  l,
		stream:    stream,
		clientID:  clientID,
		source:    source,
		localAddr: localAddr,
		writeMu:   writeMu,
		tunnel:    tunnel,
		additions: additions,
	}
	buf := make([]byte, maxPacketLength)
	for {
		n, err := stream.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, shadowaead.ErrZeroChunk) {
				return nil
			}
			return err
		}
		request, err := snell.ParseUDPRequest(buf[:n])
		if err != nil {
			return err
		}
		session.handle(request.Payload, request)
	}
}

type snellFlowKey struct {
	host string
	ip   netip.Addr
	port uint16
}

type snellUDPSession struct {
	listener  *Listener
	stream    *snell.Snell
	clientID  string
	source    netip.AddrPort
	localAddr net.Addr
	writeMu   *sync.Mutex
	tunnel    C.Tunnel
	additions []inbound.Addition
	flows     map[snellFlowKey]*C.Metadata
}

func (s *snellUDPSession) handle(payload []byte, request snell.UDPRequest) {
	key := snellFlowKey{host: request.Host, ip: request.Ip, port: request.Port}
	metadata, ok := s.flows[key]
	if !ok {
		if s.flows == nil {
			s.flows = make(map[snellFlowKey]*C.Metadata, 8)
		} else if len(s.flows) >= snellUDPFlowCacheSize {
			clear(s.flows)
		}
		metadata = s.listener.metadata(C.UDP, request.Host, request.Port, s.stream, s.clientID, s.additions...)
		if request.Ip.IsValid() {
			metadata.Host = ""
			metadata.DstIP = request.Ip
		}
		s.flows[key] = metadata
	}

	packet := newSnellPacket(s.stream, s.writeMu, s.localAddr, s.source, pool.Get(len(payload)))
	copy(packet.data, payload)
	s.tunnel.HandleUDPPacket(packet, metadata)
}

func (l *Listener) metadata(network C.NetWork, host string, port uint16, conn net.Conn, clientID string, additions ...inbound.Addition) *C.Metadata {
	metadata := &C.Metadata{
		NetWork: network,
		Type:    C.SNELL,
		DstPort: port,
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		metadata.DstIP = ip.Unmap()
	} else {
		metadata.Host = host
	}
	inbound.ApplyAdditions(metadata, inbound.WithSrcAddr(conn.RemoteAddr()), inbound.WithInAddr(conn.LocalAddr()))
	inbound.ApplyAdditions(metadata, additions...)
	if clientID != "" {
		inbound.ApplyAdditions(metadata, inbound.WithInUser(clientID))
	}
	return metadata
}

func writeCommandError(w io.Writer, code byte, message string) error {
	msg := []byte(message)
	if len(msg) > 255 {
		msg = msg[:255]
	}
	buf := make([]byte, 0, 3+len(msg))
	buf = append(buf, snell.CommandError, code, byte(len(msg)))
	buf = append(buf, msg...)
	_, err := w.Write(buf)
	return err
}

type tcpRequestConn struct {
	net.Conn
	reader       *bufio.Reader
	reuse        bool
	writeMu      sync.Mutex
	closeOnce    sync.Once
	replyWritten bool
}

func (c *tcpRequestConn) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	if errors.Is(err, shadowaead.ErrZeroChunk) {
		err = io.EOF
	}
	return n, err
}

func (c *tcpRequestConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if !c.replyWritten {
		payload := make([]byte, 1+len(p))
		payload[0] = snell.CommandTunnel
		copy(payload[1:], p)
		if _, err := c.Conn.Write(payload); err != nil {
			return 0, err
		}
		c.replyWritten = true
		return len(p), nil
	}
	return c.Conn.Write(p)
}

func (c *tcpRequestConn) CloseWrite() error {
	return c.Close()
}

func (c *tcpRequestConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.writeMu.Lock()
		defer c.writeMu.Unlock()

		if !c.replyWritten {
			err = writeCommandError(c.Conn, 0x65, "Remote EOF")
			if !c.reuse {
				err = errors.Join(err, c.Conn.Close())
			}
			return
		}
		if c.reuse {
			_, err = c.Conn.Write(nil)
			return
		}
		err = c.Conn.Close()
	})
	return err
}

type udpPacket struct {
	data      []byte
	conn      *snell.Snell
	writeMu   *sync.Mutex
	localAddr net.Addr
	source    netip.AddrPort
}

func newSnellPacket(conn *snell.Snell, writeMu *sync.Mutex, localAddr net.Addr, source netip.AddrPort, data []byte) *udpPacket {
	packet := snellPacketPool.Get().(*udpPacket)
	*packet = udpPacket{
		data:      data,
		conn:      conn,
		writeMu:   writeMu,
		localAddr: localAddr,
		source:    source,
	}
	return packet
}

func (p *udpPacket) Data() []byte {
	return p.data
}

func (p *udpPacket) WriteBack(b []byte, addr netip.AddrPort) (int, error) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return snell.WritePacketResponse(p.conn, net.UDPAddrFromAddrPort(addr), b)
}

func (p *udpPacket) Drop() {
	if p.data == nil {
		return
	}
	_ = pool.Put(p.data)
	p.data = nil
	p.conn = nil
	p.writeMu = nil
	p.localAddr = nil
	p.source = netip.AddrPort{}
	snellPacketPool.Put(p)
}

func (p *udpPacket) LocalAddr() net.Addr {
	return p.localAddr
}

func (p *udpPacket) FlowKey() netip.AddrPort {
	return p.source
}

var _ C.MultiAddrListener = (*Listener)(nil)
var _ C.UDPPacket = (*udpPacket)(nil)
