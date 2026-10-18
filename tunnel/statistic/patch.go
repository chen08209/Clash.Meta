package statistic

type RequestNotify func(c Tracker)

var DefaultRequestNotify RequestNotify

// IsDirect reports whether the named outbound reaches the destination without
// a proxy. A direct proxy may carry any name, so the tunnel, which knows each
// proxy's type, replaces this name check.
var IsDirect = func(name string) bool { return name == "DIRECT" }

func (m *Manager) TotalTraffic(onlyProxy bool) (up, down int64) {
	for i := range m.stripes {
		s := &m.stripes[i]
		if onlyProxy {
			up += s.proxyUploadTotal.Load()
			down += s.proxyDownloadTotal.Load()
		} else {
			up += s.uploadTotal.Load()
			down += s.downloadTotal.Load()
		}
	}
	return
}

func (m *Manager) NowTraffic(onlyProxy bool) (up, down int64) {
	if onlyProxy {
		return m.proxyUploadBlip.Load(), m.proxyDownloadBlip.Load()
	}
	return m.uploadBlip.Load(), m.downloadBlip.Load()
}
