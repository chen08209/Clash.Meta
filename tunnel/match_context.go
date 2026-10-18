package tunnel

import (
	"context"
	"path/filepath"

	"github.com/metacubex/mihomo/component/process"
	"github.com/metacubex/mihomo/component/resolver"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/constant/features"
	"github.com/metacubex/mihomo/log"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
)

// matchContext carries the state the lazy match helper closes over. It is
// created once per connection, when the first rule that needs the helper is
// visited.
type matchContext struct {
	metadata             *C.Metadata
	resolved             bool
	attemptProcessLookup bool
}

func (c *matchContext) resolveIP() {
	if c.resolved || c.metadata.Host == "" || c.metadata.Resolved() {
		return
	}
	ctx, cancel := context.WithTimeout(
		resolver.WithInitiator(context.Background(), resolver.InitiatorRule),
		resolver.DefaultDNSTimeout,
	)
	defer cancel()
	ip, err := resolver.ResolveIP(ctx, c.metadata.Host)
	if err != nil {
		log.Debugln("[DNS] resolve %s error: %s", c.metadata.Host, err.Error())
	} else {
		if log.DebugEnabled() {
			log.Debugln("[DNS] %s --> %s", c.metadata.Host, ip.String())
		}
		c.metadata.DstIP = ip
	}
	c.resolved = true
}

func (c *matchContext) findProcess() {
	if !c.attemptProcessLookup {
		return
	}
	c.attemptProcessLookup = false
	metadata := c.metadata
	if !features.Android {
		uid, path, err := process.FindProcessName(
			metadata.NetWork.String(),
			metadata.SrcIP,
			int(metadata.SrcPort),
		)
		if err != nil {
			log.Debugln("[Process] find process error for %s: %v", metadata.String(), err)
			return
		}
		metadata.Process = filepath.Base(path)
		metadata.ProcessPath = path
		metadata.Uid = uid
		if pkg, err := process.FindPackageName(metadata); err == nil {
			metadata.Process = pkg
		}
		return
	}

	pkg, err := process.FindPackageName(metadata)
	if err != nil {
		log.Debugln("[Process] find process error for %s: %v", metadata.String(), err)
		return
	}
	metadata.Process = pkg
}

func (c *matchContext) checkPassRule(adapterName string) bool {
	adapter, ok := proxies[adapterName]
	if !ok {
		return false
	}
	for adapter := adapter; adapter != nil; adapter = adapter.Unwrap(c.metadata, false) {
		if adapter.Type() == C.PassRule {
			return true
		}
	}
	return false
}

// ruleUsesMatchHelper reports whether the rule reads the match helper. The
// listed types never touch it; everything else may, so it is treated as a
// helper user.
func ruleUsesMatchHelper(rule C.Rule) bool {
	for rule != nil {
		switch rule.(type) {
		case *rulesCommon.Domain,
			*rulesCommon.DomainKeyword,
			*rulesCommon.DomainRegex,
			*rulesCommon.DomainSuffix,
			*rulesCommon.DomainWildcard,
			*rulesCommon.DSCP,
			*rulesCommon.InName,
			*rulesCommon.InType,
			*rulesCommon.InUser,
			*rulesCommon.Match,
			*rulesCommon.NetworkType,
			*rulesCommon.Port:
			return false
		}
		wrapped, ok := rule.(interface{ Unwrap() C.Rule })
		if !ok {
			break
		}
		rule = wrapped.Unwrap()
	}
	return true
}
