//go:build windows

package packet

import (
	"net"
	"net/netip"
	"syscall"
	"unsafe"

	"github.com/metacubex/mihomo/common/pool"

	"golang.org/x/sys/windows"
)

type enhanceUDPConn struct {
	*net.UDPConn
	rawConn syscall.RawConn
}

func (c *enhanceUDPConn) WaitReadFrom() (data []byte, addr netip.AddrPort, err error) {
	if c.rawConn == nil {
		c.rawConn, _ = c.UDPConn.SyscallConn()
	}
	var readErr error
	hasData := false
	err = c.rawConn.Read(func(fd uintptr) (done bool) {
		if !hasData {
			hasData = true
			// A zero-byte read only waits for readiness; recvfrom runs on the next callback.
			return false
		}
		readBuf := pool.Get(pool.UDPBufferSize)
		wsaBuf := &windows.WSABuf{Len: uint32(len(readBuf)), Buf: &readBuf[0]}
		var rsa windows.RawSockaddrAny
		rsaLen := int32(unsafe.Sizeof(rsa))
		var recvd, flags uint32
		readErr = windows.WSARecvFrom(windows.Handle(fd), wsaBuf, 1, &recvd, &flags, &rsa, &rsaLen, nil, nil)
		if readErr == windows.WSAEWOULDBLOCK {
			_ = pool.Put(readBuf)
			return false
		}
		if readErr != nil {
			_ = pool.Put(readBuf)
			return true
		}
		switch family := rsa.Addr.Family; family {
		case windows.AF_INET:
			addr = sockaddrInet4ToAddrPort((*syscall.RawSockaddrInet4)(unsafe.Pointer(&rsa.Addr)))
		case windows.AF_INET6:
			addr = sockaddrInet6ToAddrPort((*syscall.RawSockaddrInet6)(unsafe.Pointer(&rsa.Addr)))
		}
		if recvd > 0 {
			data = readBuf[:recvd]
		} else {
			_ = pool.Put(readBuf)
		}
		hasData = false
		return true
	})
	if err != nil {
		return
	}
	if readErr != nil {
		err = readErr
	}
	return
}
