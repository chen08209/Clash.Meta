package tunnel

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/component/resolver"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

type missingRecordPacket struct {
	data    []byte
	laddr   net.Addr
	dropped int
}

func (p *missingRecordPacket) Data() []byte { return p.data }

func (p *missingRecordPacket) WriteBack(b []byte, addr net.Addr) (int, error) {
	return len(b), nil
}

func (p *missingRecordPacket) Drop() { p.dropped++ }

func (p *missingRecordPacket) LocalAddr() net.Addr { return p.laddr }

// every destination is a fake IP whose reverse record is gone
type missingRecordMapper struct{}

func (missingRecordMapper) FakeIPEnabled() bool               { return true }
func (missingRecordMapper) MappingEnabled() bool              { return true }
func (missingRecordMapper) IsFakeIP(netip.Addr) bool          { return true }
func (missingRecordMapper) IsFakeBroadcastIP(netip.Addr) bool { return false }
func (missingRecordMapper) IsExistFakeIP(netip.Addr) bool     { return false }
func (missingRecordMapper) FindHostByIP(netip.Addr) (string, bool) {
	return "", false
}

func (missingRecordMapper) FlushFakeIP() error             { return nil }
func (missingRecordMapper) InsertHostByIP(netip.Addr, string) {}
func (missingRecordMapper) StoreFakePoolState()            {}

func TestHandleUDPConnMissingFakeRecord(t *testing.T) {
	restoreStatus := status.Load()
	status.Store(Running)
	defer status.Store(restoreStatus)

	restoreMapper := resolver.DefaultHostMapper
	resolver.DefaultHostMapper = missingRecordMapper{}
	defer func() { resolver.DefaultHostMapper = restoreMapper }()

	laddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 53531}
	packet := &missingRecordPacket{data: make([]byte, 64), laddr: laddr}
	metadata := &C.Metadata{
		NetWork: C.UDP,
		Type:    C.REDIR,
		SrcIP:   netip.AddrFrom4([4]byte{192, 168, 1, 100}),
		SrcPort: 53531,
		DstIP:   netip.AddrFrom4([4]byte{198, 18, 0, 1}),
		DstPort: 443,
	}
	adapter := C.NewPacketAdapter(packet, metadata)
	key := adapter.Key()
	natTable.Delete(key) // clear any entry a previous run left behind

	handleUDPConn(adapter)

	require.Eventually(t, func() bool {
		// the failed session must not leave an entry behind
		_, loaded := natTable.GetOrCreate(key, func() C.PacketSender { return newPacketSender() })
		return !loaded
	}, 2*time.Second, 5*time.Millisecond)

	require.Eventually(t, func() bool { return packet.dropped > 0 }, 2*time.Second, 5*time.Millisecond)
}
