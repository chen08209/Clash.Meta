package net

import (
	"net"

	"github.com/metacubex/mihomo/common/buf"
	"github.com/metacubex/mihomo/common/once"
)

type earlyConn struct {
	ExtendedConn // only expose standard N.ExtendedConn function to outside
	resFunc      func() error
	resOnce      once.Once
	resErr       error
}

func (conn *earlyConn) Response() error {
	conn.resOnce.Do(func() {
		conn.resErr = conn.resFunc()
	})
	return conn.resErr
}

func (conn *earlyConn) Read(b []byte) (n int, err error) {
	err = conn.Response()
	if err != nil {
		return 0, err
	}
	return conn.ExtendedConn.Read(b)
}

func (conn *earlyConn) ReadBuffer(buffer *buf.Buffer) (err error) {
	err = conn.Response()
	if err != nil {
		return err
	}
	return conn.ExtendedConn.ReadBuffer(buffer)
}

func (conn *earlyConn) Upstream() any {
	return conn.ExtendedConn
}

func (conn *earlyConn) Success() bool {
	return conn.resOnce.Done() && conn.resErr == nil
}

func (conn *earlyConn) ReaderReplaceable() bool {
	return conn.Success()
}

func (conn *earlyConn) ReaderPossiblyReplaceable() bool {
	return !conn.Success()
}

func (conn *earlyConn) WriterReplaceable() bool {
	return true
}

var _ ExtendedConn = (*earlyConn)(nil)

func NewEarlyConn(c net.Conn, f func() error) net.Conn {
	return &earlyConn{ExtendedConn: NewExtendedConn(c), resFunc: f}
}
