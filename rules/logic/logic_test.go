package logic

import (
	"fmt"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/rules/common"
	"github.com/stretchr/testify/require"
)

type stubRule struct {
	match     func(*C.Metadata, C.RuleMatchHelper) (bool, string)
	ruleType  C.RuleType
	payload   string
	adapter   string
	providers []string
}

type passRuleContext struct{}

func (passRuleContext) ResolveIP() {}

func (passRuleContext) FindProcess() {}

func (passRuleContext) CheckPassRule(adapter string) bool { return adapter == "PASSED" }

func (r *stubRule) RuleType() C.RuleType { return r.ruleType }

func (r *stubRule) Match(metadata *C.Metadata, helper C.RuleMatchHelper) (bool, string) {
	return r.match(metadata, helper)
}

func (r *stubRule) Adapter() string         { return r.adapter }
func (r *stubRule) Payload() string         { return r.payload }
func (r *stubRule) ProviderNames() []string { return r.providers }

func testParseRule(tp, payload, target string, _ []string, _ map[string][]C.Rule) (C.Rule, error) {
	switch tp {
	case "DOMAIN":
		return common.NewDomain(payload, target), nil
	case "DST-PORT":
		return common.NewPort(payload, target, C.DstPort)
	case "TRUE":
		return &stubRule{match: func(*C.Metadata, C.RuleMatchHelper) (bool, string) {
			return true, target
		}, ruleType: C.Domain, payload: payload, adapter: target}, nil
	case "FALSE":
		return &stubRule{match: func(*C.Metadata, C.RuleMatchHelper) (bool, string) {
			return false, target
		}, ruleType: C.Domain, payload: payload, adapter: target}, nil
	default:
		return nil, fmt.Errorf("unsupported test rule type [%s]", tp)
	}
}

func TestANDSemantics(t *testing.T) {
	and, err := NewAND("((DOMAIN,example.com),(DST-PORT,443))", "DIRECT", testParseRule)
	require.NoError(t, err)
	require.Equal(t, C.AND, and.RuleType())

	matched, adapter := and.Match(&C.Metadata{Host: "example.com", DstPort: 443}, C.RuleMatchHelper{})
	require.True(t, matched)
	require.Equal(t, "DIRECT", adapter)

	matched, _ = and.Match(&C.Metadata{Host: "example.com", DstPort: 8443}, C.RuleMatchHelper{})
	require.False(t, matched)
	matched, _ = and.Match(&C.Metadata{Host: "other.com", DstPort: 443}, C.RuleMatchHelper{})
	require.False(t, matched)

	// The rendered payload uses the rule type's String(), which is CamelCase
	// rather than the config spelling.
	require.Equal(t, "((Domain,example.com) && (DstPort,443))", and.Payload())
}

func TestORSemantics(t *testing.T) {
	or, err := NewOR("((DOMAIN,example.com),(DST-PORT,443))", "PROXY", testParseRule)
	require.NoError(t, err)
	require.Equal(t, C.OR, or.RuleType())

	matched, adapter := or.Match(&C.Metadata{Host: "example.com", DstPort: 1}, C.RuleMatchHelper{})
	require.True(t, matched)
	require.Equal(t, "PROXY", adapter)

	matched, _ = or.Match(&C.Metadata{Host: "other.com", DstPort: 443}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = or.Match(&C.Metadata{Host: "other.com", DstPort: 1}, C.RuleMatchHelper{})
	require.False(t, matched)

	require.Equal(t, "((Domain,example.com) || (DstPort,443))", or.Payload())
}

func TestNOTSemantics(t *testing.T) {
	not, err := NewNOT("((DOMAIN,cn))", "PROXY", testParseRule)
	require.NoError(t, err)
	require.Equal(t, C.NOT, not.RuleType())

	matched, adapter := not.Match(&C.Metadata{Host: "google.com"}, C.RuleMatchHelper{})
	require.True(t, matched)
	require.Equal(t, "PROXY", adapter)
	matched, _ = not.Match(&C.Metadata{Host: "cn"}, C.RuleMatchHelper{})
	require.False(t, matched)

	// The payload is rendered lazily through the once guard.
	first := not.Payload()
	require.Equal(t, "(!(Domain,cn))", first)
	require.Equal(t, first, not.Payload())
}

func TestSubRuleMatchAndRecursion(t *testing.T) {
	// The sub-rule payload is a rule expression; the adapter names the
	// sub-rule table to walk when the condition matches.
	inner, err := NewSubRule("(TRUE,enter)", "inner-name", map[string][]C.Rule{
		"inner-name": {
			&stubRule{match: func(*C.Metadata, C.RuleMatchHelper) (bool, string) {
				return true, "next-name"
			}, ruleType: C.SubRules, adapter: "next-name", payload: "SUB-RULE"},
		},
		"next-name": {
			&stubRule{match: func(*C.Metadata, C.RuleMatchHelper) (bool, string) {
				return true, "DIRECT"
			}, ruleType: C.Domain, payload: "always", adapter: "DIRECT"},
		},
	}, testParseRule)
	require.NoError(t, err)

	matched, adapter := inner.Match(&C.Metadata{Host: "example.com"}, C.RuleMatchHelper{})
	require.True(t, matched)
	require.Equal(t, "DIRECT", adapter, "a sub-rule table recurses through its entries")
}

func TestSubRuleCheckPassRuleHelper(t *testing.T) {
	pass := &stubRule{match: func(*C.Metadata, C.RuleMatchHelper) (bool, string) {
		return true, "PASSED"
	}, ruleType: C.Domain, payload: "pass", adapter: "DIRECT"}
	final := &stubRule{match: func(*C.Metadata, C.RuleMatchHelper) (bool, string) {
		return true, "DIRECT"
	}, ruleType: C.Domain, payload: "final", adapter: "DIRECT"}

	sub, err := NewSubRule("(TRUE,pass)", "inner-name", map[string][]C.Rule{
		"inner-name": {pass, final},
	}, testParseRule)
	require.NoError(t, err)

	// Without the helper the PASSED adapter wins; with a helper that marks it
	// a pass rule the sub rule continues to the next entry.
	matched, adapter := sub.Match(&C.Metadata{}, C.RuleMatchHelper{})
	require.True(t, matched)
	require.Equal(t, "PASSED", adapter)

	helper := C.RuleMatchHelper{Context: passRuleContext{}}
	matched, adapter = sub.Match(&C.Metadata{}, helper)
	require.True(t, matched)
	require.Equal(t, "DIRECT", adapter)
}

func TestProviderNamesAggregate(t *testing.T) {
	and, err := NewAND("((DOMAIN,example.com),(DST-PORT,443))", "DIRECT", testParseRule)
	require.NoError(t, err)
	require.Empty(t, and.ProviderNames())
}

func TestLogicParseErrors(t *testing.T) {
	_, err := NewAND("((DOMAIN,example.com)", "DIRECT", testParseRule)
	require.ErrorContains(t, err, "missing )")

	_, err = NewAND("DOMAIN,example.com)", "DIRECT", testParseRule)
	require.ErrorContains(t, err, "payload format error")

	_, err = NewOR("(TRUE,(FALSE,b)", "DIRECT", testParseRule)
	require.ErrorContains(t, err, "missing )")

	_, err = NewAND("((MATCH,DIRECT))", "DIRECT", testParseRule)
	require.ErrorContains(t, err, "unsupported rule type")

	_, err = NewNOT("((TRUE,a),(TRUE,b))", "DIRECT", testParseRule)
	require.ErrorContains(t, err, "not rule must contain one rule")

	_, err = NewSubRule("SUB", "DIRECT", map[string][]C.Rule{}, testParseRule)
	require.Error(t, err)

	// An empty inner group surfaces as an empty sub payload; the double-paren
	// form errors, while the bare outer-paren form parses to zero rules and
	// the matcher degenerates to always-false for OR.
	_, err = NewAND("(())", "DIRECT", testParseRule)
	require.ErrorContains(t, err, "format is error")

	emptyOr, err := NewOR("()", "DIRECT", testParseRule)
	require.NoError(t, err)
	matched, _ := emptyOr.Match(&C.Metadata{}, C.RuleMatchHelper{})
	require.False(t, matched)
}
