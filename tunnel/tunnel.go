package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/metacubex/mihomo/common/atomic"
	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/component/dialer"
	"github.com/metacubex/mihomo/component/loopback"
	"github.com/metacubex/mihomo/component/nat"
	"github.com/metacubex/mihomo/component/process"
	"github.com/metacubex/mihomo/component/proxydialer"
	"github.com/metacubex/mihomo/component/resolver"
	"github.com/metacubex/mihomo/component/slowdown"
	"github.com/metacubex/mihomo/component/sniffer"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel/statistic"

	"slices"
)

const (
	queueCapacity  = 64  // chan capacity of the UDP worker queues
	senderCapacity = 128 // chan capacity of PacketSender
)

var (
	status    = atomic.NewInt32Enum(Suspend)
	udpInit   sync.Once
	udpQueues []chan udpPacket
	natTable  = nat.New()

	// default timeout for UDP session
	udpTimeout = 60 * time.Second

	findProcessMode = atomic.NewInt32Enum(process.FindProcessStrict)

	ruleUpdateCallback = utils.NewCallback[P.RuleProvider]()
)

type tunnel struct{}

type udpPacket struct {
	packet C.PacketAdapter
	key    netip.AddrPort
}

var Tunnel = tunnel{}
var _ C.Tunnel = Tunnel
var _ P.Tunnel = Tunnel
var _ proxydialer.Tunnel = Tunnel

func (t tunnel) HandleTCPConn(conn net.Conn, metadata *C.Metadata) {
	handleTCPConn(conn, metadata)
}

func initUDP() {
	numUDPWorkers := 4
	if num := runtime.GOMAXPROCS(0); num > numUDPWorkers {
		numUDPWorkers = num
	}

	udpQueues = make([]chan udpPacket, numUDPWorkers)
	for i := 0; i < numUDPWorkers; i++ {
		queue := make(chan udpPacket, queueCapacity)
		udpQueues[i] = queue
		go processUDP(queue)
	}
}

func (t tunnel) HandleUDPPacket(packet C.UDPPacket, metadata *C.Metadata) {
	udpInit.Do(initUDP)

	packetAdapter := C.NewPacketAdapter(packet, metadata)
	key, valid := packetFlowKey(packetAdapter)
	if !valid {
		packet.Drop()
		return
	}

	hash := utils.MapHashAddrPort(key)
	queueNo := uint(hash) % uint(len(udpQueues))

	select {
	case udpQueues[queueNo] <- udpPacket{packet: packetAdapter, key: key}:
	default:
		packet.Drop()
	}
}

func (t tunnel) NatTable() C.NatTable {
	return natTable
}

func (t tunnel) Proxies() map[string]C.Proxy {
	return loadRuntime().proxies
}

func (t tunnel) Providers() map[string]P.ProxyProvider {
	return loadRuntime().providers
}

func (t tunnel) RuleProviders() map[string]P.RuleProvider {
	return loadRuntime().ruleProviders
}

func (t tunnel) RuleUpdateCallback() *utils.Callback[P.RuleProvider] {
	return ruleUpdateCallback
}

func OnSuspend() {
	status.Store(Suspend)
}

func OnInnerLoading() {
	status.Store(Inner)
}

func OnRunning() {
	status.Store(Running)
}

func Status() TunnelStatus {
	return status.Load()
}

func SetSniffing(b bool) {
	updateRuntime(func(next *runtimeSnapshot) {
		if next.snifferDispatcher != nil && next.snifferDispatcher.Enable() {
			next.sniffingEnable = b
		}
	})
}

func IsSniffing() bool {
	return loadRuntime().sniffingEnable
}

// NatTable return nat table
func NatTable() C.NatTable {
	return natTable
}

// Rules return all rules
func Rules() []C.Rule {
	return loadRuntime().rules
}

func Listeners() map[string]C.InboundListener {
	return loadRuntime().listeners
}

// UpdateRules handle update rules
func UpdateRules(newRules []C.Rule, newSubRule map[string][]C.Rule, rp map[string]P.RuleProvider) {
	updateRuntime(func(next *runtimeSnapshot) {
		next.rules = cloneRules(newRules)
		next.ruleIndex = buildRuleIndex(next.rules)
		next.ruleProviders = cloneMap(rp)
		next.subRules = cloneSubRules(newSubRule)
		next.subRuleIndexes = buildSubRuleIndexes(next.subRules)
	})
}

// Proxies return all proxies
func Proxies() map[string]C.Proxy {
	return loadRuntime().proxies
}

// Providers return all compatible providers
func Providers() map[string]P.ProxyProvider {
	return loadRuntime().providers
}

// RuleProviders return all loaded rule providers
func RuleProviders() map[string]P.RuleProvider {
	return loadRuntime().ruleProviders
}

// UpdateProxies handle update proxies
func UpdateProxies(newProxies map[string]C.Proxy, newProviders map[string]P.ProxyProvider) {
	configMux.Lock()
	next := cloneRuntime(loadRuntime())
	next.proxies = cloneMap(newProxies)
	next.providers = cloneMap(newProviders)
	currentRuntime.Store(next)
	invalidateAllProxies()
	configMux.Unlock()
}

func UpdateListeners(newListeners map[string]C.InboundListener) {
	updateRuntime(func(next *runtimeSnapshot) {
		next.listeners = cloneMap(newListeners)
	})
}

func UpdateSniffer(dispatcher *sniffer.Dispatcher) {
	updateRuntime(func(next *runtimeSnapshot) {
		next.snifferDispatcher = dispatcher
		next.sniffingEnable = dispatcher.Enable()
	})
}

// Mode return current mode
func Mode() TunnelMode {
	return loadRuntime().mode
}

// SetMode change the mode of tunnel
func SetMode(m TunnelMode) {
	updateRuntime(func(next *runtimeSnapshot) {
		next.mode = m
	})
}

func FindProcessMode() process.FindProcessMode {
	return findProcessMode.Load()
}

// SetFindProcessMode replace SetAlwaysFindProcess
// always find process info if legacyAlways = true or mode.Always() = true, may be increase many memory
func SetFindProcessMode(mode process.FindProcessMode) {
	findProcessMode.Store(mode)
}

func isHandle(t C.Type) bool {
	status := status.Load()
	return status == Running || (status == Inner && t == C.INNER)
}

func fixMetadata(metadata *C.Metadata) {
	// first unmap dstIP
	metadata.DstIP = metadata.DstIP.Unmap()
	// handle IP string on host
	// Parsing an empty host allocates an error, so only pay for it when set.
	if metadata.Host != "" {
		if ip, err := netip.ParseAddr(metadata.Host); err == nil {
			metadata.DstIP = ip.Unmap()
			metadata.Host = ""
		}
	}
}

func needLookupIP(metadata *C.Metadata) bool {
	return resolver.MappingEnabled() && metadata.Host == "" && metadata.DstIP.IsValid()
}

func preHandleMetadata(metadata *C.Metadata) error {
	// preprocess enhanced-mode metadata
	if needLookupIP(metadata) {
		host, exist := resolver.FindHostByIP(metadata.DstIP)
		if exist {
			metadata.Host = host
			metadata.DNSMode = C.DNSMapping
			if resolver.IsFakeIP(metadata.DstIP) {
				// only clear dstIP if it is confirmed to be a fake IP
				metadata.DstIP = netip.Addr{}
				metadata.DNSMode = C.DNSFakeIP
			} else if node, ok := resolver.DefaultHosts.Search(host, false); ok {
				// redir-host should lookup the hosts
				metadata.DstIP, _ = node.RandIP()
			} else if node != nil && node.IsDomain {
				metadata.Host = node.Domain
			}
		} else if resolver.IsFakeIP(metadata.DstIP) {
			return fmt.Errorf("fake DNS record %s missing", metadata.DstIP)
		}
	} else if metadata.Host != "" {
		if node, ok := resolver.DefaultHosts.Search(metadata.Host, true); ok {
			// try use domain mapping
			metadata.Host = node.Domain
		}
	}

	return nil
}

func resolveMetadata(metadata *C.Metadata) (proxy C.Proxy, rule C.Rule, err error) {
	runtimeState := loadRuntime()
	if metadata.SpecialProxy != "" {
		var exist bool
		proxy, exist = runtimeState.proxies[metadata.SpecialProxy]
		if !exist {
			err = fmt.Errorf("proxy %s not found", metadata.SpecialProxy)
		}
		return
	}
	hostsResolved := false
	findProcessMode := FindProcessMode()
	processLookupEnabled := findProcessMode == process.FindProcessStrict
	if metadata.Host != "" {
		if node, ok := resolver.DefaultHosts.Search(metadata.Host, false); ok {
			metadata.DstIP, _ = node.RandIP()
			hostsResolved = true
		}
	}

	switch findProcessMode {
	case process.FindProcessAlways:
		processContext := &matchContext{
			metadata:             metadata,
			runtimeState:         runtimeState,
			resolved:             hostsResolved,
			attemptProcessLookup: metadata.Type != C.INNER,
		}
		processContext.findProcess()
	}

	switch runtimeState.mode {
	case Direct:
		proxy = runtimeState.proxies["DIRECT"]
	case Global:
		proxy = runtimeState.proxies["GLOBAL"]
	// Rule
	default:
		proxy, rule, err = match(
			metadata,
			runtimeState,
			hostsResolved,
			processLookupEnabled,
		)
	}
	return
}

// processUDP starts a loop to handle udp packet
func processUDP(queue chan udpPacket) {
	for conn := range queue {
		handleUDPConn(conn)
	}
}

func handleUDPConn(item udpPacket) {
	packet := item.packet
	if !isHandle(packet.Metadata().Type) {
		packet.Drop()
		return
	}

	metadata := packet.Metadata()
	if !metadata.Valid() {
		packet.Drop()
		log.Warnln("[Metadata] not valid: %#v", metadata)
		return
	}
	// An existing flow already passed precheck on its first packet; repeating it
	// per packet only clones metadata and re-queries the resolvers.
	if sender, exist := natTable.LoadForFlow(item.key); exist {
		sender.Send(packet) // nonblocking
		return
	}

	runtimeState := loadRuntime()
	fixMetadata(metadata) // fix some metadata not set via metadata.SetRemoteAddr or metadata.SetRemoteAddress

	if err := preHandleMetadata(metadata.Clone()); err != nil { // precheck without modify metadata
		packet.Drop()
		log.Debugln("[Metadata PreHandle] error: %s", err)
		return
	}

	sender, loaded := natTable.GetOrCreateForFlow(item.key, func() C.PacketSender {
		sender := newPacketSender()
		if runtimeState.sniffingEnable && runtimeState.snifferDispatcher.Enable() {
			return runtimeState.snifferDispatcher.UDPSniff(packet, sender)
		}
		return sender
	})
	if !loaded {
		// The dial runs on its own goroutine, so it gets a private metadata
		// clone and an origin snapshot. Later datagrams of the same flow may
		// reuse one metadata object in place, and that object must not be read
		// again once this worker has moved on.
		dialMetadata := metadata.Clone()
		originMetadata := &C.Metadata{
			Host:    metadata.Host,
			DstIP:   metadata.DstIP,
			DstPort: metadata.DstPort,
		}
		go startUDPSender(originMetadata, dialMetadata, packet, sender, item.key)
	}
	sender.Send(packet) // nonblocking
}

// Kept as functions taking parameters rather than closures: a closure capturing
// the per-packet metadata variable forces it onto the heap on every packet.
func startUDPSender(originMetadata *C.Metadata, dialMetadata *C.Metadata, packet C.PacketAdapter, sender C.PacketSender, key netip.AddrPort) {
	pc, writeBackProxy, err := dialUDPSender(originMetadata, dialMetadata, packet, sender, key)
	if err != nil {
		sender.Close()
		natTable.DeleteForFlow(key)
		return
	}
	sender.Process(pc, writeBackProxy)
}

// dialUDPSender receives a metadata clone the worker made before the dial went
// async, plus an origin snapshot: neither is shared with later datagrams.
func dialUDPSender(originMetadata *C.Metadata, metadata *C.Metadata, packet C.PacketAdapter, sender C.PacketSender, key netip.AddrPort) (C.PacketConn, C.WriteBackProxy, error) {
	if err := sender.DoSniff(metadata); err != nil {
		log.Warnln("[UDP] DoSniff error: %s", err.Error())
		return nil, nil, err
	}

	_ = preHandleMetadata(metadata) // error was pre-checked

	proxy, rule, err := resolveMetadata(metadata)
	if err != nil {
		log.Warnln("[UDP] Parse metadata failed: %s", err.Error())
		return nil, nil, err
	}

	dialMetadata := metadata.Pure()
	ctx, cancel := context.WithTimeout(context.Background(), C.DefaultUDPTimeout)
	defer cancel()
	rawPc, err := retry(ctx, func(ctx context.Context) (C.PacketConn, error) {
		return proxy.ListenPacketContext(ctx, dialMetadata)
	}, func(err error) {
		logMetadataErr(metadata, rule, proxy, err)
	})
	if err != nil {
		return nil, nil, err
	}
	logMetadata(metadata, rule, rawPc)

	pc := statistic.NewUDPTracker(rawPc, statistic.DefaultManager, metadata, rule, 0, 0, true)

	sender.AddMapping(originMetadata, dialMetadata)
	oAddrPort := dialMetadata.AddrPort()
	writeBackProxy := nat.NewWriteBackProxy(packet)

	go handleUDPToLocal(writeBackProxy, pc, sender, key, oAddrPort)
	return pc, writeBackProxy, nil
}

func handleTCPConn(conn net.Conn, metadata *C.Metadata) {
	runtimeState := loadRuntime()
	if !isHandle(metadata.Type) {
		_ = conn.Close()
		return
	}

	originConn := conn
	defer func() {
		_ = originConn.Close()
	}()

	if !metadata.Valid() {
		log.Warnln("[Metadata] not valid: %#v", metadata)
		return
	}
	fixMetadata(metadata) // fix some metadata not set via metadata.SetRemoteAddr or metadata.SetRemoteAddress

	preHandleFailed := false
	if err := preHandleMetadata(metadata); err != nil {
		log.Debugln("[Metadata PreHandle] error: %s", err)
		preHandleFailed = true
	}

	bufferedConn := func() *N.BufferedConn {
		if buffered, ok := conn.(*N.BufferedConn); ok {
			return buffered
		}
		buffered := N.NewBufferedConn(conn)
		conn = buffered
		return buffered
	}
	if runtimeState.sniffingEnable && runtimeState.snifferDispatcher.Enable() {
		buffered := bufferedConn()
		buffered.ResetPeeked() // reset before sniffer
		// Try to sniff a domain when `preHandleMetadata` failed, this is usually
		// caused by a "Fake DNS record missing" error when enhanced-mode is fake-ip.
		if runtimeState.snifferDispatcher.TCPSniff(buffered, metadata) {
			// we now have a domain name
			preHandleFailed = false
		}
	}

	// If both trials have failed, we can do nothing but give up
	if preHandleFailed {
		log.Debugln("[Metadata PreHandle] failed to sniff a domain for connection %s --> %s, give up",
			metadata.SourceDetail(), metadata.RemoteAddress())
		return
	}

	proxy, rule, err := resolveMetadata(metadata)
	if err != nil {
		log.Warnln("[Metadata] parse failed: %s", err.Error())
		return
	}

	dialMetadata := metadata
	if len(metadata.Host) > 0 {
		if node, ok := resolver.DefaultHosts.Search(metadata.Host, false); ok {
			if dstIp, _ := node.RandIP(); !resolver.IsFakeIP(dstIp) {
				dialMetadata.DstIP = dstIp
				dialMetadata.DNSMode = C.DNSHosts
				dialMetadata = dialMetadata.Pure()
			}
		}
	}

	var peekBytes []byte
	var peekLen int

	ctx, cancel := context.WithTimeout(context.Background(), C.DefaultTCPTimeout)
	defer cancel()
	remoteConn, err := retry(ctx, func(ctx context.Context) (remoteConn C.Conn, err error) {
		remoteConn, err = proxy.DialContext(ctx, dialMetadata)
		if err != nil {
			return
		}

		if N.NeedHandshake(remoteConn) {
			buffered := bufferedConn()
			defer func() {
				if err != nil {
					_ = remoteConn.Close()
					for _, chain := range remoteConn.Chains() {
						if chain == "REJECT" {
							err = nil
							return
						}
					}
					remoteConn = nil
				}
			}()
			if !buffered.Peeked() {
				// The remote conn exists, so the buffer can be filled now and the
				// client's hello handed straight to it. A plain relay has no use
				// for the peek and waits on its own first read.
				_ = buffered.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
				_, _ = buffered.Peek(1)
				_ = buffered.SetReadDeadline(time.Time{})
			}
			peekBytes, _ = buffered.Peek(buffered.Buffered())
			_, err = remoteConn.Write(peekBytes)
			if err != nil {
				return
			}
			if peekLen = len(peekBytes); peekLen > 0 {
				_, _ = buffered.Discard(peekLen)
			}
		}
		return
	}, func(err error) {
		logMetadataErr(metadata, rule, proxy, err)
	})
	if err != nil {
		return
	}
	logMetadata(metadata, rule, remoteConn)

	remoteConn = statistic.NewTCPTracker(remoteConn, statistic.DefaultManager, metadata, rule, int64(peekLen), 0, true)
	defer func(remoteConn C.Conn) {
		_ = remoteConn.Close()
	}(remoteConn)

	_ = conn.SetReadDeadline(time.Time{}) // reset
	handleSocket(conn, remoteConn)
}

func logMetadataErr(metadata *C.Metadata, rule C.Rule, proxy C.ProxyAdapter, err error) {
	if rule == nil {
		log.Warnln("[%s] dial %s %s --> %s error: %s", strings.ToUpper(metadata.NetWork.String()), proxy.Name(), metadata.SourceDetail(), metadata.RemoteAddress(), err.Error())
	} else {
		log.Warnln("[%s] dial %s (match %s/%s) %s --> %s error: %s", strings.ToUpper(metadata.NetWork.String()), proxy.Name(), rule.RuleType().String(), rule.Payload(), metadata.SourceDetail(), metadata.RemoteAddress(), err.Error())
	}
}

func logMetadata(
	metadata *C.Metadata,
	rule C.Rule,
	remoteConn C.Connection,
) {
	if !log.InfoEnabled() {
		return // every branch below only builds log strings
	}
	mode := loadRuntime().mode
	switch {
	case metadata.SpecialProxy != "":
		log.Infoln("[%s] %s --> %s using %s", strings.ToUpper(metadata.NetWork.String()), metadata.SourceDetail(), metadata.RemoteAddress(), remoteConn.Chains().String())
	case rule != nil:
		if rule.Payload() != "" {
			log.Infoln("[%s] %s --> %s match %s using %s", strings.ToUpper(metadata.NetWork.String()), metadata.SourceDetail(), metadata.RemoteAddress(), fmt.Sprintf("%s(%s)", rule.RuleType().String(), rule.Payload()), remoteConn.Chains().String())
		} else {
			log.Infoln("[%s] %s --> %s match %s using %s", strings.ToUpper(metadata.NetWork.String()), metadata.SourceDetail(), metadata.RemoteAddress(), rule.RuleType().String(), remoteConn.Chains().String())
		}
	case mode == Global:
		log.Infoln("[%s] %s --> %s using GLOBAL", strings.ToUpper(metadata.NetWork.String()), metadata.SourceDetail(), metadata.RemoteAddress())
	case mode == Direct:
		log.Infoln("[%s] %s --> %s using DIRECT", strings.ToUpper(metadata.NetWork.String()), metadata.SourceDetail(), metadata.RemoteAddress())
	default:
		log.Infoln("[%s] %s --> %s doesn't match any rule using %s", strings.ToUpper(metadata.NetWork.String()), metadata.SourceDetail(), metadata.RemoteAddress(), remoteConn.Chains().String())
	}
}

func match(
	metadata *C.Metadata,
	runtimeState *runtimeSnapshot,
	hostsResolved bool,
	processLookupEnabled bool,
) (C.Proxy, C.Rule, error) {
	var activeContext *matchContext
	var helper C.RuleMatchHelper
	var rematchChain []string
	for {
		var rematchProxy C.Proxy
		var rematchRule C.Rule
		rules, index := getRules(metadata, runtimeState)
		cursor := index.next(0, metadata)
	NextRule:
		for cursor < len(rules) {
			rule := rules[cursor]
			if index.usesHelperAt(cursor, rules) && activeContext == nil {
				activeContext = &matchContext{
					metadata:     metadata,
					runtimeState: runtimeState,
					resolved:     hostsResolved,
					attemptProcessLookup: processLookupEnabled &&
						metadata.Type != C.INNER,
				}
				helper = C.RuleMatchHelper{
					ResolveIP:     activeContext.resolveIP,
					FindProcess:   activeContext.findProcess,
					CheckPassRule: activeContext.checkPassRule,
				}
			}
			if matched, ada := rule.Match(metadata, helper); matched {
				adapter, ok := runtimeState.proxies[ada]
				if !ok {
					cursor = index.after(cursor, metadata)
					continue NextRule
				}

				// parse multi-layer nesting
				for adapter := adapter; adapter != nil; adapter = adapter.Unwrap(metadata, false) {
					if adapter.Type() == C.Pass {
						log.Debugln("%s match Pass rule", adapter.Name())
						cursor = index.after(cursor, metadata)
						continue NextRule
					}
					if adapter.Type() == C.Rematch {
						log.Debugln("%s match Rematch rule", adapter.Name())
						rematchProxy = adapter
						rematchRule = rule
						break NextRule
					}
				}

				if metadata.NetWork == C.UDP && !adapter.SupportUDP() {
					log.Debugln("%s UDP is not supported", adapter.Name())
					cursor = index.after(cursor, metadata)
					continue NextRule
				}

				return adapter, rule, nil
			}
			cursor = index.after(cursor, metadata)
		}
		if rematchProxy != nil {
			if slices.Contains(rematchChain, rematchProxy.Name()) {
				log.Warnln("[Rule] rematch cycle detected on %s", rematchProxy.Name())
				return rematchProxy, rematchRule, nil
			}
			rematchChain = append(rematchChain, rematchProxy.Name())
			conn, err := rematchProxy.DialContext(context.Background(), metadata) // not a real connection, just for metadata update
			if conn != nil {
				_ = conn.Close()
			}
			if err != nil {
				log.Warnln("[Rule] rematch proxy %s failed to update metadata: %s", rematchProxy.Name(), err)
				return rematchProxy, rematchRule, nil
			}
			log.Debugln("[Rule] rematch proxy %s update metadata to rematch-name=%q sub-rule=%q", rematchProxy.Name(), metadata.InName, metadata.SpecialRules)
			continue
		}
		return runtimeState.proxies["DIRECT"], nil, nil
	}
}

func getRules(
	metadata *C.Metadata,
	runtimeState *runtimeSnapshot,
) ([]C.Rule, *ruleIndex) {
	if sr, ok := runtimeState.subRules[metadata.SpecialRules]; ok {
		log.Debugln("[Rule] use %s rules", metadata.SpecialRules)
		return sr, runtimeState.subRuleIndexes[metadata.SpecialRules]
	} else {
		log.Debugln("[Rule] use default rules")
		return runtimeState.rules, runtimeState.ruleIndex
	}
}

func shouldStopRetry(err error) bool {
	if errors.Is(err, resolver.ErrIPNotFound) {
		return true
	}
	if errors.Is(err, resolver.ErrIPVersion) {
		return true
	}
	if errors.Is(err, resolver.ErrIPv6Disabled) {
		return true
	}
	if errors.Is(err, loopback.ErrReject) {
		return true
	}
	// A refused destination will not start listening inside the retry budget,
	// so waiting seconds for it stalls every request on that route.
	if dialer.IsConnectionRefused(err) {
		return true
	}
	return false
}

func retry[T any](ctx context.Context, ft func(context.Context) (T, error), fe func(err error)) (t T, err error) {
	var s *slowdown.SlowDown
	for i := 0; i < 10; i++ {
		t, err = ft(ctx)
		if err != nil {
			if fe != nil {
				fe(err)
			}
			if shouldStopRetry(err) {
				return
			}
			if s == nil {
				s = slowdown.New() // only a failing dial ever backs off
			}
			if s.Wait(ctx) == nil {
				continue
			} else {
				return
			}
		} else {
			break
		}
	}
	return
}
