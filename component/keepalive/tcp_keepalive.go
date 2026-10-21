package keepalive

import (
	"net"
	"runtime"
	"time"

	"github.com/metacubex/mihomo/common/atomic"
)

var (
	keepAliveIdle     = atomic.NewInt64(0)
	keepAliveInterval = atomic.NewInt64(0)
	disableKeepAlive  = atomic.NewBool(false)
)

func SetKeepAliveIdle(t time.Duration) {
	keepAliveIdle.Store(int64(t))
}

func SetKeepAliveInterval(t time.Duration) {
	keepAliveInterval.Store(int64(t))
}

func KeepAliveIdle() time.Duration {
	return time.Duration(keepAliveIdle.Load())
}

func KeepAliveInterval() time.Duration {
	return time.Duration(keepAliveInterval.Load())
}

func SetDisableKeepAlive(disable bool) {
	if runtime.GOOS == "android" {
		setDisableKeepAlive(true)
	} else {
		setDisableKeepAlive(disable)
	}
}

func setDisableKeepAlive(disable bool) {
	disableKeepAlive.Store(disable)
}

func DisableKeepAlive() bool {
	return disableKeepAlive.Load()
}

type TCPConn interface {
	net.Conn
	SetKeepAlive(keepalive bool) error
	SetKeepAliveConfig(config net.KeepAliveConfig) error
}

func keepAliveConfig() net.KeepAliveConfig {
	config := net.KeepAliveConfig{
		Enable:   true,
		Idle:     KeepAliveIdle(),
		Interval: KeepAliveInterval(),
	}
	if !SupportTCPKeepAliveCount() {
		// it's recommended to set both Idle and Interval to non-negative values in conjunction with a -1
		// for Count on those old Windows if you intend to customize the TCP keep-alive settings.
		config.Count = -1
	}
	return config
}

func tcpKeepAlive(tcp TCPConn) {
	if DisableKeepAlive() {
		_ = tcp.SetKeepAlive(false)
	} else {
		_ = tcp.SetKeepAliveConfig(keepAliveConfig())
	}
}

func setNetDialer(dialer *net.Dialer) {
	if DisableKeepAlive() {
		dialer.KeepAlive = -1 // If negative, keep-alive probes are disabled.
		dialer.KeepAliveConfig.Enable = false
	} else {
		dialer.KeepAliveConfig = keepAliveConfig()
	}
}

func setNetListenConfig(lc *net.ListenConfig) {
	if DisableKeepAlive() {
		lc.KeepAlive = -1 // If negative, keep-alive probes are disabled.
		lc.KeepAliveConfig.Enable = false
	} else {
		lc.KeepAliveConfig = keepAliveConfig()
	}
}

func SetNetDialer(dialer *net.Dialer) {
	setNetDialer(dialer)
}

func SetNetListenConfig(lc *net.ListenConfig) {
	setNetListenConfig(lc)
}

func TCPKeepAlive(c net.Conn) {
	if tcp, ok := c.(TCPConn); ok && tcp != nil {
		tcpKeepAlive(tcp)
	}
}
