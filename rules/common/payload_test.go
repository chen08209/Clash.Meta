package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRulePayloadShapes(test *testing.T) {
	tests := []struct {
		name       string
		rule       string
		needTarget bool
		tp         string
		payload    string
		target     string
		params     []string
	}{
		{"payload only", "IP-CIDR,1.2.3.0/24", false, "IP-CIDR", "1.2.3.0/24", "", nil},
		{"payload with target", "IP-CIDR,1.2.3.0/24,DIRECT", true, "IP-CIDR", "1.2.3.0/24", "DIRECT", nil},
		{"payload target and params", "IP-CIDR,1.2.3.0/24,DIRECT,no-resolve", true, "IP-CIDR", "1.2.3.0/24", "DIRECT", []string{"no-resolve"}},
		{"payload and params without target", "IP-CIDR,1.2.3.0/24,no-resolve", false, "IP-CIDR", "1.2.3.0/24", "", []string{"no-resolve"}},
		{"match carries the target only", "MATCH,DIRECT", false, "MATCH", "", "DIRECT", nil},
		{"lowercase type is upper cased", "domain,example.com", false, "DOMAIN", "example.com", "", nil},
		{"spaces around the items are trimmed", " DOMAIN , example.com ", false, "DOMAIN", "example.com", "", nil},
		{"sub-rule keeps its commas in the payload", "SUB-RULE,rule-a,rule-b", false, "SUB-RULE", "rule-a,rule-b", "", nil},
		{"sub-rule takes the target from the end", "SUB-RULE,rule-a,rule-b,DIRECT", true, "SUB-RULE", "rule-a,rule-b", "DIRECT", nil},
		{"domain regex keeps its commas", `DOMAIN-REGEX,^a.*,b$`, false, "DOMAIN-REGEX", `^a.*,b$`, "", nil},
		{"bare type has no payload", "MATCH", false, "MATCH", "", "", nil},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			tp, payload, target, params := ParseRulePayload(testCase.rule, testCase.needTarget)
			assert.Equal(test, testCase.tp, tp, "type")
			assert.Equal(test, testCase.payload, payload, "payload")
			assert.Equal(test, testCase.target, target, "target")
			if testCase.params == nil {
				assert.Empty(test, params, "params")
			} else {
				assert.Equal(test, testCase.params, params, "params")
			}
		})
	}
}

func TestParseParamsMarksSrcAndNoResolve(test *testing.T) {
	tests := []struct {
		name      string
		params    []string
		isSrc     bool
		noResolve bool
	}{
		{"no params", nil, false, false},
		{"no resolve", []string{"no-resolve"}, false, true},
		{"src implies no resolve", []string{"src"}, true, true},
		{"src and no resolve", []string{"src", "no-resolve"}, true, true},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			isSrc, noResolve := ParseParams(testCase.params)
			assert.Equal(test, testCase.isSrc, isSrc)
			assert.Equal(test, testCase.noResolve, noResolve)
		})
	}
}
