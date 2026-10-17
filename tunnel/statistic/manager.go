package statistic

import (
	"os"
	"time"

	"github.com/metacubex/mihomo/common/atomic"
	"github.com/metacubex/mihomo/common/xsync"
	"github.com/metacubex/mihomo/component/memory"

	"github.com/gofrs/uuid/v5"
)

var DefaultManager *Manager

func init() {
	DefaultManager = &Manager{
		stripes: newCounterStripes(),
		pid:     int32(os.Getpid()),
	}

	go DefaultManager.handle()
}

// counterStripe isolates one connection's traffic counters on their own cache
// lines: atomic.Int64 is 8-byte aligned on this target, so the padding, not the
// type, is what keeps neighbours apart. The 32-stripe array is a 4 KiB object
// the Go allocator aligns to a 4096-byte boundary.
type counterStripe struct {
	uploadTemp         atomic.Int64
	uploadTotal        atomic.Int64
	proxyUploadTemp    atomic.Int64
	proxyUploadTotal   atomic.Int64
	uploadPad          [4]int64
	downloadTemp       atomic.Int64
	downloadTotal      atomic.Int64
	proxyDownloadTemp  atomic.Int64
	proxyDownloadTotal atomic.Int64
	stripePad          [4]int64
}

const counterStripeCount = 32

func newCounterStripes() []counterStripe {
	return make([]counterStripe, counterStripeCount)
}

type Manager struct {
	connections       xsync.Map[uuid.UUID, Tracker]
	stripes           []counterStripe
	stripeCursor      atomic.Uint64
	uploadBlip        atomic.Int64
	downloadBlip      atomic.Int64
	proxyUploadBlip   atomic.Int64
	proxyDownloadBlip atomic.Int64
	pid               int32
	memory            uint64
}

func (m *Manager) Join(c Tracker) {
	if DefaultRequestNotify != nil {
		DefaultRequestNotify(c)
	}
	m.connections.Store(c.Info().UUID, c)
}

func (m *Manager) Leave(c Tracker) {
	m.connections.Delete(c.Info().UUID)
}

func (m *Manager) Get(id string) (c Tracker) {
	key, err := uuid.FromString(id)
	if err != nil {
		return nil
	}
	if value, ok := m.connections.Load(key); ok {
		c = value
	}
	return
}

func (m *Manager) Range(f func(c Tracker) bool) {
	m.connections.Range(func(key uuid.UUID, value Tracker) bool {
		return f(value)
	})
}

func (m *Manager) PushUploaded(direct bool, size int64) {
	m.sharedStripe().pushUploaded(size, direct)
}

func (m *Manager) bindStripe() *counterStripe {
	if len(m.stripes) == 0 {
		return nil
	}
	return &m.stripes[m.stripeCursor.Add(1)%uint64(counterStripeCount)]
}

func (m *Manager) sharedStripe() *counterStripe {
	if len(m.stripes) == 0 {
		return nil
	}
	return &m.stripes[0]
}

func (c *counterStripe) pushUploaded(size int64, direct bool) {
	if c == nil {
		return
	}
	if !direct {
		c.proxyUploadTemp.Add(size)
		c.proxyUploadTotal.Add(size)
	}
	c.uploadTemp.Add(size)
	c.uploadTotal.Add(size)
}

func (c *counterStripe) pushDownloaded(size int64, direct bool) {
	if c == nil {
		return
	}
	if !direct {
		c.proxyDownloadTemp.Add(size)
		c.proxyDownloadTotal.Add(size)
	}
	c.downloadTemp.Add(size)
	c.downloadTotal.Add(size)
}

func (m *Manager) totalUpload() (total int64) {
	for i := range m.stripes {
		total += m.stripes[i].uploadTotal.Load()
	}
	return total
}

func (m *Manager) totalDownload() (total int64) {
	for i := range m.stripes {
		total += m.stripes[i].downloadTotal.Load()
	}
	return total
}

func (m *Manager) totalProxyUpload() (total int64) {
	for i := range m.stripes {
		total += m.stripes[i].proxyUploadTotal.Load()
	}
	return total
}

func (m *Manager) totalProxyDownload() (total int64) {
	for i := range m.stripes {
		total += m.stripes[i].proxyDownloadTotal.Load()
	}
	return total
}

func (m *Manager) swapUploadTemp() (sum int64) {
	for i := range m.stripes {
		sum += m.stripes[i].uploadTemp.Swap(0)
	}
	return sum
}

func (m *Manager) swapDownloadTemp() (sum int64) {
	for i := range m.stripes {
		sum += m.stripes[i].downloadTemp.Swap(0)
	}
	return sum
}

func (m *Manager) swapProxyUploadTemp() (sum int64) {
	for i := range m.stripes {
		sum += m.stripes[i].proxyUploadTemp.Swap(0)
	}
	return sum
}

func (m *Manager) swapProxyDownloadTemp() (sum int64) {
	for i := range m.stripes {
		sum += m.stripes[i].proxyDownloadTemp.Swap(0)
	}
	return sum
}

func (m *Manager) PushDownloaded(direct bool, size int64) {
	m.sharedStripe().pushDownloaded(size, direct)
}

func (m *Manager) Now() (up int64, down int64) {
	return m.uploadBlip.Load(), m.downloadBlip.Load()
}

func (m *Manager) Total() (up, down int64) {
	return m.totalUpload(), m.totalDownload()
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
	return &Snapshot{
		UploadTotal:   m.totalUpload(),
		DownloadTotal: m.totalDownload(),
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
		stripe := &m.stripes[i]
		stripe.uploadTemp.Store(0)
		stripe.uploadTotal.Store(0)
		stripe.proxyUploadTemp.Store(0)
		stripe.proxyUploadTotal.Store(0)
		stripe.downloadTemp.Store(0)
		stripe.downloadTotal.Store(0)
		stripe.proxyDownloadTemp.Store(0)
		stripe.proxyDownloadTotal.Store(0)
	}

	m.uploadBlip.Store(0)
	m.downloadBlip.Store(0)
	m.proxyUploadBlip.Store(0)
	m.proxyDownloadBlip.Store(0)
}

func (m *Manager) handle() {
	ticker := time.NewTicker(time.Second)

	for range ticker.C {
		m.uploadBlip.Store(m.swapUploadTemp())
		m.downloadBlip.Store(m.swapDownloadTemp())
		m.proxyUploadBlip.Store(m.swapProxyUploadTemp())
		m.proxyDownloadBlip.Store(m.swapProxyDownloadTemp())
	}
}

type Snapshot struct {
	DownloadTotal int64          `json:"downloadTotal"`
	UploadTotal   int64          `json:"uploadTotal"`
	Connections   []*TrackerInfo `json:"connections"`
	Memory        uint64         `json:"memory"`
}
