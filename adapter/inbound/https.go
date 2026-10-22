package inbound

import (
	"net"

	C "github.com/metacubex/mihomo/constant"

	"github.com/metacubex/http"
)

// NewHTTPS receive CONNECT request and return ConnContext
func NewHTTPS(request *http.Request, conn net.Conn, additions ...Addition) (net.Conn, *C.Metadata) {
	metadata := parseHTTPAddr(request)
	metadata.Type = C.HTTPS
	SetSrcAddr(metadata, conn.RemoteAddr())
	SetInAddr(metadata, conn.LocalAddr())
	ApplyAdditions(metadata, additions...)
	return conn, metadata
}
