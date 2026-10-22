package tunnel

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/pool"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
)

type packetSender struct {
	ctx    context.Context
	cancel context.CancelFunc
	ch     chan C.PacketAdapter
	closed atomic.Bool
	active atomic.Int64

	mappingMutex   sync.RWMutex
	originToTarget map[packetOrigin]netip.Addr
	targetToOrigin map[netip.Addr]netip.Addr

	// originCache is confined to the Process loop. RestoreReadFrom is part of
	// the public PacketSender interface, so its cache is atomic.
	originCacheKey    packetOrigin
	originCacheTarget netip.Addr
	originCacheValid  bool
	readCache         atomic.Pointer[packetReadMapping]

	// Only the Process goroutine touches this; the read loop keeps its own copy.
	readDeadline int64

	// The destination cache is touched only by the Process goroutine.
	destAddrPort netip.AddrPort
	destAddr     *net.UDPAddr
}

type packetOrigin struct {
	host string
	ip   netip.Addr
}

type packetReadMapping struct {
	target netip.Addr
	origin netip.Addr
}

func packetOriginKey(metadata *C.Metadata) packetOrigin {
	if metadata.Host != "" {
		return packetOrigin{host: metadata.Host}
	}
	return packetOrigin{ip: metadata.DstIP}
}

// newPacketSender return a chan based C.PacketSender
// It ensures that packets can be sent sequentially and without blocking
func newPacketSender() C.PacketSender {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan C.PacketAdapter, senderCapacity)
	sender := &packetSender{
		ctx:            ctx,
		cancel:         cancel,
		ch:             ch,
		originToTarget: make(map[packetOrigin]netip.Addr),
		targetToOrigin: make(map[netip.Addr]netip.Addr),
	}
	return sender
}

func (s *packetSender) AddMapping(originMetadata *C.Metadata, metadata *C.Metadata) {
	s.mappingMutex.Lock()
	defer s.mappingMutex.Unlock()

	originKey := packetOriginKey(originMetadata)
	originAddr := originMetadata.DstIP
	targetAddr := metadata.DstIP

	if target, exists := s.originToTarget[originKey]; !exists || !target.IsValid() {
		s.originToTarget[originKey] = targetAddr
	}
	if origin, exists := s.targetToOrigin[targetAddr]; !exists || !origin.IsValid() {
		s.targetToOrigin[targetAddr] = originAddr
	}
}

func (s *packetSender) RestoreReadFrom(addr netip.Addr) netip.Addr {
	if cached := s.readCache.Load(); cached != nil && addr == cached.target {
		return cached.origin
	}

	s.mappingMutex.RLock()
	originAddr := s.targetToOrigin[addr]
	s.mappingMutex.RUnlock()
	if originAddr.IsValid() {
		s.readCache.Store(&packetReadMapping{target: addr, origin: originAddr})
		return originAddr
	}
	s.readCache.Store(nil)
	return addr
}

func (s *packetSender) targetFor(originKey packetOrigin) netip.Addr {
	if s.originCacheValid && originKey == s.originCacheKey {
		return s.originCacheTarget
	}

	s.mappingMutex.RLock()
	target := s.originToTarget[originKey]
	s.mappingMutex.RUnlock()
	if target.IsValid() {
		s.originCacheKey = originKey
		s.originCacheTarget = target
		s.originCacheValid = true
	} else {
		s.originCacheValid = false
	}
	return target
}

func (s *packetSender) processPacket(pc C.PacketConn, packet C.PacketAdapter) {
	defer packet.Drop()
	metadata := packet.Metadata()

	var addr *net.UDPAddr

	targetAddr := s.targetFor(packetOriginKey(metadata))

	if targetAddr.IsValid() {
		addr = s.destinationFor(targetAddr, metadata.DstPort)
	}

	if addr == nil {
		originMetadata := metadata  // save origin metadata
		metadata = metadata.Clone() // don't modify PacketAdapter's metadata

		_ = preHandleMetadata(metadata) // error was pre-checked
		metadata = metadata.Pure()
		if metadata.Host != "" {
			// TODO: ResolveUDP may take a long time to block the Process loop
			//       but we want keep sequence sending so can't open a new goroutine
			if err := pc.ResolveUDP(s.ctx, metadata); err != nil {
				log.Warnln("[UDP] Resolve Ip error: %s", err)
				return
			}
		}

		if !metadata.DstIP.IsValid() {
			log.Warnln("[UDP] Destination ip not valid: %#v", metadata)
			return
		}
		s.AddMapping(originMetadata, metadata)
		addr = metadata.UDPAddr()
	}
	if err := handleUDPToRemote(packet, pc, addr); err != nil {
		return
	}
	if shouldRefreshDeadline(s.readDeadline, time.Now()) {
		s.readDeadline = refreshReadDeadline(pc, time.Now())
	}
}

// destinationFor returns one *net.UDPAddr per packet, reusing the previous one
// while the destination repeats. A returned object is never mutated: a changed
// destination allocates a fresh one, so a writer that keeps the address past
// WriteTo still observes the value it was handed. A reused value is two
// allocations cheaper per packet.
func (s *packetSender) destinationFor(target netip.Addr, port uint16) *net.UDPAddr {
	addrPort := netip.AddrPortFrom(target, port)
	if s.destAddr != nil && s.destAddrPort == addrPort {
		return s.destAddr
	}
	addr := net.UDPAddrFromAddrPort(addrPort)
	s.destAddrPort = addrPort
	s.destAddr = addr
	return addr
}

func (s *packetSender) Process(pc C.PacketConn, proxy C.WriteBackProxy) {
	for {
		select {
		case <-s.ctx.Done():
			return // sender closed
		case packet := <-s.ch:
			if proxy != nil {
				proxy.UpdateWriteBack(packet)
			}
			s.processPacket(pc, packet)
		}
	}
}

func (s *packetSender) dropAll() {
	for {
		select {
		case data := <-s.ch:
			data.Drop() // drop all data still in chan
		default:
			return // no data, exit goroutine
		}
	}
}

func (s *packetSender) Send(packet C.PacketAdapter) {
	if s.closed.Load() {
		packet.Drop() // sender closed before Send()
		return
	}

	s.active.Add(1)
	if s.closed.Load() {
		s.active.Add(-1)
		packet.Drop() // sender closed during Send()
		return
	}

	queued := false
	select {
	case s.ch <- packet:
		queued = true
	default:
	}
	s.active.Add(-1)
	if !queued {
		packet.Drop() // chan is full
	}
}

func (s *packetSender) Close() {
	if s.closed.Swap(true) {
		return
	}
	for s.active.Load() != 0 {
		runtime.Gosched()
	}
	s.cancel()
	s.dropAll()
}

func (s *packetSender) DoSniff(metadata *C.Metadata) error { return nil }

func handleUDPToRemote(packet C.UDPPacket, pc C.PacketConn, addr *net.UDPAddr) error {
	if addr == nil {
		return errors.New("udp addr invalid")
	}

	if _, err := pc.WriteTo(packet.Data(), addr); err != nil {
		return err
	}

	return nil
}

// shouldRefreshDeadline reports whether the idle deadline tracked by last
// (unix nanoseconds, zero when unset) is closer than half the timeout. A busy
// flow then pays one atomic-free load per packet instead of a runtime deadline
// update, and closes between udpTimeout/2 and udpTimeout after its last packet.
func shouldRefreshDeadline(last int64, now time.Time) bool {
	return last == 0 || now.UnixNano() >= last-int64(udpTimeout/2)
}

func refreshReadDeadline(pc C.PacketConn, now time.Time) int64 {
	next := now.Add(udpTimeout)
	_ = pc.SetReadDeadline(next)
	return next.UnixNano()
}

// idlePacketConn owns a udp flow's idle expiry. Closing the conn is the one
// interruption every outbound packet conn honours: the sing-backed ones
// (hysteria, tuic, shadowquic) cannot report a read deadline, and emulating one
// with a pipe reader costs a goroutine per datagram. The deadline the flow sets
// therefore becomes a timer that closes the conn instead.
type idlePacketConn struct {
	C.PacketConn
	timer *time.Timer
	once  sync.Once
}

func newIdlePacketConn(pc C.PacketConn) *idlePacketConn {
	conn := &idlePacketConn{PacketConn: pc}
	conn.timer = time.AfterFunc(udpTimeout, conn.shutdown)
	return conn
}

func (c *idlePacketConn) SetReadDeadline(t time.Time) error {
	if t.IsZero() {
		c.timer.Stop()
		return nil
	}
	// Resetting an AfterFunc timer cannot recall a callback that already
	// started, which is the same race a socket deadline has when it fires as a
	// packet arrives.
	c.timer.Reset(time.Until(t))
	return nil
}

func (c *idlePacketConn) shutdown() {
	c.once.Do(func() { _ = c.PacketConn.Close() })
}

func (c *idlePacketConn) Close() error {
	c.timer.Stop()
	var err error
	c.once.Do(func() { err = c.PacketConn.Close() })
	return err
}

func (c *idlePacketConn) Upstream() any {
	return c.PacketConn
}

func handleUDPToLocal(
	writeBack C.WriteBack,
	pc C.PacketConn,
	sender C.PacketSender,
	key netip.AddrPort,
	oAddrPort netip.AddrPort,
) {
	defer func() {
		sender.Close()
		_ = pc.Close()
		closeAllLocalCoon(key)
		natTable.DeleteForFlow(key)
	}()

	// The read loop is the only goroutine refreshing on inbound packets, so the
	// tracked deadline needs no synchronisation.
	var readDeadline int64
	for {
		now := time.Now()
		if shouldRefreshDeadline(readDeadline, now) {
			readDeadline = refreshReadDeadline(pc, now)
		}

		data, fromAddrPort, err := pc.WaitReadFrom()
		if err != nil {
			return
		}

		if !fromAddrPort.IsValid() {
			log.Warnln("server return an invalid address, force replace to (%s), this may be caused by a wrongly implemented server", oAddrPort)
			fromAddrPort = oAddrPort // oAddrPort was Unmapped
		}

		fromAddr := fromAddrPort.Addr().Unmap()

		// restore DestinationNAT
		fromAddr = sender.RestoreReadFrom(fromAddr).Unmap()

		fromAddrPort = netip.AddrPortFrom(fromAddr, fromAddrPort.Port())

		_, err = writeBack.WriteBack(data, fromAddrPort)
		if data != nil {
			_ = pool.Put(data)
		}
		if err != nil {
			return
		}
	}
}

func closeAllLocalCoon(lAddr netip.AddrPort) {
	natTable.RangeForLocalConnForFlow(lAddr, func(key netip.AddrPort, value *net.UDPConn) bool {
		conn := value

		conn.Close()
		log.Debugln("Closing TProxy local conn... lAddr=%s rAddr=%s", lAddr, key)
		return true
	})
}

func handleSocket(inbound, outbound net.Conn) {
	N.Relay(inbound, outbound)
}
