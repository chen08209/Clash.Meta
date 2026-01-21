package updater

import (
	"context"
	"sync"

	"github.com/metacubex/mihomo/log"
)

var (
	GeoUpdateHook func(geoType string, updating bool, skipped bool, updateErr error)
)

func sendGeoUpdateStatus(geoType string, updating bool, skipped bool, updateErr error) {
	if GeoUpdateHook != nil {
		GeoUpdateHook(geoType, updating, skipped, updateErr)
	}
}

var (
	geoUpdateMutex  sync.Mutex
	geoUpdateCancel context.CancelFunc
)

func StopGeoUpdater() {
	geoUpdateMutex.Lock()
	defer geoUpdateMutex.Unlock()
	stopGeoUpdater()
}

func stopGeoUpdater() {
	if geoUpdateCancel == nil {
		return
	}
	geoUpdateCancel()
	geoUpdateCancel = nil
}

func RegisterGeoUpdaterWithCancel() {
	geoUpdateMutex.Lock()
	defer geoUpdateMutex.Unlock()

	stopGeoUpdater()

	if updateInterval <= 0 {
		log.Infoln("[GEO] Invalid update interval: %d", updateInterval)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	geoUpdateCancel = cancel
	registerGeoUpdater(ctx)
}
