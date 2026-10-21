package provider

import (
	"fmt"

	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/rules/common"
)

type classicalStrategy struct {
	rules []C.Rule
	index *classicalRuleIndex
	count int
	parse common.ParseRuleFunc
}

func (c *classicalStrategy) Behavior() P.RuleBehavior {
	return P.Classical
}

func (c *classicalStrategy) Match(metadata *C.Metadata, helper C.RuleMatchHelper) bool {
	host := metadata.RuleHost()
	for position := 0; position < len(c.rules); {
		segment := c.index.segment(position)
		if segment.exact != nil || segment.suffix != nil {
			hit := c.index.next(position, host)
			if hit < segment.end {
				if matched, _ := c.rules[hit].Match(metadata, helper); matched {
					return true
				}
			}
			position = segment.end
			continue
		}

		for ; position < segment.end; position++ {
			if matched, _ := c.rules[position].Match(metadata, helper); matched {
				return true
			}
		}
	}
	return false
}

func (c *classicalStrategy) Count() int {
	return c.count
}

func (c *classicalStrategy) Reset() {
	c.rules = nil
	c.index = nil
	c.count = 0
}

func (c *classicalStrategy) Insert(rule string) {
	r, err := c.payloadToRule(rule)
	if err != nil {
		log.Warnln("parse classical rule [%s] error: %s", rule, err.Error())
	} else {
		c.rules = append(c.rules, r)
		c.count++
	}
}

func (c *classicalStrategy) payloadToRule(rule string) (C.Rule, error) {
	tp, payload, target, params := common.ParseRulePayload(rule, false)
	switch tp {
	case "MATCH", "RULE-SET", "SUB-RULE":
		return nil, fmt.Errorf("unsupported rule type on classical rule-set: %s", tp)
	}
	if c.parse == nil {
		return nil, fmt.Errorf("no rule parser is wired for the classical rule-set")
	}
	return c.parse(tp, payload, target, params, nil)
}

func (c *classicalStrategy) FinishInsert() {
	c.index = buildClassicalRuleIndex(c.rules)
}

func NewClassicalStrategy(parse common.ParseRuleFunc) *classicalStrategy {
	return &classicalStrategy{rules: []C.Rule{}, parse: parse}
}
