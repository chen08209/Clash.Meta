package once

import (
	"sync"
	"sync/atomic"
)

// Once runs a function at most one time and reports whether it has run.
// Reset returns it to the un-run state, which sync.Once cannot express.
type Once struct {
	done atomic.Uint32
	m    sync.Mutex
}

func (o *Once) Do(f func()) {
	if o.done.Load() == 1 {
		return
	}
	o.m.Lock()
	defer o.m.Unlock()
	if o.done.Load() == 0 {
		// A panicking f still counts as run, matching sync.Once.
		defer o.done.Store(1)
		f()
	}
}

func (o *Once) Done() bool {
	return o.done.Load() == 1
}

func (o *Once) Reset() {
	o.m.Lock()
	defer o.m.Unlock()
	o.done.Store(0)
}
