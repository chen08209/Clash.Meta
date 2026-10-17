package tunnel

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	N "github.com/metacubex/mihomo/common/net"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
)

type packetSender struct {
	ctx    context.Context
	cancel context.CancelFunc
	ch     chan C.PacketAdapter

	mappingMutex sync.Mutex
	mappings     atomic.Pointer[packetMappings]
}

type packetOrigin struct {
	host string
	ip   netip.Addr
}

type packetMappings struct {
	originToTarget map[packetOrigin]netip.Addr
	targetToOrigin map[netip.Addr]netip.Addr
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
		ctx:    ctx,
		cancel: cancel,
		ch:     ch,
	}
	sender.mappings.Store(&packetMappings{
		originToTarget: make(map[packetOrigin]netip.Addr),
		targetToOrigin: make(map[netip.Addr]netip.Addr),
	})
	return sender
}

func clonePacketMap[M ~map[K]V, K comparable, V any](source M) M {
	result := make(M, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (s *packetSender) AddMapping(originMetadata *C.Metadata, metadata *C.Metadata) {
	s.mappingMutex.Lock()
	defer s.mappingMutex.Unlock()
	current := s.mappings.Load()
	originKey := packetOriginKey(originMetadata)
	originAddr := originMetadata.DstIP
	targetAddr := metadata.DstIP
	originValid := current.originToTarget[originKey].IsValid()
	targetValid := current.targetToOrigin[targetAddr].IsValid()
	if originValid && targetValid {
		return
	}
	next := &packetMappings{
		originToTarget: clonePacketMap(current.originToTarget),
		targetToOrigin: clonePacketMap(current.targetToOrigin),
	}
	if !originValid {
		next.originToTarget[originKey] = targetAddr
	}
	if !targetValid {
		next.targetToOrigin[targetAddr] = originAddr
	}
	s.mappings.Store(next)
}

func (s *packetSender) RestoreReadFrom(addr netip.Addr) netip.Addr {
	if originAddr := s.mappings.Load().targetToOrigin[addr]; originAddr.IsValid() {
		return originAddr
	}
	return addr
}

func (s *packetSender) processPacket(pc C.PacketConn, packet C.PacketAdapter) {
	defer packet.Drop()
	metadata := packet.Metadata()

	var addr *net.UDPAddr

	targetAddr := s.mappings.Load().originToTarget[packetOriginKey(metadata)]

	if targetAddr.IsValid() {
		addr = net.UDPAddrFromAddrPort(netip.AddrPortFrom(targetAddr, metadata.DstPort))
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
	_ = handleUDPToRemote(packet, pc, addr)
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
	select {
	case <-s.ctx.Done():
		packet.Drop() // sender closed before Send()
		return
	default:
	}

	select {
	case s.ch <- packet:
		// put ok, so don't drop packet, will process by other side of chan
	case <-s.ctx.Done():
		packet.Drop() // sender closed when putting data to chan
	default:
		packet.Drop() // chan is full
	}
}

func (s *packetSender) Close() {
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
	// reset timeout
	_ = pc.SetReadDeadline(time.Now().Add(udpTimeout))

	return nil
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

	for {
		_ = pc.SetReadDeadline(time.Now().Add(udpTimeout))
		data, put, from, err := pc.WaitReadFrom()
		if err != nil {
			return
		}

		fromUDPAddr, isUDPAddr := from.(*net.UDPAddr)
		if !isUDPAddr {
			fromUDPAddr = net.UDPAddrFromAddrPort(oAddrPort) // oAddrPort was Unmapped
			log.Warnln("server return a [%T](%s) which isn't a *net.UDPAddr, force replace to (%s), this may be caused by a wrongly implemented server", from, from, oAddrPort)
		} else if fromUDPAddr == nil {
			fromUDPAddr = net.UDPAddrFromAddrPort(oAddrPort) // oAddrPort was Unmapped
			log.Warnln("server return a nil *net.UDPAddr, force replace to (%s), this may be caused by a wrongly implemented server", oAddrPort)
		}

		fromAddrPort := fromUDPAddr.AddrPort()
		fromAddr := fromAddrPort.Addr().Unmap()

		// restore DestinationNAT
		fromAddr = sender.RestoreReadFrom(fromAddr).Unmap()

		fromAddrPort = netip.AddrPortFrom(fromAddr, fromAddrPort.Port())

		_, err = writeBack.WriteBack(data, net.UDPAddrFromAddrPort(fromAddrPort))
		if put != nil {
			put()
		}
		if err != nil {
			return
		}
	}
}

func closeAllLocalCoon(lAddr netip.AddrPort) {
	natTable.RangeForLocalConnForFlow(lAddr, func(key string, value *net.UDPConn) bool {
		conn := value

		conn.Close()
		log.Debugln("Closing TProxy local conn... lAddr=%s rAddr=%s", lAddr, key)
		return true
	})
}

func handleSocket(inbound, outbound net.Conn) {
	N.Relay(inbound, outbound)
}
