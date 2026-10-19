package common

import (
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func TestDomainSuffixMatch(t *testing.T) {
	rule := NewDomainSuffix("EXAMPLE.com", "DIRECT")
	require.Equal(t, "example.com", rule.Payload())

	matched, _ := rule.Match(&C.Metadata{Host: "example.com"}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = rule.Match(&C.Metadata{Host: "www.example.com"}, C.RuleMatchHelper{})
	require.True(t, matched)
	matched, _ = rule.Match(&C.Metadata{Host: "example.org"}, C.RuleMatchHelper{})
	require.False(t, matched)
}

func BenchmarkDomainSuffixMatch(b *testing.B) {
	rule := NewDomainSuffix("example.com", "DIRECT")
	metadata := &C.Metadata{Host: "www.example.com"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if matched, _ := rule.Match(metadata, C.RuleMatchHelper{}); !matched {
			b.Fatal("suffix did not match")
		}
	}
}
