package sudoku

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/metacubex/mihomo/common/pool"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/transport/socks5"
)

const (
	maxUoTPayload = 64 * 1024
)

// WriteDatagram sends a single UDP datagram frame over a reliable stream.
func WriteDatagram(w io.Writer, addr socks5.Addr, payload []byte) error {
	if addrLen := len(addr); addrLen == 0 || addrLen > maxUoTPayload {
		return fmt.Errorf("address too long: %d", addrLen)
	}
	if payloadLen := len(payload); payloadLen > maxUoTPayload {
		return fmt.Errorf("payload too large: %d", payloadLen)
	}

	var header [4]byte
	binary.BigEndian.PutUint16(header[:2], uint16(len(addr)))
	binary.BigEndian.PutUint16(header[2:], uint16(len(payload)))

	return writeAllChunks(w, header[:], addr, payload)
}

// ReadDatagram parses a single UDP datagram frame from the reliable stream.
func ReadDatagram(r io.Reader) (socks5.Addr, []byte, error) {
	addrLen, payloadLen, err := readDatagramHeader(r)
	if err != nil {
		return nil, nil, err
	}

	addrBuf := pool.Get(addrLen)
	if _, err := io.ReadFull(r, addrBuf); err != nil {
		_ = pool.Put(addrBuf)
		return nil, nil, err
	}
	addr := socks5.SplitAddr(addrBuf)
	if addr == nil {
		_ = pool.Put(addrBuf)
		return nil, nil, errors.New("invalid address")
	}

	payload := pool.Get(payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		_ = pool.Put(addrBuf)
		_ = pool.Put(payload)
		return nil, nil, err
	}

	return addr, payload, nil
}

// ReleaseDatagram returns the address and payload buffers obtained from
// ReadDatagram to their pools.
func ReleaseDatagram(addr socks5.Addr, payload []byte) {
	_ = pool.Put(addr)
	_ = pool.Put(payload)
}

// UoTPacketConn adapts a net.Conn with the Sudoku UoT framing to net.PacketConn.
type UoTPacketConn struct {
	conn    net.Conn
	writeMu sync.Mutex
}

func NewUoTPacketConn(conn net.Conn) *UoTPacketConn {
	return &UoTPacketConn{conn: conn}
}

func (c *UoTPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		addr, payloadLen, err := readDatagramHeaderAndAddress(c.conn)
		if err != nil {
			return 0, nil, err
		}

		udpAddr := addr.UDPAddr()
		_ = pool.Put(addr)
		if payloadLen > len(p) {
			if discardErr := discardBytes(c.conn, payloadLen); discardErr != nil {
				return 0, nil, discardErr
			}
			return 0, nil, io.ErrShortBuffer
		}
		if udpAddr == nil {
			if discardErr := discardBytes(c.conn, payloadLen); discardErr != nil {
				return 0, nil, discardErr
			}
			log.Debugln("[Sudoku][UoT] discard datagram with invalid address")
			continue
		}
		if _, err := io.ReadFull(c.conn, p[:payloadLen]); err != nil {
			return 0, nil, err
		}
		return payloadLen, udpAddr, nil
	}
}

func (c *UoTPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if addr == nil {
		return 0, errors.New("address is nil")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	target := socks5.ParseAddrToSocksAddr(addr)
	if target == nil {
		return 0, errors.New("invalid address")
	}
	if err := WriteDatagram(c.conn, target, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *UoTPacketConn) Close() error {
	return c.conn.Close()
}

func (c *UoTPacketConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *UoTPacketConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *UoTPacketConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *UoTPacketConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func readDatagramHeader(r io.Reader) (int, int, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, 0, err
	}

	addrLen := int(binary.BigEndian.Uint16(header[:2]))
	payloadLen := int(binary.BigEndian.Uint16(header[2:]))
	if addrLen <= 0 || addrLen > maxUoTPayload {
		return 0, 0, fmt.Errorf("invalid address length: %d", addrLen)
	}
	if payloadLen < 0 || payloadLen > maxUoTPayload {
		return 0, 0, fmt.Errorf("invalid payload length: %d", payloadLen)
	}
	return addrLen, payloadLen, nil
}

func readDatagramHeaderAndAddress(r io.Reader) (socks5.Addr, int, error) {
	addrLen, payloadLen, err := readDatagramHeader(r)
	if err != nil {
		return nil, 0, err
	}

	addrBuf := pool.Get(addrLen)
	if _, err := io.ReadFull(r, addrBuf); err != nil {
		_ = pool.Put(addrBuf)
		return nil, 0, err
	}

	addr := socks5.SplitAddr(addrBuf)
	if addr == nil {
		_ = pool.Put(addrBuf)
		return nil, 0, errors.New("invalid address")
	}
	return addr, payloadLen, nil
}

func discardBytes(r io.Reader, n int) error {
	if n <= 0 {
		return nil
	}
	_, err := io.CopyN(io.Discard, r, int64(n))
	return err
}
