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

func (c *enhanceUDPConn) WaitReadFromAddrPort() (data []byte, put func(), addr netip.AddrPort, rawAddr net.Addr, err error) {
	if c.rawConn == nil {
		c.rawConn, _ = c.UDPConn.SyscallConn()
	}
	var readErr error
	hasData := false
	err = c.rawConn.Read(func(fd uintptr) (done bool) {
		if !hasData {
			hasData = true
			// golang's internal/poll.FD.RawRead will Use a zero-byte read as a way to get notified when this
			// socket is readable if we return false. So the `recvfrom` syscall will not block the system thread.
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
		if readFrom != nil {
			switch from := readFrom.(type) {
			case *windows.SockaddrInet4:
				addr = netip.AddrPortFrom(netip.AddrFrom4(from.Addr).Unmap(), uint16(from.Port))
			case *windows.SockaddrInet6:
				zone := ""
				if from.ZoneId != 0 {
					zone = strconv.FormatInt(int64(from.ZoneId), 10)
				}
				addr = netip.AddrPortFrom(netip.AddrFrom16(from.Addr).WithZone(zone).Unmap(), uint16(from.Port))
			}
		}
		// A zero-length datagram is a successful UDP read, not EOF.
		hasData = false
		return true
	})
	if err != nil {
		return
	}
	if readErr != nil {
		err = readErr
		return
	}
	return
}

func (c *enhanceUDPConn) WaitReadFrom() (data []byte, put func(), addr net.Addr, err error) {
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
		if readFrom != nil {
			switch from := readFrom.(type) {
			case *windows.SockaddrInet4:
				ip := from.Addr // copy from.Addr; ip escapes, so this line allocates 4 bytes
				addr = &net.UDPAddr{IP: ip[:], Port: from.Port}
			case *windows.SockaddrInet6:
				ip := from.Addr // copy from.Addr; ip escapes, so this line allocates 16 bytes
				zone := ""
				if from.ZoneId != 0 {
					zone = strconv.FormatInt(int64(from.ZoneId), 10)
				}
				addr = &net.UDPAddr{IP: ip[:], Port: from.Port, Zone: zone}
			}
		}
		hasData = false
		return true
	})
	if err != nil {
		return
	}
	if readErr != nil {
		err = readErr
		return
	}
	return
}
