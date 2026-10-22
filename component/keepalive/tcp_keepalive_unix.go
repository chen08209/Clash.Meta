//go:build unix

package keepalive

func SupportTCPKeepAliveIdle() bool {
	return true
}

func SupportTCPKeepAliveInterval() bool {
	return true
}

func SupportTCPKeepAliveCount() bool {
	return true
}
