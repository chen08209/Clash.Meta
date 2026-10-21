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
			// A zero-byte read only waits for readiness; the recv runs on the next callback.
			return false
		}
		readBuf := pool.Get(pool.UDPBufferSize)
		var buf windows.WSABuf
		buf.Len = uint32(len(readBuf))
		buf.Buf = &readBuf[0]
		var recvd uint32
		var flags uint32
		// The kernel sockaddr stays a callback local so concurrent readers
		// cannot share it; the syscall wrapper would heap-allocate one per
		// datagram instead.
		var rsa windows.RawSockaddrAny
		rsaLen := int32(unsafe.Sizeof(rsa))
		readErr = windows.WSARecvFrom(windows.Handle(fd), &buf, 1, &recvd, &flags, &rsa, &rsaLen, nil, nil)
		if readErr == windows.WSAEWOULDBLOCK {
			_ = pool.Put(readBuf)
			return false
		}
		if readErr != nil {
			_ = pool.Put(readBuf)
		} else if recvd > 0 {
			data = readBuf[:recvd]
		} else {
			// A zero-length datagram is a successful UDP read, not EOF.
			_ = pool.Put(readBuf)
		}
		// AF_INET6 is 23 on windows, not the POSIX value the shared
		// converters sit beside, so match on the windows constants.
		switch rsa.Addr.Family {
		case windows.AF_INET:
			addr = sockaddrInet4ToAddrPort((*syscall.RawSockaddrInet4)(unsafe.Pointer(&rsa)))
		case windows.AF_INET6:
			addr = sockaddrInet6ToAddrPort((*syscall.RawSockaddrInet6)(unsafe.Pointer(&rsa)))
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
