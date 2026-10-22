package sudoku

import (
	"bytes"
	"io"
	"testing"

	"github.com/metacubex/mihomo/transport/socks5"
)

type frameReader struct {
	data []byte
	off  int
}

func (r *frameReader) Read(p []byte) (int, error) {
	if r.off == len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

func BenchmarkReadDatagram(b *testing.B) {
	var frame bytes.Buffer
	addr := socks5.ParseAddr("example.com:53")
	if err := WriteDatagram(&frame, addr, make([]byte, 64)); err != nil {
		b.Fatal(err)
	}
	encoded := frame.Bytes()
	reader := &frameReader{data: encoded}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.off = 0
		addr, payload, err := ReadDatagram(reader)
		if err != nil {
			b.Fatal(err)
		}
		ReleaseDatagram(addr, payload)
	}
}
