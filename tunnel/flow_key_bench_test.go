package tunnel

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"

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
