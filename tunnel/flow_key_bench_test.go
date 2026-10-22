package tunnel

import (
	"hash/maphash"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func TestPacketFlowKeyDoesNotAllocate(t *testing.T) {
	adapter := C.NewPacketAdapter(
		fixedLocalAddrPacket{
			localAddr: &net.UDPAddr{
				IP:   net.IPv4(192, 168, 1, 2),
				Port: 54321,
			},
		},
		&C.Metadata{},
	)

	allocations := testing.AllocsPerRun(100, func() {
		key, valid := packetFlowKey(adapter)
		require.True(t, valid)
		require.True(t, key.IsValid())
	})
	require.EqualValues(t, 0, allocations)
}

func TestPacketSenderDropsWhenQueueIsFull(t *testing.T) {
	sender := newPacketSender()
	defer sender.Close()
	for i := 0; i < senderCapacity; i++ {
		sender.Send(queuedPacket{})
	}

	var dropped atomic.Int32
	overflow := blockingDropPacket{drops: &dropped}
	sender.Send(overflow)

	require.EqualValues(t, 1, dropped.Load())
}

func TestPacketSenderDropsAfterClose(t *testing.T) {
	sender := newPacketSender()
	sender.Close()

	var dropped atomic.Int32
	sender.Send(blockingDropPacket{drops: &dropped})

	require.EqualValues(t, 1, dropped.Load())
}

func TestPacketSenderCloseDrainsConcurrentSends(t *testing.T) {
	for iteration := 0; iteration < 100; iteration++ {
		sender := newPacketSender().(*packetSender)
		packet := &queuedPacket{}
		start := make(chan struct{})

		var waiting sync.WaitGroup
		waiting.Add(1)
		go func() {
			defer waiting.Done()
			<-start
			for index := 0; index < senderCapacity; index++ {
				sender.Send(packet)
			}
		}()

		close(start)
		sender.Close()
		waiting.Wait()
		require.Zero(t, len(sender.ch))
	}
}

func BenchmarkPacketFlowKey(b *testing.B) {
	adapter := C.NewPacketAdapter(
		fixedLocalAddrPacket{
			localAddr: &net.UDPAddr{
				IP:   net.IPv4(192, 168, 1, 2),
				Port: 54321,
			},
		},
		&C.Metadata{},
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, valid := packetFlowKey(adapter); !valid {
			b.Fatal("flow key was invalid")
		}
	}
}

func BenchmarkUDPQueueHash(b *testing.B) {
	key := netip.MustParseAddrPort("192.168.1.2:54321")
	seed := maphash.MakeSeed()

	b.Run("maphash", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = maphash.Comparable(seed, key)
		}
	})

	b.Run("addrport-fnv", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = utils.MapHashAddrPort(key)
		}
	})
}

func BenchmarkPacketSenderSend(b *testing.B) {
	packet := &queuedPacket{}

	b.Run("ready", func(b *testing.B) {
		sender := newPacketSender().(*packetSender)
		b.Cleanup(sender.Close)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sender.Send(packet)
			<-sender.ch
		}
	})

	b.Run("full", func(b *testing.B) {
		sender := newPacketSender().(*packetSender)
		b.Cleanup(sender.Close)
		for i := 0; i < senderCapacity; i++ {
			sender.Send(packet)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sender.Send(packet)
		}
	})

	b.Run("closed", func(b *testing.B) {
		sender := newPacketSender().(*packetSender)
		sender.Close()

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sender.Send(packet)
		}
	})
}

type blockingDropPacket struct {
	C.PacketAdapter
	drops *atomic.Int32
}

func (packet blockingDropPacket) Drop() {
	packet.drops.Add(1)
}

type queuedPacket struct {
	C.PacketAdapter
}

func (queuedPacket) Drop() {}
