//go:build linux && 386

package packet

import (
	"net"
	"net/netip"
)

type enhanceUDPConn struct {
	*net.UDPConn
}

// linux/386 multiplexes socket calls through socketcall and has no
// SYS_RECVFROM constant, so the raw-recvfrom read path does not compile there
// and the read falls back to the shared pooled ReadFrom path.
func (c *enhanceUDPConn) WaitReadFrom() (data []byte, addr netip.AddrPort, err error) {
	return waitReadFrom(c.UDPConn)
}
