//go:build linux

package packet

import (
	"net"
	"net/netip"
	"syscall"
	"unsafe"

	"github.com/metacubex/mihomo/common/pool"
)

type enhanceUDPConn struct {
	*net.UDPConn
	rawConn syscall.RawConn
}

// WaitReadFrom recvfrom's straight into pooled memory through a preallocated
// kernel sockaddr: the syscall wrappers would allocate a fresh Sockaddr and
// box the errno on every would-block retry.
func (c *enhanceUDPConn) WaitReadFrom() (data []byte, addr netip.AddrPort, err error) {
	if c.rawConn == nil {
		c.rawConn, _ = c.UDPConn.SyscallConn()
	}
	var readErr error
	err = c.rawConn.Read(func(fd uintptr) (done bool) {
		readBuf := pool.Get(pool.UDPBufferSize)
		var rsa syscall.RawSockaddrInet6
		rsaLen := uint32(unsafe.Sizeof(rsa))
		n, _, errno := syscall.Syscall6(syscall.SYS_RECVFROM, fd,
			uintptr(unsafe.Pointer(&readBuf[0])), uintptr(len(readBuf)), 0,
			uintptr(unsafe.Pointer(&rsa)), uintptr(unsafe.Pointer(&rsaLen)))
		readN := int(n)
		if errno == syscall.EAGAIN {
			_ = pool.Put(readBuf)
			return false
		}
		if errno != 0 {
			_ = pool.Put(readBuf)
			readErr = errno
			return true
		}
		switch rsa.Family {
		case syscall.AF_INET:
			addr = sockaddrInet4ToAddrPort((*syscall.RawSockaddrInet4)(unsafe.Pointer(&rsa)))
		case syscall.AF_INET6:
			addr = sockaddrInet6ToAddrPort((*syscall.RawSockaddrInet6)(unsafe.Pointer(&rsa)))
		}
		if readN > 0 {
			data = readBuf[:readN]
		} else {
			// A zero-length datagram is a successful UDP read, not EOF.
			_ = pool.Put(readBuf)
		}
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
