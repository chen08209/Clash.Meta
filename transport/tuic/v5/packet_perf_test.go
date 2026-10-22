//go:build !race

package v5

import (
	"bufio"
	"bytes"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func benchPacket() Packet {
	return NewPacket(1, 2, 1, 0, 1350,
		NewAddressAddrPort(netip.MustParseAddrPort("203.0.113.7:443")),
		make([]byte, 1350))
}

// 0.5 leaves room for the one buffer a sync.Pool refills after a GC while still
// failing the per-call allocation binary.Write made.
func TestPacketWriteToAllocatesNothing(t *testing.T) {
	packet := benchPacket()
	buf := &bytes.Buffer{}

	allocations := testing.AllocsPerRun(1000, func() {
		buf.Reset()
		if err := packet.WriteTo(buf); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 0.5)
}

// The payload and the address still have to be copied out of the stream, so the
// floor is two allocations; the length fields must not add any.
func TestReadPacketAllocatesPayloadAndAddressOnly(t *testing.T) {
	packet := benchPacket()
	var buf bytes.Buffer
	require.NoError(t, packet.WriteTo(&buf))
	raw := buf.Bytes()

	rawReader := bytes.NewReader(raw)
	reader := bufio.NewReaderSize(rawReader, 4096)
	allocations := testing.AllocsPerRun(1000, func() {
		rawReader.Reset(raw)
		reader.Reset(rawReader)
		head, err := ReadCommandHead(reader)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ReadPacketWithHead(head, reader); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, allocations, 2.5)
}

func BenchmarkPacketWriteTo(b *testing.B) {
	packet := benchPacket()
	buf := &bytes.Buffer{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := packet.WriteTo(buf); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadPacket(b *testing.B) {
	packet := benchPacket()
	var buf bytes.Buffer
	if err := packet.WriteTo(&buf); err != nil {
		b.Fatal(err)
	}
	raw := buf.Bytes()

	rawReader := bytes.NewReader(raw)
	reader := bufio.NewReaderSize(rawReader, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rawReader.Reset(raw)
		reader.Reset(rawReader)
		head, err := ReadCommandHead(reader)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := ReadPacketWithHead(head, reader); err != nil {
			b.Fatal(err)
		}
	}
}
