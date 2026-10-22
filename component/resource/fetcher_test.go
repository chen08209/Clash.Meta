package resource

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/metacubex/mihomo/common/utils"
	P "github.com/metacubex/mihomo/constant/provider"
)

// churningVehicle hands back a fresh hash on every read, so the fetcher always
// takes loadBuf's write path and stores the hash the api reads back.
type churningVehicle struct {
	calls atomic.Uint64
}

func (v *churningVehicle) Type() P.VehicleType { return P.Compatible }
func (v *churningVehicle) Path() string        { return "" }
func (v *churningVehicle) Url() string         { return "" }
func (v *churningVehicle) Proxy() string       { return "" }
func (v *churningVehicle) Write([]byte) error  { return nil }

func (v *churningVehicle) Read(context.Context, utils.HashType) ([]byte, utils.HashType, error) {
	buf := []byte(strconv.FormatUint(v.calls.Add(1), 10))
	return buf, utils.MakeHash(buf), nil
}

// An update stores the hash and the timestamp while the api asks for the
// timestamp and another update reads the hash back; every side has to hold the
// state lock or the race detector reports the pair.
func TestFetcherStateStaysRaceFreeWhileUpdating(t *testing.T) {
	fetcher := NewFetcher(
		"race",
		0,
		&churningVehicle{},
		nil,
		func(buf []byte) (int, error) { return len(buf), nil },
		nil,
	)

	var wait sync.WaitGroup
	for _, run := range []func(){
		func() {
			for i := 0; i < 200; i++ {
				_, _, _ = fetcher.Update()
			}
		},
		func() {
			for i := 0; i < 200; i++ {
				_, _, _ = fetcher.SideUpdate([]byte("side"))
			}
		},
		func() {
			for i := 0; i < 200; i++ {
				_ = fetcher.UpdatedAt()
			}
		},
	} {
		wait.Add(1)
		go func(run func()) {
			defer wait.Done()
			run()
		}(run)
	}
	wait.Wait()
}
