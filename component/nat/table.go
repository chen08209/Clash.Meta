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
	udpConns xsync.Map[netip.AddrPort, *net.UDPConn]
	locks    xsync.Map[netip.AddrPort, *sync.Cond]
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

func (t *Table) GetForLocalConn(lAddr, rAddr netip.AddrPort) *net.UDPConn {
	state, exist := t.getLocalConnStateForFlow(lAddr)
	if !exist {
		return nil
	}
	item, exist := state.udpConns.Load(rAddr)
	if !exist {
		return nil
	}
	return item
}

func (t *Table) AddForLocalConn(lAddr, rAddr netip.AddrPort, conn *net.UDPConn) bool {
	if !t.hasFlow(lAddr) {
		return false
	}
	state, _ := t.localConns.LoadOrStoreFn(lAddr, newLocalConnState)
	state.udpConns.Store(rAddr, conn)
	return true
}

func (t *Table) RangeForLocalConnForFlow(
	key netip.AddrPort,
	f func(key netip.AddrPort, value *net.UDPConn) bool,
) {
	state, exist := t.getLocalConnStateForFlow(key)
	if !exist {
		return
	}
	state.udpConns.Range(f)
}

func (t *Table) GetOrCreateLockForLocalConn(lAddr, rAddr netip.AddrPort) (*sync.Cond, bool) {
	if !t.hasFlow(lAddr) {
		return nil, false
	}
	state, _ := t.localConns.LoadOrStoreFn(lAddr, newLocalConnState)
	item, loaded := state.locks.LoadOrStoreFn(rAddr, makeLock)
	return item, loaded
}

func (t *Table) DeleteForLocalConn(lAddr, rAddr netip.AddrPort) {
	state, loaded := t.getLocalConnStateForFlow(lAddr)
	if !loaded {
		return
	}
	state.udpConns.Delete(rAddr)
}

func (t *Table) DeleteLockForLocalConn(lAddr, rAddr netip.AddrPort) {
	state, loaded := t.getLocalConnStateForFlow(lAddr)
	if !loaded {
		return
	}
	state.locks.Delete(rAddr)
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
