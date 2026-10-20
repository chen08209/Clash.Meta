package nat

import (
	"net"
	"net/netip"
	"sync/atomic"

	C "github.com/metacubex/mihomo/constant"
)

type writeBackProxy struct {
	// Every packet of a flow carries a *constant.PacketAdapter, so the concrete
	// type stored here never changes and the per-packet update writes the
	// interface directly instead of boxing a fresh pointer.
	wb atomic.Value
}

func (w *writeBackProxy) WriteBack(b []byte, addr net.Addr) (n int, err error) {
	return w.wb.Load().(C.WriteBack).WriteBack(b, addr)
}

func (w *writeBackProxy) WriteBackAddrPort(b []byte, addr netip.AddrPort) (n int, err error) {
	wb := w.wb.Load().(C.WriteBack)
	if fast, isFast := wb.(C.AddrPortWriteBack); isFast {
		return fast.WriteBackAddrPort(b, addr)
	}
	return wb.WriteBack(b, net.UDPAddrFromAddrPort(addr))
}

func (w *writeBackProxy) UpdateWriteBack(wb C.WriteBack) {
	w.wb.Store(wb)
}

func NewWriteBackProxy(wb C.WriteBack) C.WriteBackProxy {
	w := &writeBackProxy{}
	w.UpdateWriteBack(wb)
	return w
}
