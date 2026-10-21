package context

import (
	"net"

	"github.com/gofrs/uuid/v5"
	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
)

type ConnContext struct {
	id       uuid.UUID
	metadata *C.Metadata
	conn     net.Conn
}

func NewConnContext(conn net.Conn, metadata *C.Metadata) *ConnContext {
	return &ConnContext{
		id:       utils.NewUUIDV4(),
		metadata: metadata,
		conn:     conn,
	}
}

// ID implement C.ConnContext ID
func (c *ConnContext) ID() uuid.UUID {
	return c.id
}

// Metadata implement C.ConnContext Metadata
func (c *ConnContext) Metadata() *C.Metadata {
	return c.metadata
}

// Conn implement C.ConnContext Conn
func (c *ConnContext) Conn() net.Conn {
	return c.conn
}
