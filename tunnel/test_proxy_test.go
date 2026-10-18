package tunnel

import (
	C "github.com/metacubex/mihomo/constant"
)

type directTestProxy struct {
	C.Proxy
}

func (directTestProxy) Name() string {
	return "DIRECT"
}

func (directTestProxy) Type() C.AdapterType {
	return C.Direct
}

func (directTestProxy) Adapter() C.ProxyAdapter {
	return nil
}

func (directTestProxy) Unwrap(*C.Metadata, bool) C.Proxy {
	return nil
}
