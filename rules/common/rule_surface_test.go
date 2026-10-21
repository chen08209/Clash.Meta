package common

import (
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchRuleConstructor(test *testing.T) {
	rule := NewMatch("DIRECT")
	assert.Equal(test, C.MATCH, rule.RuleType())
	assert.Equal(test, "DIRECT", rule.Adapter())
	assert.Equal(test, "", rule.Payload())

	matched, adapter := rule.Match(&C.Metadata{}, C.RuleMatchHelper{})
	assert.True(test, matched)
	assert.Equal(test, "DIRECT", adapter)
}

func TestIPCIDRRuleSurface(test *testing.T) {
	rule, err := NewIPCIDR("1.2.3.0/24", "DIRECT")
	require.NoError(test, err)
	assert.Equal(test, C.IPCIDR, rule.RuleType())
	assert.Equal(test, "DIRECT", rule.Adapter())
	assert.Equal(test, "1.2.3.0/24", rule.Payload())
	assert.Equal(test, "1.2.3.0/24", rule.IPPrefix().String())
	assert.False(test, rule.IsSourceIPRule())

	source, err := NewIPCIDR("1.2.3.0/24", "DIRECT", WithIPCIDRSourceIP(true))
	require.NoError(test, err)
	assert.Equal(test, C.SrcIPCIDR, source.RuleType())
	assert.True(test, source.IsSourceIPRule())

	_, err = NewIPCIDR("not-a-prefix", "DIRECT")
	require.Error(test, err)
}
