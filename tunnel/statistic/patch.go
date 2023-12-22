package statistic

type RequestNotify func(c Tracker)

var DefaultRequestNotify RequestNotify

func (m *Manager) Total() (up, down int64) {
	return m.uploadTotal.Load(), m.downloadTotal.Load()
}
