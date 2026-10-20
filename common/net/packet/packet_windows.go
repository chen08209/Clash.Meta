//go:build windows

package packet

import (
	"net"
	"net/netip"
	"strconv"
	"syscall"

	"github.com/metacubex/mihomo/common/pool"

	"golang.org/x/sys/windows"
)

type enhanceUDPConn struct {
	*net.UDPConn
	rawConn syscall.RawConn
}

func (c *enhanceUDPConn) WaitReadFrom() (data []byte, put func(), addr netip.AddrPort, err error) {
	if c.rawConn == nil {
		c.rawConn, _ = c.UDPConn.SyscallConn()
	}
	var readErr error
	hasData := false
	err = c.rawConn.Read(func(fd uintptr) (done bool) {
		if !hasData {
			hasData = true
			// A zero-byte read only waits for readiness; Recvfrom runs on the next callback.
			return false
		}
		readBuf := pool.Get(pool.UDPBufferSize)
		put = func() {
			_ = pool.Put(readBuf)
		}
		var readFrom windows.Sockaddr
		var readN int
		readN, readFrom, readErr = windows.Recvfrom(windows.Handle(fd), readBuf, 0)
		if readN > 0 {
			data = readBuf[:readN]
		} else {
			put()
			put = nil
			data = nil
		}
		if readErr == windows.WSAEWOULDBLOCK {
			return false
		}
		switch from := readFrom.(type) {
		case *windows.SockaddrInet4:
			addr = netip.AddrPortFrom(netip.AddrFrom4(from.Addr), uint16(from.Port))
		case *windows.SockaddrInet6:
			zone := ""
			if from.ZoneId != 0 {
				zone = strconv.FormatInt(int64(from.ZoneId), 10)
			}
			addr = netip.AddrPortFrom(netip.AddrFrom16(from.Addr).WithZone(zone).Unmap(), uint16(from.Port))
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
