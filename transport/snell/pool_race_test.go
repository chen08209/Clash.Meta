package snell

import (
	"context"
	"errors"
	"testing"

	"github.com/metacubex/mihomo/transport/shadowsocks/shadowaead"
)

// The relay writes data frames while the session teardown half-closes the same
// stream; the two writers must not race on the cipher state.
func TestPoolConnWriteAndCloseWriteDoNotRace(t *testing.T) {
	rawConn := &recordingConn{
		readData: []byte{CommandTunnel},
		readErr:  shadowaead.ErrZeroChunk,
	}
	pooledConn := &Snell{Conn: rawConn}
	pool := NewPool(func(context.Context) (*Snell, error) {
		return nil, errors.New("factory should not be called")
	})
	conn := &PoolConn{Snell: pooledConn, pool: pool}
	conn.MarkReusable()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			if _, err := conn.Write([]byte{1}); err != nil {
				return
			}
		}
	}()
	if err := conn.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	<-done

	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}
