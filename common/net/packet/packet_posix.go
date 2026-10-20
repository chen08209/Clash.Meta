//go:build !windows

package packet

import (
	"net"
	"net/netip"
	"strconv"
	"syscall"

	"github.com/metacubex/mihomo/common/pool"
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
	err = c.rawConn.Read(func(fd uintptr) (done bool) {
		readBuf := pool.Get(pool.UDPBufferSize)
		put = func() {
			_ = pool.Put(readBuf)
		}
		var readFrom syscall.Sockaddr
		var readN int
		readN, _, _, readFrom, readErr = syscall.Recvmsg(int(fd), readBuf, nil, 0)
		if readN > 0 {
			data = readBuf[:readN]
		} else {
			put()
			put = nil
			data = nil
		}
		if readErr == syscall.EAGAIN {
			return false
		}
		if readFrom != nil {
			switch from := readFrom.(type) {
			case *syscall.SockaddrInet4:
				addr = netip.AddrPortFrom(netip.AddrFrom4(from.Addr).Unmap(), uint16(from.Port))
			case *syscall.SockaddrInet6:
				zone := ""
				if from.ZoneId != 0 {
					zone = strconv.FormatInt(int64(from.ZoneId), 10)
				}
				addr = netip.AddrPortFrom(netip.AddrFrom16(from.Addr).WithZone(zone).Unmap(), uint16(from.Port))
			}
		}
		// A zero-length datagram is a successful UDP read, not EOF.
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
	err = c.rawConn.Read(func(fd uintptr) (done bool) {
		readBuf := pool.Get(pool.UDPBufferSize)
		put = func() {
			_ = pool.Put(readBuf)
		}
		var readFrom syscall.Sockaddr
		var readN int
		readN, _, _, readFrom, readErr = syscall.Recvmsg(int(fd), readBuf, nil, 0)
		if readN > 0 {
			data = readBuf[:readN]
		} else {
			put()
			put = nil
			data = nil
		}
		if readErr == syscall.EAGAIN {
			return false
		}
		if readFrom != nil {
			switch from := readFrom.(type) {
			case *syscall.SockaddrInet4:
				ip := from.Addr // copy from.Addr; ip escapes, so this line allocates 4 bytes
				addr = &net.UDPAddr{IP: ip[:], Port: from.Port}
			case *syscall.SockaddrInet6:
				ip := from.Addr // copy from.Addr; ip escapes, so this line allocates 16 bytes
				zone := ""
				if from.ZoneId != 0 {
					zone = strconv.FormatInt(int64(from.ZoneId), 10)
				}
				addr = &net.UDPAddr{IP: ip[:], Port: from.Port, Zone: zone}
			}
		}
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
