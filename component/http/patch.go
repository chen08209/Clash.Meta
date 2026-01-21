package http

import (
	"github.com/metacubex/mihomo/component/ca"

	"github.com/metacubex/tls"
)

var TLSConfigHook func(tlsConfig *tls.Config)

func applyTLSConfigHook(caOption ca.Option, tlsConfig *tls.Config) {
	if TLSConfigHook != nil && caOption == (ca.Option{}) {
		TLSConfigHook(tlsConfig)
	}
}
