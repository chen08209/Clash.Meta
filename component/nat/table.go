package nat

import (
	"net"
	"net/netip"
	"sync"

	"github.com/metacubex/mihomo/common/xsync"
	C "github.com/metacubex/mihomo/constant"
)

type Table struct {
	mapping    xsync.Map[netip.AddrPort, *flowEntry]
	localConns xsync.Map[netip.AddrPort, *localConnState]
}

type flowEntry struct {
	PacketSender C.PacketSender
}

type localConnState struct {
	udpConns xsync.Map[string, *net.UDPConn]
	locks    xsync.Map[string, *sync.Cond]
}

func (t *Table) GetOrCreateForFlow(
	key netip.AddrPort,
	maker func() C.PacketSender,
) (C.PacketSender, bool) {
	item, loaded := t.mapping.LoadOrStoreFn(key, func() *flowEntry {
		return &flowEntry{
			PacketSender: maker(),
		}
	})
	return item.PacketSender, loaded
}

func (t *Table) LoadForFlow(key netip.AddrPort) (C.PacketSender, bool) {
	item, exist := t.mapping.Load(key)
	if !exist {
		return nil, false
	}
	return item.PacketSender, true
}

func (t *Table) GetOrCreate(key string, maker func() C.PacketSender) (C.PacketSender, bool) {
	flowKey, ok := parseFlowKey(key)
	if !ok {
		return nil, false
	}
	return t.GetOrCreateForFlow(flowKey, maker)
}

func (t *Table) DeleteForFlow(key netip.AddrPort) {
	t.mapping.Delete(key)
	t.localConns.Delete(key)
}

func (t *Table) Delete(key string) {
	if flowKey, ok := parseFlowKey(key); ok {
		t.DeleteForFlow(flowKey)
	}
}

func (t *Table) GetForLocalConn(lAddr, rAddr string) *net.UDPConn {
	flowKey, ok := parseFlowKey(lAddr)
	if !ok {
		return nil
	}
	state, exist := t.getLocalConnStateForFlow(flowKey)
	if !exist {
		return nil
	}
	item, exist := state.udpConns.Load(rAddr)
	if !exist {
		return nil
	}
	return item
}

func (t *Table) AddForLocalConn(lAddr, rAddr string, conn *net.UDPConn) bool {
	flowKey, ok := parseFlowKey(lAddr)
	if !ok || !t.hasFlow(flowKey) {
		return false
	}
	state, _ := t.localConns.LoadOrStoreFn(flowKey, newLocalConnState)
	state.udpConns.Store(rAddr, conn)
	return true
}

func (t *Table) RangeForLocalConn(lAddr string, f func(key string, value *net.UDPConn) bool) {
	flowKey, ok := parseFlowKey(lAddr)
	if !ok {
		return
	}
	state, exist := t.getLocalConnStateForFlow(flowKey)
	if !exist {
		return
	}
	state.udpConns.Range(f)
}

func (t *Table) RangeForLocalConnForFlow(
	key netip.AddrPort,
	f func(key string, value *net.UDPConn) bool,
) {
	state, exist := t.getLocalConnStateForFlow(key)
	if !exist {
		return
	}
	state.udpConns.Range(f)
}

func (t *Table) GetOrCreateLockForLocalConn(lAddr, key string) (*sync.Cond, bool) {
	flowKey, ok := parseFlowKey(lAddr)
	if !ok || !t.hasFlow(flowKey) {
		return nil, false
	}
	state, _ := t.localConns.LoadOrStoreFn(flowKey, newLocalConnState)
	item, loaded := state.locks.LoadOrStoreFn(key, makeLock)
	return item, loaded
}

func (t *Table) DeleteForLocalConn(lAddr, key string) {
	flowKey, ok := parseFlowKey(lAddr)
	if !ok {
		return
	}
	state, loaded := t.getLocalConnStateForFlow(flowKey)
	if !loaded {
		return
	}
	state.udpConns.Delete(key)
}

func (t *Table) DeleteLockForLocalConn(lAddr, key string) {
	flowKey, ok := parseFlowKey(lAddr)
	if !ok {
		return
	}
	state, loaded := t.getLocalConnStateForFlow(flowKey)
	if !loaded {
		return
	}
	state.locks.Delete(key)
}

func (t *Table) hasFlow(key netip.AddrPort) bool {
	_, exist := t.mapping.Load(key)
	return exist
}

func (t *Table) getLocalConnStateForFlow(key netip.AddrPort) (*localConnState, bool) {
	if !t.hasFlow(key) {
		return nil, false
	}
	return t.localConns.Load(key)
}

func newLocalConnState() *localConnState {
	return &localConnState{}
}

func parseFlowKey(key string) (netip.AddrPort, bool) {
	flowKey, err := netip.ParseAddrPort(key)
	if err != nil || !flowKey.IsValid() {
		return netip.AddrPort{}, false
	}
	return flowKey, true
}

func makeLock() *sync.Cond {
	return sync.NewCond(&sync.Mutex{})
}

// New return *Cache
func New() *Table {
	return &Table{}
}
