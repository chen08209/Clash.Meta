package trie_test

import (
	"fmt"
	"net/netip"
	"testing"

	"github.com/metacubex/mihomo/component/trie"
	"github.com/stretchr/testify/assert"
)

var localIP = netip.AddrFrom4([4]byte{127, 0, 0, 1})

func TestTrie_Basic(t *testing.T) {
	tree := trie.New[netip.Addr]()
	domains := []string{
		"example.com",
		"google.com",
		"localhost",
	}

	for _, domain := range domains {
		assert.NoError(t, tree.Insert(domain, localIP))
	}

	node := tree.Search("example.com")
	assert.NotNil(t, node)
	assert.True(t, node.Data() == localIP)
	assert.NotNil(t, tree.Insert("", localIP))
	assert.Nil(t, tree.Search(""))
	assert.NotNil(t, tree.Search("localhost"))
	assert.Nil(t, tree.Search("www.google.com"))
}

func TestTrie_Wildcard(t *testing.T) {
	tree := trie.New[netip.Addr]()
	domains := []string{
		"*.example.com",
		"sub.*.example.com",
		"*.dev",
		".org",
		".example.net",
		".apple.*",
		"+.foo.com",
		"+.stun.*.*",
		"+.stun.*.*.*",
		"+.stun.*.*.*.*",
		"stun.l.google.com",
	}

	for _, domain := range domains {
		assert.NoError(t, tree.Insert(domain, localIP))
	}

	assert.NotNil(t, tree.Search("sub.example.com"))
	assert.NotNil(t, tree.Search("sub.foo.example.com"))
	assert.NotNil(t, tree.Search("test.org"))
	assert.NotNil(t, tree.Search("test.example.net"))
	assert.NotNil(t, tree.Search("test.apple.com"))
	assert.NotNil(t, tree.Search("test.foo.com"))
	assert.NotNil(t, tree.Search("foo.com"))
	assert.NotNil(t, tree.Search("global.stun.website.com"))
	assert.Nil(t, tree.Search("foo.sub.example.com"))
	assert.Nil(t, tree.Search("foo.example.dev"))
	assert.Nil(t, tree.Search("example.com"))
}

func TestTrie_Priority(t *testing.T) {
	tree := trie.New[int]()
	domains := []string{
		".dev",
		"example.dev",
		"*.example.dev",
		"test.example.dev",
	}

	assertFn := func(domain string, data int) {
		node := tree.Search(domain)
		assert.NotNil(t, node)
		assert.Equal(t, data, node.Data())
	}

	for idx, domain := range domains {
		assert.NoError(t, tree.Insert(domain, idx+1))
	}

	assertFn("test.dev", 1)
	assertFn("foo.bar.dev", 1)
	assertFn("example.dev", 2)
	assertFn("foo.example.dev", 3)
	assertFn("test.example.dev", 4)
}

func TestTrie_Boundary(t *testing.T) {
	tree := trie.New[netip.Addr]()
	assert.NoError(t, tree.Insert("*.dev", localIP))

	assert.NotNil(t, tree.Insert(".", localIP))
	assert.NotNil(t, tree.Insert("..dev", localIP))
	assert.Nil(t, tree.Search("dev"))
}

func TestTrie_WildcardBoundary(t *testing.T) {
	tree := trie.New[netip.Addr]()
	assert.NoError(t, tree.Insert("+.*", localIP))
	assert.NoError(t, tree.Insert("stun.*.*.*", localIP))

	assert.NotNil(t, tree.Search("example.com"))
}

func TestTrie_InvalidWildcardPlacement(t *testing.T) {
	// "+" is only valid as a whole first segment ("+.example.com"); "*" is only
	// valid as a whole segment. Anything else must be rejected so that
	// DomainTrie.Search (treats a stray wildcard as a literal label) and
	// DomainSet.Has (treats the wildcard byte as a wildcard) can never disagree.
	valid := []string{"+.example.com", "*.example.com", "+.*", "stun.*.*.*", "*", "a.*", "*.a"}
	t.Run("valid", func(t *testing.T) {
		for _, d := range valid {
			tree := trie.New[netip.Addr]()
			assert.NoErrorf(t, tree.Insert(d, localIP), "should accept %q", d)
		}
	})

	invalid := []struct {
		domain string
		reason string
	}{
		{domain: "stun.+", reason: `"+" wildcard is only allowed in the first label`},
		{domain: "a.+.b", reason: `"+" wildcard is only allowed in the first label`},
		{domain: "a.+", reason: `"+" wildcard is only allowed in the first label`},
		{domain: "+", reason: `"+" wildcard must be followed by another label`},
		{domain: "+.+.com", reason: `"+" wildcard is only allowed in the first label`},
		{domain: "a+b.com", reason: `"+" wildcard must occupy the entire label 1`},
		{domain: "a*b.com", reason: `"*" wildcard must occupy the entire label 1`},
		{domain: "*a.com", reason: `"*" wildcard must occupy the entire label 1`},
		{domain: "a*.com", reason: `"*" wildcard must occupy the entire label 1`},
	}
	t.Run("invalid", func(t *testing.T) {
		for _, testCase := range invalid {
			tree := trie.New[netip.Addr]()
			err := tree.Insert(testCase.domain, localIP)
			assert.ErrorIsf(t, err, trie.ErrInvalidDomain, "should reject %q", testCase.domain)
			assert.ErrorContainsf(t, err, testCase.reason, "should explain why %q was rejected", testCase.domain)
		}
	})

	// Accepted patterns must stay consistent between Search and DomainSet.Has.
	queries := []string{"example.com", "a.example.com", "com", "x.com", "za.com", "anything.at.all"}
	t.Run("consistency", func(t *testing.T) {
		for _, d := range valid {
			tree := trie.New[netip.Addr]()
			assert.NoError(t, tree.Insert(d, localIP))
			setFromTrie := tree.NewDomainSet()
			var builder trie.DomainSetBuilder
			assert.NoError(t, builder.Insert(d))
			setFromBuilder := builder.Build()
			assert.Equal(t, setFromTrie, setFromBuilder)
			for _, q := range queries {
				searchHit := tree.Search(q) != nil
				trieSetHit := setFromTrie != nil && setFromTrie.Has(q)
				builderSetHit := setFromBuilder != nil && setFromBuilder.Has(q)
				assert.Equalf(t, searchHit, trieSetHit, "pattern %q query %q: Search=%v TrieSet=%v", d, q, searchHit, trieSetHit)
				assert.Equalf(t, searchHit, builderSetHit, "pattern %q query %q: Search=%v BuilderSet=%v", d, q, searchHit, builderSetHit)
			}
		}
	})
}

func TestTrie_ValidAndSplitDomain(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		assertValid := func(domain string, expected []string) {
			parts, err := trie.ValidAndSplitDomain(domain)
			assert.NoErrorf(t, err, "should accept %q", domain)
			assert.Equal(t, expected, parts)
		}
		assertValid("GOOGLE.COM", []string{"google", "com"})
		assertValid("Mijia Cloud", []string{"mijia cloud"})
	})

	invalid := []struct {
		domain string
		reason string
	}{
		{domain: "", reason: "domain is empty"},
		{domain: "example.com.", reason: "trailing dot is not allowed"},
		{domain: " example.com", reason: "leading whitespace is not allowed"},
		{domain: "example.com ", reason: "trailing whitespace is not allowed"},
		{domain: "A..COM", reason: "label 2 is empty"},
	}
	t.Run("invalid", func(t *testing.T) {
		for _, testCase := range invalid {
			parts, err := trie.ValidAndSplitDomain(testCase.domain)
			assert.Nil(t, parts)
			assert.ErrorIs(t, err, trie.ErrInvalidDomain)
			assert.ErrorContains(t, err, testCase.reason)
			assert.ErrorContains(t, err, fmt.Sprintf("%q", testCase.domain))
		}
	})
}

func TestDomainTrieSearchLowercaseQueryDoesNotAllocate(t *testing.T) {
	tree := trie.New[int]()
	assert.NoError(t, tree.Insert("benchmark.example.com", 1))

	allocations := testing.AllocsPerRun(
		100,
		func() {
			assert.NotNil(t, tree.Search("benchmark.example.com"))
		},
	)
	assert.EqualValues(t, 0, allocations)
}

func TestTrie_Foreach(t *testing.T) {
	tree := trie.New[int]()
	domainList := []string{
		"google.com",
		"stun.*.*.*",
		"test.*.google.com",
		"+.baidu.com",
		"*.baidu.com",
		"*.*.baidu.com",
		"baidu.com",
		".example.org",
	}
	for index, domain := range domainList {
		assert.NoError(t, tree.Insert(domain, index))
	}
	rebuilt := trie.New[int]()
	values := make(map[string]int)
	tree.Foreach(func(domain string, data int) bool {
		assert.NotContains(t, values, domain)
		values[domain] = data
		assert.NoError(t, rebuilt.Insert(domain, data))
		return true
	})
	assert.Equal(t, map[string]int{
		"google.com":        0,
		"stun.*.*.*":        1,
		"test.*.google.com": 2,
		".baidu.com":        3,
		"*.baidu.com":       4,
		"*.*.baidu.com":     5,
		"baidu.com":         6,
		".example.org":      7,
	}, values)
	for _, tree := range []*trie.DomainTrie[int]{tree, rebuilt} {
		for domain, expected := range map[string]int{"google.com": 0, "baidu.com": 6, "a.b.c.baidu.com": 3, "www.example.org": 7} {
			node := tree.Search(domain)
			if assert.NotNil(t, node, domain) {
				assert.Equal(t, expected, node.Data(), domain)
			}
		}
		assert.Nil(t, tree.Search("example.org"))
	}
}
