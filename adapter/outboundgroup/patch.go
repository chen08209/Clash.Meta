package outboundgroup

import (
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

// joinProviderProxies puts a `use` provider named in use-position after that
// many of the group's own proxies, which otherwise all come first.
func (gb *GroupBase) joinProviderProxies(parts [][]C.Proxy) []C.Proxy {
	own := -1
	if len(gb.usePosition) != 0 {
		for i, pd := range gb.providers {
			if pd.VehicleType() == P.Compatible {
				own = i
				break
			}
		}
	}

	var proxies []C.Proxy
	if own < 0 {
		for _, part := range parts {
			proxies = append(proxies, part...)
		}
		return proxies
	}

	named := parts[own]
	before := make([][]C.Proxy, len(named)+1)
	for i, pd := range gb.providers {
		if i == own {
			continue
		}
		position, ok := gb.usePosition[pd.Name()]
		if !ok || position > len(named) {
			position = len(named)
		} else if position < 0 {
			position = 0
		}
		before[position] = append(before[position], parts[i]...)
	}
	for i, part := range before {
		proxies = append(proxies, part...)
		if i < len(named) {
			proxies = append(proxies, named[i])
		}
	}
	return proxies
}
