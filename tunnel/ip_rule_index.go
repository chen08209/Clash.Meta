package tunnel

import (
	"net/netip"
)

// ipRuleTrie indexes consecutive IPCIDR rules by their prefix bits, so a
// packet with a known address finds the first matching rule of the run
// without walking it. IPv4 and IPv6 live in separate tries: a mapped v4
// address is looked up unmapped, which matches the Contains semantics of the
// rules it replaces.
type ipRuleTrie struct {
	v4 *ipRuleNode
	v6 *ipRuleNode
}

type ipRuleNode struct {
	children  [2]*ipRuleNode
	positions []int
}

func (t *ipRuleTrie) insert(prefix netip.Prefix, position int) {
	addr := prefix.Addr().Unmap()
	if addr.Is4() {
		a4 := addr.As4()
		t.v4 = insertIPRule(t.v4, a4[:], prefix.Bits(), position)
	} else {
		a16 := addr.As16()
		t.v6 = insertIPRule(t.v6, a16[:], prefix.Bits(), position)
	}
}

func insertIPRule(root *ipRuleNode, bits []byte, prefixLen int, position int) *ipRuleNode {
	if root == nil {
		root = &ipRuleNode{}
	}
	node := root
	for bit := 0; bit < prefixLen; bit++ {
		value := (bits[bit>>3] >> (7 - uint(bit)&7)) & 1
		if node.children[value] == nil {
			node.children[value] = &ipRuleNode{}
		}
		node = node.children[value]
	}
	node.positions = append(node.positions, position)
	return root
}

// lookup returns the first position at or after from whose prefix contains
// the address, or -1 when no rule of the run matches.
func (t *ipRuleTrie) lookup(ip netip.Addr, from int) int {
	unmapped := ip.Unmap()
	if unmapped.Is4() {
		a4 := unmapped.As4()
		return t.v4.lookup(a4[:], 32, from)
	}
	a16 := unmapped.As16()
	return t.v6.lookup(a16[:], 128, from)
}

func (n *ipRuleNode) lookup(bits []byte, bitLen int, from int) int {
	if n == nil {
		return -1
	}
	best := -1
	if hit := positionAtOrAfter(n.positions, from); hit >= 0 {
		best = hit
	}
	node := n
	for bit := 0; bit < bitLen; bit++ {
		value := (bits[bit>>3] >> (7 - uint(bit)&7)) & 1
		node = node.children[value]
		if node == nil {
			break
		}
		if hit := positionAtOrAfter(node.positions, from); hit >= 0 && (best < 0 || hit < best) {
			best = hit
		}
	}
	return best
}
