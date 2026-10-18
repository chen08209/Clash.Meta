package statistic

import (
	"os"
	"time"

	"github.com/metacubex/mihomo/common/atomic"
	"github.com/metacubex/mihomo/common/xsync"
	"github.com/metacubex/mihomo/component/memory"
)

var DefaultManager *Manager

func init() {
	DefaultManager = &Manager{
		pid: int32(os.Getpid()),
	}

	go DefaultManager.handle()
}

// managerStripeCount is the number of counter shards. Every relay picks one at
// creation, so concurrent connections add on distinct cache lines instead of
// all hammering one shared line per counter.
const managerStripeCount = 16

// managerStripe holds one shard of the traffic counters. The padding keeps
// neighbouring shards off each other's cache lines.
type managerStripe struct {
	uploadTemp         atomic.Int64
	downloadTemp       atomic.Int64
	uploadTotal        atomic.Int64
	downloadTotal      atomic.Int64
	proxyUploadTemp    atomic.Int64
	proxyDownloadTemp  atomic.Int64
	proxyUploadTotal   atomic.Int64
	proxyDownloadTotal atomic.Int64
	_                  [64]byte
}

type Manager struct {
	connections        xsync.Map[string, Tracker]
	stripes            [managerStripeCount]managerStripe
	uploadBlip         atomic.Int64
	downloadBlip       atomic.Int64
	proxyUploadBlip    atomic.Int64
	proxyDownloadBlip  atomic.Int64
	nextStripe         atomic.Int64
	pid                int32
	memory             uint64
}

func (m *Manager) Join(c Tracker) {
	if DefaultRequestNotify != nil {
		DefaultRequestNotify(c)
	}
	m.connections.Store(c.ID(), c)
}

func (m *Manager) Leave(c Tracker) {
	m.connections.Delete(c.ID())
}

func (m *Manager) Get(id string) (c Tracker) {
	if value, ok := m.connections.Load(id); ok {
		c = value
	}
	return
}

func (m *Manager) Range(f func(c Tracker) bool) {
	m.connections.Range(func(key string, value Tracker) bool {
		return f(value)
	})
}

func (m *Manager) pickStripe() int {
	return int(m.nextStripe.Add(1)-1) % managerStripeCount
}

func (m *Manager) PushUploaded(stripe int, direct bool, size int64) {
	s := &m.stripes[stripe]
	if !direct {
		s.proxyUploadTemp.Add(size)
		s.proxyUploadTotal.Add(size)
	}
	s.uploadTemp.Add(size)
	s.uploadTotal.Add(size)
}

func (m *Manager) PushDownloaded(stripe int, direct bool, size int64) {
	s := &m.stripes[stripe]
	if !direct {
		s.proxyDownloadTemp.Add(size)
		s.proxyDownloadTotal.Add(size)
	}
	s.downloadTemp.Add(size)
	s.downloadTotal.Add(size)
}

func (m *Manager) Now() (up int64, down int64) {
	return m.uploadBlip.Load(), m.downloadBlip.Load()
}

func (m *Manager) Total() (up, down int64) {
	for i := range m.stripes {
		up += m.stripes[i].uploadTotal.Load()
		down += m.stripes[i].downloadTotal.Load()
	}
	return
}

func (m *Manager) Memory() uint64 {
	m.updateMemory()
	return m.memory
}

func (m *Manager) Snapshot() *Snapshot {
	var connections []*TrackerInfo
	m.Range(func(c Tracker) bool {
		connections = append(connections, c.Info())
		return true
	})
	upTotal, downTotal := m.Total()
	return &Snapshot{
		UploadTotal:   upTotal,
		DownloadTotal: downTotal,
		Connections:   connections,
		Memory:        m.memory,
	}
}

func (m *Manager) updateMemory() {
	stat, err := memory.GetMemoryInfo(m.pid)
	if err != nil {
		return
	}
	m.memory = stat.RSS
}

func (m *Manager) ResetStatistic() {
	for i := range m.stripes {
		s := &m.stripes[i]
		s.uploadTemp.Store(0)
		s.downloadTemp.Store(0)
		s.uploadTotal.Store(0)
		s.downloadTotal.Store(0)
		s.proxyUploadTemp.Store(0)
		s.proxyDownloadTemp.Store(0)
		s.proxyUploadTotal.Store(0)
		s.proxyDownloadTotal.Store(0)
	}
	m.uploadBlip.Store(0)
	m.downloadBlip.Store(0)
	m.proxyUploadBlip.Store(0)
	m.proxyDownloadBlip.Store(0)
}

func (m *Manager) handle() {
	ticker := time.NewTicker(time.Second)

	for range ticker.C {
		m.rotateBlips()
	}
}

func (m *Manager) rotateBlips() {
	var up, down, proxyUp, proxyDown int64
	for i := range m.stripes {
		s := &m.stripes[i]
		up += s.uploadTemp.Swap(0)
		down += s.downloadTemp.Swap(0)
		proxyUp += s.proxyUploadTemp.Swap(0)
		proxyDown += s.proxyDownloadTemp.Swap(0)
	}
	m.uploadBlip.Store(up)
	m.downloadBlip.Store(down)
	m.proxyUploadBlip.Store(proxyUp)
	m.proxyDownloadBlip.Store(proxyDown)
}

type Snapshot struct {
	DownloadTotal int64          `json:"downloadTotal"`
	UploadTotal   int64          `json:"uploadTotal"`
	Connections   []*TrackerInfo `json:"connections"`
	Memory        uint64         `json:"memory"`
}
