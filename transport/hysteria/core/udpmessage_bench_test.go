package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func benchUDPMessage() udpMessage {
	return udpMessage{
		SessionID: 0x01020304,
		Host:      "203.0.113.7",
		Port:      443,
		MsgID:     7,
		FragCount: 1,
		Data:      make([]byte, 1350),
	}
}

// TestUDPMessageWireFormat pins the exact bytes the hysteria server reads.
func TestUDPMessageWireFormat(t *testing.T) {
	m := udpMessage{
		SessionID: 0x01020304,
		Host:      "1.2.3.4",
		Port:      443,
		MsgID:     9,
		FragID:    1,
		FragCount: 2,
		Data:      []byte{0xde, 0xad},
	}
	want := []byte{
		0x01, 0x02, 0x03, 0x04,
		0x00, 0x07,
		'1', '.', '2', '.', '3', '.', '4',
		0x01, 0xbb,
		0x00, 0x09,
		0x01,
		0x02,
		0x00, 0x02,
		0xde, 0xad,
	}
	require.Equal(t, want, m.Pack())

	var back udpMessage
	require.NoError(t, back.Unpack(want))
	require.Equal(t, m.SessionID, back.SessionID)
	require.Equal(t, m.Host, back.Host)
	require.Equal(t, m.Port, back.Port)
	require.Equal(t, m.MsgID, back.MsgID)
	require.Equal(t, m.FragID, back.FragID)
	require.Equal(t, m.FragCount, back.FragCount)
	require.Equal(t, m.Data, back.Data)
}

func TestUDPMessageCodecAllocatesOnce(t *testing.T) {
	m := benchUDPMessage()

	packAllocations := testing.AllocsPerRun(1000, func() {
		_ = m.Pack()
	})
	require.LessOrEqual(t, int(packAllocations), 1)

	raw := m.Pack()
	unpackAllocations := testing.AllocsPerRun(1000, func() {
		var back udpMessage
		if err := back.Unpack(raw); err != nil {
			t.Fatal(err)
		}
	})
	require.LessOrEqual(t, int(unpackAllocations), 1)
}

func BenchmarkUDPMessagePack(b *testing.B) {
	m := benchUDPMessage()
	b.ReportAllocs()
	b.SetBytes(int64(m.Size()))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Pack()
	}
}

func BenchmarkUDPMessageUnpack(b *testing.B) {
	raw := benchUDPMessage().Pack()
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m udpMessage
		if err := m.Unpack(raw); err != nil {
			b.Fatal(err)
		}
	}
}
