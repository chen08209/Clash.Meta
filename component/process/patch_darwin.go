package process

import (
	"net/netip"
	"os"
	"slices"
	"sync"
	"syscall"
	"unsafe"
)

// macOS lists in net.inet.*.pcblist_n only the caller's own sockets when a
// third-party app started the caller, as FlClash starts its core, so
// findProcessName finds no other process there. libproc still answers for
// every process of the same user, which is how lsof reads sockets.

const (
	procInfoCallListPids  = 0x1
	procInfoCallPidFdInfo = 0x3
	procAllPids           = 0x1
	procPidListFds        = 0x1
	procPidFdSocketInfo   = 0x3

	proxFdTypeSocket = 0x2
	sockInfoIn       = 0x1
	sockInfoTCP      = 0x2
	ipProtoUDP       = 17
	iniIPv4          = 0x1
	iniIPv6          = 0x2

	// Offsets into struct socket_fdinfo of <sys/proc_info.h>, the same on
	// arm64 and amd64. tcp_sockinfo starts with its in_sockinfo, so TCP and
	// UDP sockets keep their addresses at the same place.
	procFdInfoSize   = 8
	socketFdInfoSize = 792
	soiProtocol      = 24 + 156
	soiKind          = 24 + 232
	insi             = 24 + 240
	insiLport        = insi + 4
	insiVflag        = insi + 24
	insiLaddr        = insi + 48
)

// recentPids holds the processes the last lookups found, tried before every
// other: a browser opens its connections in bursts from one process.
var recentPids struct {
	sync.Mutex
	pids [8]int32
	next int
}

func findProcessByFd(network string, ip netip.Addr, port int) (uint32, string, error) {
	recentPids.Lock()
	recent := recentPids.pids
	recentPids.Unlock()

	s := fdScan{network: network, ip: ip, port: port, info: make([]byte, socketFdInfoSize)}
	for _, pid := range recent {
		if pid > 0 && s.match(pid) {
			return s.found(pid)
		}
	}
	self := int32(os.Getpid())
	for _, pid := range listPids() {
		if pid <= 0 || pid == self || slices.Contains(recent[:], pid) {
			continue
		}
		if s.match(pid) {
			rememberPid(pid)
			return s.found(pid)
		}
	}
	if s.fallbackUDPPid != 0 {
		return s.found(s.fallbackUDPPid)
	}
	return 0, "", ErrNotFound
}

func rememberPid(pid int32) {
	recentPids.Lock()
	defer recentPids.Unlock()
	recentPids.pids[recentPids.next] = pid
	recentPids.next = (recentPids.next + 1) % len(recentPids.pids)
}

type fdScan struct {
	network        string
	ip             netip.Addr
	port           int
	info           []byte
	fds            []byte
	fallbackUDPPid int32
}

func (s *fdScan) found(pid int32) (uint32, string, error) {
	path, err := getExecPathFromPID(uint32(pid))
	return 0, path, err
}

// match reports whether pid holds the socket bound to the source address.
func (s *fdScan) match(pid int32) bool {
	isIPv4 := s.ip.Is4()
	s.fds = listFds(pid, s.fds)
	for i := 0; i+procFdInfoSize <= len(s.fds); i += procFdInfoSize {
		if readNativeUint32(s.fds[i+4:i+8]) != proxFdTypeSocket {
			continue
		}
		fd := uintptr(readNativeUint32(s.fds[i : i+4]))
		n, _, errno := syscall.Syscall6(syscall.SYS_PROC_INFO, procInfoCallPidFdInfo, uintptr(pid),
			procPidFdSocketInfo, fd, uintptr(unsafe.Pointer(&s.info[0])), socketFdInfoSize)
		if errno != 0 || n != socketFdInfoSize {
			continue
		}
		info := s.info
		kind := readNativeUint32(info[soiKind : soiKind+4])
		switch s.network {
		case TCP:
			if kind != sockInfoTCP {
				continue
			}
		case UDP:
			if kind != sockInfoIn || readNativeUint32(info[soiProtocol:soiProtocol+4]) != ipProtoUDP {
				continue
			}
		}
		// insi_lport holds the port in network byte order.
		lport := uint16(readNativeUint32(info[insiLport : insiLport+4]))
		if int(lport>>8|lport<<8) != s.port {
			continue
		}
		var srcIP netip.Addr
		switch flag := info[insiVflag]; {
		case flag&iniIPv4 > 0 && isIPv4:
			srcIP, _ = netip.AddrFromSlice(info[insiLaddr+12 : insiLaddr+16])
		case flag&iniIPv6 > 0 && !isIPv4:
			srcIP, _ = netip.AddrFromSlice(info[insiLaddr : insiLaddr+16])
		default:
			continue
		}
		srcIP = srcIP.Unmap()
		if s.ip == srcIP {
			return true
		}
		if s.network == UDP && srcIP.IsUnspecified() && s.fallbackUDPPid == 0 {
			s.fallbackUDPPid = pid
		}
	}
	return false
}

func listPids() []int32 {
	buf := make([]int32, 1024)
	for {
		n, _, errno := syscall.Syscall6(syscall.SYS_PROC_INFO, procInfoCallListPids, procAllPids, 0, 0,
			uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*4))
		if errno != 0 {
			return nil
		}
		if count := int(n) / 4; count < len(buf) {
			return buf[:count]
		}
		buf = make([]int32, len(buf)*2)
	}
}

// listFds reuses buf for the descriptors of pid, or returns it empty when pid
// is gone or belongs to another user.
func listFds(pid int32, buf []byte) []byte {
	if cap(buf) == 0 {
		buf = make([]byte, 256*procFdInfoSize)
	}
	buf = buf[:cap(buf)]
	for {
		n, _, errno := syscall.Syscall6(syscall.SYS_PROC_INFO, proccallnumpidinfo, uintptr(pid), procPidListFds, 0,
			uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if errno != 0 {
			return buf[:0]
		}
		if int(n) < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, len(buf)*2)
	}
}
