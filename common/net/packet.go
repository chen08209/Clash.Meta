package net

import (
	"github.com/metacubex/mihomo/common/net/deadline"
	"github.com/metacubex/mihomo/common/net/packet"
)

type EnhancePacketConn = packet.EnhancePacketConn
type WaitReadFrom = packet.WaitReadFrom
type AddrPortWaitReadFrom = packet.AddrPortWaitReadFrom

var NewEnhancePacketConn = packet.NewEnhancePacketConn
var NewThreadSafePacketConn = packet.NewThreadSafePacketConn
var NewRefPacketConn = packet.NewRefPacketConn
var AddrPortFromNetAddr = packet.AddrPortFromNetAddr
var WaitReadFromAddrPort = packet.WaitReadFromAddrPort

var NewDeadlineNetPacketConn = deadline.NewNetPacketConn
var NewDeadlineEnhancePacketConn = deadline.NewEnhancePacketConn
var NewDeadlineSingPacketConn = deadline.NewSingPacketConn
var NewDeadlineEnhanceSingPacketConn = deadline.NewEnhanceSingPacketConn
