package core

import "testing"

func BenchmarkQuicPktConnReadFrom(b *testing.B) {
	msgCh := make(chan *udpMessage, 1)
	conn := &quicPktConn{MsgCh: msgCh}
	msg := &udpMessage{Host: "203.0.113.7", Port: 443, Data: make([]byte, 1350)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgCh <- msg
		if _, _, err := conn.ReadFrom(); err != nil {
			b.Fatal(err)
		}
	}
}
