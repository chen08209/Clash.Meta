package net

import (
	"bufio"
	"net"

	"github.com/metacubex/mihomo/common/buf"
)

var _ ExtendedConn = (*BufferedConn)(nil)

// InitialPeekBufferSize is the read buffer a BufferedConn starts with. Peeking
// is its only reason to exist and the sniffer grows it on demand, so it starts
// small; a relay never reads through it once it is empty.
const InitialPeekBufferSize = 512

type BufferedConn struct {
	r *bufio.Reader
	net.Conn
	peeked bool
}

func NewBufferedConn(c net.Conn) *BufferedConn {
	if bc, ok := c.(*BufferedConn); ok {
		return bc
	}
	return &BufferedConn{r: bufio.NewReaderSize(c, InitialPeekBufferSize), Conn: c}
}

func WarpConnWithBioReader(c net.Conn, br *bufio.Reader) net.Conn {
	if br != nil && br.Buffered() > 0 {
		if bc, ok := c.(*BufferedConn); ok && bc.r == br {
			return bc
		}
		return &BufferedConn{r: br, Conn: c, peeked: true}
	}
	return c
}

// Reader returns the internal bufio.Reader.
func (c *BufferedConn) Reader() *bufio.Reader {
	return c.r
}

func (c *BufferedConn) ResetPeeked() {
	c.peeked = false
}

func (c *BufferedConn) Peeked() bool {
	return c.peeked
}

// Peek returns the next n bytes without advancing the reader.
func (c *BufferedConn) Peek(n int) ([]byte, error) {
	c.peeked = true
	return c.r.Peek(n)
}

func (c *BufferedConn) Discard(n int) (discarded int, err error) {
	return c.r.Discard(n)
}

func (c *BufferedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

func (c *BufferedConn) ReadByte() (byte, error) {
	return c.r.ReadByte()
}

func (c *BufferedConn) UnreadByte() error {
	return c.r.UnreadByte()
}

func (c *BufferedConn) Buffered() int {
	return c.r.Buffered()
}

func (c *BufferedConn) ReadBuffer(buffer *buf.Buffer) (err error) {
	if c.r != nil && c.r.Buffered() > 0 {
		_, err = buffer.ReadOnceFrom(c.r)
		return
	}
	if extended, ok := c.Conn.(ExtendedConn); ok {
		return extended.ReadBuffer(buffer)
	}
	_, err = buffer.ReadOnceFrom(c.Conn)
	return err
}

func (c *BufferedConn) WriteBuffer(buffer *buf.Buffer) (err error) {
	if extended, ok := c.Conn.(ExtendedConn); ok {
		return extended.WriteBuffer(buffer)
	}
	_, err = buffer.WriteTo(c.Conn)
	return err
}

func (c *BufferedConn) ReadCached() *buf.Buffer { // call in sing/common/bufio.Copy
	if c.r != nil && c.r.Buffered() > 0 {
		length := c.r.Buffered()
		b, _ := c.r.Peek(length)
		_, _ = c.r.Discard(length)
		return buf.As(b)
	}
	c.r = nil // drop bufio.Reader to let gc can clean up its internal buf
	return nil
}

func (c *BufferedConn) Upstream() any {
	return c.Conn
}

func (c *BufferedConn) ReaderReplaceable() bool {
	if c.r != nil && c.r.Buffered() > 0 {
		return false
	}
	return true
}

// WriterReplaceable returns false so relay code keeps this ExtendedWriter
// instead of unwrapping to a raw conn and wrapping it again.
func (c *BufferedConn) WriterReplaceable() bool {
	return false
}
