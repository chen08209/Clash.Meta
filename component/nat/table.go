package nat

import (
	"net"
	"net/netip"
	"sync"

	"github.com/metacubex/mihomo/common/xsync"
	C "github.com/metacubex/mihomo/constant"
)

type Table struct {
	mapping xsync.Map[netip.AddrPort, *entry]
}

type entry struct {
	PacketSender    C.PacketSender
	LocalUDPConnMap xsync.Map[netip.AddrPort, *net.UDPConn]
	LocalLockMap    xsync.Map[netip.AddrPort, *sync.Cond]
}

func (t *Table) GetOrCreate(key netip.AddrPort, maker func() C.PacketSender) (C.PacketSender, bool) {
	item, loaded := t.mapping.LoadOrStoreFn(key, func() *entry {
		return &entry{
			PacketSender: maker(),
		}
	})
	return item.PacketSender, loaded
}

func (t *Table) Delete(key netip.AddrPort) {
	t.mapping.Delete(key)
}

func (t *Table) GetForLocalConn(lAddr, rAddr netip.AddrPort) *net.UDPConn {
	entry, exist := t.getEntry(lAddr)
	if !exist {
		return nil
	}
	item, exist := entry.LocalUDPConnMap.Load(rAddr)
	if !exist {
		return nil
	}
	return item
}

func (t *Table) AddForLocalConn(lAddr, rAddr netip.AddrPort, conn *net.UDPConn) bool {
	entry, exist := t.getEntry(lAddr)
	if !exist {
		return false
	}
	entry.LocalUDPConnMap.Store(rAddr, conn)
	return true
}

func (t *Table) RangeForLocalConn(lAddr netip.AddrPort, f func(key netip.AddrPort, value *net.UDPConn) bool) {
	entry, exist := t.getEntry(lAddr)
	if !exist {
		return
	}
	entry.LocalUDPConnMap.Range(f)
}

func (t *Table) GetOrCreateLockForLocalConn(lAddr, key netip.AddrPort) (*sync.Cond, bool) {
	entry, loaded := t.getEntry(lAddr)
	if !loaded {
		return nil, false
	}
	item, loaded := entry.LocalLockMap.LoadOrStoreFn(key, makeLock)
	return item, loaded
}

func (t *Table) DeleteForLocalConn(lAddr, key netip.AddrPort) {
	entry, loaded := t.getEntry(lAddr)
	if !loaded {
		return
	}
	entry.LocalUDPConnMap.Delete(key)
}

func (t *Table) DeleteLockForLocalConn(lAddr, key netip.AddrPort) {
	entry, loaded := t.getEntry(lAddr)
	if !loaded {
		return
	}
	entry.LocalLockMap.Delete(key)
}

func (t *Table) getEntry(key netip.AddrPort) (*entry, bool) {
	return t.mapping.Load(key)
}

func makeLock() *sync.Cond {
	return sync.NewCond(&sync.Mutex{})
}

// New return *Cache
func New() *Table {
	return &Table{}
}
