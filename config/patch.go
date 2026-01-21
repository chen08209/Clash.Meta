package config

import "sync"

var (
	proxyNameList   []string
	proxyNameListMu sync.RWMutex
)

func GetProxyNameList() []string {
	proxyNameListMu.RLock()
	defer proxyNameListMu.RUnlock()
	return proxyNameList
}

func SetProxyNameList(list []string) {
	proxyNameListMu.Lock()
	defer proxyNameListMu.Unlock()
	proxyNameList = list
}
