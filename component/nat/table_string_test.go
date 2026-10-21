package nat

import (
	"net"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTableStringKeyedFlowLifecycle(t *testing.T) {
	table := New()
	key := "192.168.1.2:54321"
	flowKey := netip.MustParseAddrPort(key)

	sender := &noopPacketSender{}
	created, loaded := table.GetOrCreate(key, func() C.PacketSender {
		return sender
	})
	assert.Same(t, sender, created)
	assert.False(t, loaded)

	// the same string key resolves to the same flow and keeps the sender
	made := 0
	again, loaded := table.GetOrCreate(key, func() C.PacketSender {
		made++
		return &noopPacketSender{}
	})
	assert.Same(t, sender, again)
	assert.True(t, loaded)
	assert.Zero(t, made)

	// the typed load sees it too, and a miss is reported
	loadedSender, exist := table.LoadForFlow(flowKey)
	require.True(t, exist)
	assert.Same(t, sender, loadedSender)
	_, exist = table.LoadForFlow(netip.MustParseAddrPort("192.168.1.9:9999"))
	assert.False(t, exist)

	// an unparsable key creates nothing and never calls the maker
	badSender, badLoaded := table.GetOrCreate("not-an-addrport", func() C.PacketSender {
		t.Fatal("maker called for an invalid flow key")
		return nil
	})
	assert.Nil(t, badSender)
	assert.False(t, badLoaded)

	// the string delete removes the flow, and an invalid key is a no-op
	table.Delete(key)
	_, exist = table.LoadForFlow(flowKey)
	assert.False(t, exist)
	table.Delete("not-an-addrport")
}

func TestTableLocalConnLockLifecycle(t *testing.T) {
	table := New()
	lAddr := netip.MustParseAddrPort("192.168.1.2:54321")
	rAddr := netip.MustParseAddrPort("93.184.216.34:443")

	// without a flow there is nothing to lock
	cond, loaded := table.GetOrCreateLockForLocalConn(lAddr, rAddr)
	assert.Nil(t, cond)
	assert.False(t, loaded)

	table.GetOrCreateForFlow(lAddr, func() C.PacketSender { return nil })

	cond, loaded = table.GetOrCreateLockForLocalConn(lAddr, rAddr)
	require.NotNil(t, cond)
	assert.False(t, loaded, "the first lock is created")
	cond.L.Lock()
	cond.L.Unlock()

	// the second request hands back the same lock
	same, loaded := table.GetOrCreateLockForLocalConn(lAddr, rAddr)
	assert.Same(t, cond, same)
	assert.True(t, loaded)

	// dropping the lock lets the next request create a fresh one
	table.DeleteLockForLocalConn(lAddr, rAddr)
	fresh, loaded := table.GetOrCreateLockForLocalConn(lAddr, rAddr)
	require.NotNil(t, fresh)
	assert.NotSame(t, cond, fresh)
	assert.False(t, loaded)

	// dropping the local conn and the lock on a flow without state is a no-op
	other := netip.MustParseAddrPort("192.168.1.7:1111")
	table.DeleteForLocalConn(other, rAddr)
	table.DeleteLockForLocalConn(other, rAddr)

	localConn := &net.UDPConn{}
	require.True(t, table.AddForLocalConn(lAddr, rAddr, localConn))
	assert.Same(t, localConn, table.GetForLocalConn(lAddr, rAddr))

	table.DeleteForLocalConn(lAddr, rAddr)
	assert.Nil(t, table.GetForLocalConn(lAddr, rAddr))

	table.DeleteLockForLocalConn(lAddr, rAddr)
	_, loaded = table.GetOrCreateLockForLocalConn(lAddr, rAddr)
	assert.False(t, loaded)
}
