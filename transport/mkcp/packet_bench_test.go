package mkcp

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

const benchDataPayloadSize = 1350

func buildDataPacket(payloadLen int) []byte {
	b := make([]byte, dataSegmentOverhead+payloadLen)
	binary.BigEndian.PutUint16(b, 7)
	b[2] = byte(commandData)
	binary.BigEndian.PutUint32(b[4:], 1000)
	binary.BigEndian.PutUint32(b[8:], 1)
	binary.BigEndian.PutUint32(b[12:], 1)
	binary.BigEndian.PutUint16(b[16:], uint16(payloadLen))
	return b
}

func buildAckPacket(count int) []byte {
	b := make([]byte, 17+count*4)
	binary.BigEndian.PutUint16(b, 7)
	b[2] = byte(commandACK)
	binary.BigEndian.PutUint32(b[4:], 32)
	binary.BigEndian.PutUint32(b[8:], 1)
	binary.BigEndian.PutUint32(b[12:], 1000)
	b[16] = byte(count)
	for i := 0; i < count; i++ {
		binary.BigEndian.PutUint32(b[17+i*4:], uint32(i+1))
	}
	return b
}

// A parsed data payload aliases the datagram buffer the caller already owns,
// so the reader only pays for the segment and the result slice.
func TestReadSegmentDoesNotCopyTheDataPayload(t *testing.T) {
	reader := packetReader{}
	packet := buildDataPacket(benchDataPayloadSize)

	allocations := testing.AllocsPerRun(1000, func() {
		if segments := reader.read(packet); len(segments) != 1 {
			t.Fatal("bad parse")
		}
	})
	require.LessOrEqual(t, int(allocations), 2)
}

func BenchmarkReadDataSegment(b *testing.B) {
	reader := packetReader{}
	packet := buildDataPacket(benchDataPayloadSize)
	b.ReportAllocs()
	b.SetBytes(int64(len(packet)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if segments := reader.read(packet); len(segments) != 1 {
			b.Fatal("bad parse")
		}
	}
}

func BenchmarkReadAckSegment(b *testing.B) {
	reader := packetReader{}
	packet := buildAckPacket(3)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if segments := reader.read(packet); len(segments) != 1 {
			b.Fatal("bad parse")
		}
	}
}
