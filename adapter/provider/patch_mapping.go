package provider

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/metacubex/mihomo/common/convert"
	"github.com/metacubex/mihomo/common/structure"
	"github.com/metacubex/mihomo/common/yaml"
	"github.com/metacubex/mihomo/component/age"
	P "github.com/metacubex/mihomo/constant/provider"

	"github.com/dlclark/regexp2"
)

// ProxyMapping returns the mapping pd, a proxy provider configured by mapping,
// builds its proxy named name from, with the provider's dialer-proxy and
// override applied, or nil when pd has no proxy of that name. It picks the
// mapping the way NewProxiesParser does and must keep in step with it.
func ProxyMapping(pd P.ProxyProvider, mapping map[string]any, name string) (map[string]any, error) {
	decoder := structure.NewDecoder(structure.Option{TagName: "provider", WeaklyTypedInput: true})
	schema := &proxyProviderSchema{}
	if err := decoder.Decode(mapping, schema); err != nil {
		return nil, err
	}

	var excludeTypes []string
	if schema.ExcludeType != "" {
		excludeTypes = strings.Split(schema.ExcludeType, "|")
	}
	var excludeFilterRegs []*regexp2.Regexp
	if schema.ExcludeFilter != "" {
		regs, err := compileFilters(schema.ExcludeFilter)
		if err != nil {
			return nil, err
		}
		excludeFilterRegs = regs
	}
	filterRegs, err := compileFilters(schema.Filter)
	if err != nil {
		return nil, err
	}

	buf, err := providerContent(pd, schema.Payload)
	if err != nil {
		return nil, err
	}
	if buf, err = age.DecryptBytes(buf, schema.AgeSecretKey); err != nil {
		return nil, err
	}
	source := &ProxySchema{}
	if err := yaml.Unmarshal(buf, source); err != nil {
		proxies, err1 := convert.ConvertsV2Ray(buf)
		if err1 != nil {
			return nil, fmt.Errorf("%w, %w", err, err1)
		}
		source.Proxies = proxies
	}

	matches := func(reg *regexp2.Regexp, name string) bool {
		mat, _ := reg.MatchString(name)
		return mat
	}
	seen := map[string]struct{}{}
	for _, filterReg := range filterRegs {
		for _, proxy := range source.Proxies {
			if len(excludeTypes) > 0 {
				proxyType, ok := proxy["type"].(string)
				if !ok || slices.ContainsFunc(excludeTypes, func(excluded string) bool {
					return strings.EqualFold(proxyType, excluded)
				}) {
					continue
				}
			}
			proxyName, ok := proxy["name"].(string)
			if !ok || slices.ContainsFunc(excludeFilterRegs, func(reg *regexp2.Regexp) bool {
				return matches(reg, proxyName)
			}) {
				continue
			}
			if schema.Filter != "" && !matches(filterReg, proxyName) {
				continue
			}
			if _, ok := seen[proxyName]; ok {
				continue
			}
			if schema.DialerProxy != "" {
				proxy["dialer-proxy"] = schema.DialerProxy
			}
			if err := schema.Override.Apply(proxy); err != nil {
				return nil, err
			}
			if proxy["name"] == name {
				return proxy, nil
			}
			seen[proxyName] = struct{}{}
		}
	}
	return nil, nil
}

func compileFilters(filter string) ([]*regexp2.Regexp, error) {
	var regs []*regexp2.Regexp
	for _, expr := range strings.Split(filter, "`") {
		reg, err := regexp2.Compile(expr, regexp2.None)
		if err != nil {
			return nil, fmt.Errorf("invalid filter regex: %w", err)
		}
		regs = append(regs, reg)
	}
	return regs, nil
}

// A fetched provider keeps its payload until its vehicle's file loads.
func providerContent(pd P.ProxyProvider, payload []map[string]any) ([]byte, error) {
	if set, ok := pd.(*ProxySetProvider); ok {
		buf, err := os.ReadFile(set.Vehicle().Path())
		if !errors.Is(err, os.ErrNotExist) {
			return buf, err
		}
	}
	return yaml.Marshal(ProxySchema{Proxies: payload})
}
