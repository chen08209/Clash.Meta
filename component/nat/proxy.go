package nat

import (
	"net"
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

func (w *writeBackProxy) UpdateWriteBack(wb C.WriteBack) {
	w.wb.Store(wb)
}

func NewWriteBackProxy(wb C.WriteBack) C.WriteBackProxy {
	w := &writeBackProxy{}
	w.UpdateWriteBack(wb)
	return w
}
