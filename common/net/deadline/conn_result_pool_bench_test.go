package deadline

import (
	"net"
	"testing"
	"time"
)

type immediateConn struct {
	data []byte
}

func (c *immediateConn) Read(p []byte) (int, error) { return copy(p, c.data), nil }
func (*immediateConn) Write(p []byte) (int, error)  { return len(p), nil }
func (*immediateConn) Close() error                 { return nil }
func (*immediateConn) LocalAddr() net.Addr          { return nil }
func (*immediateConn) RemoteAddr() net.Addr         { return nil }
func (*immediateConn) SetDeadline(time.Time) error  { return nil }
func (*immediateConn) SetReadDeadline(time.Time) error {
	return nil
}
func (*immediateConn) SetWriteDeadline(time.Time) error { return nil }

func BenchmarkConnReadDeadline(b *testing.B) {
	conn := NewConn(&immediateConn{data: make([]byte, 64)})
	if err := conn.SetReadDeadline(time.Now().Add(time.Hour)); err != nil {
		b.Fatal(err)
	}
	buffer := make([]byte, 64)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := conn.Read(buffer); err != nil {
			b.Fatal(err)
		}
	}
}
