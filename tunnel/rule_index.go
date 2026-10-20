package tunnel

import (
	"net/netip"
	"strings"

	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
)

type ruleIndex struct {
	segments          []ruleSegment
	segmentOfPosition []int
	nextDuplicate     map[int]int
	usesHelper        []bool
}

type ruleSegment struct {
	end    int
	exact  map[string]int
	suffix *suffixNode
	ipDst  *ipRuleTrie
	ipSrc  *ipRuleTrie
}

type suffixNode struct {
	children  map[string]*suffixNode
	positions []int
}

func buildRuleIndex(rules []C.Rule) *ruleIndex {
	if len(rules) == 0 {
		return nil
	}

	index := &ruleIndex{
		segments:          make([]ruleSegment, 0, 8),
		segmentOfPosition: make([]int, len(rules)),
		nextDuplicate:     make(map[int]int),
		usesHelper:        make([]bool, len(rules)),
	}
	for position := 0; position < len(rules); {
		index.usesHelper[position] = ruleUsesMatchHelper(rules[position])
		if _, _, ok := indexedDomainPayload(rules[position]); ok {
			segment := ruleSegment{
				exact:  make(map[string]int),
				suffix: &suffixNode{},
			}
			last := make(map[string]int)
			for position < len(rules) {
				index.usesHelper[position] = ruleUsesMatchHelper(rules[position])
				domain, exact, ok := indexedDomainPayload(rules[position])
				if !ok {
					break
				}
				if exact {
					_, exists := segment.exact[domain]
					if exists {
						index.nextDuplicate[last[domain]] = position
						last[domain] = position
					} else {
						segment.exact[domain] = position
						last[domain] = position
					}
				} else {
					insertSuffix(segment.suffix, domain, position)
				}
				index.segmentOfPosition[position] = len(index.segments)
				position++
			}
			segment.end = position
			index.segments = append(index.segments, segment)
			continue
		}

		if _, _, ok := indexedIPPayload(rules[position]); ok {
			segment := ruleSegment{}
			for position < len(rules) {
				index.usesHelper[position] = ruleUsesMatchHelper(rules[position])
				ipPrefix, ipSource, ok := indexedIPPayload(rules[position])
				if !ok {
					break
				}
				if ipSource {
					if segment.ipSrc == nil {
						segment.ipSrc = &ipRuleTrie{}
					}
					segment.ipSrc.insert(ipPrefix, position)
				} else {
					if segment.ipDst == nil {
						segment.ipDst = &ipRuleTrie{}
					}
					segment.ipDst.insert(ipPrefix, position)
				}
				index.segmentOfPosition[position] = len(index.segments)
				position++
			}
			segment.end = position
			index.segments = append(index.segments, segment)
			continue
		}

		for position < len(rules) {
			index.usesHelper[position] = ruleUsesMatchHelper(rules[position])
			if _, _, ok := indexedDomainPayload(rules[position]); ok {
				break
			}
			index.segmentOfPosition[position] = len(index.segments)
			position++
		}
		index.segments = append(index.segments, ruleSegment{end: position})
	}
	return index
}

func (index *ruleIndex) next(position int, metadata *C.Metadata) int {
	if index == nil || position >= len(index.segmentOfPosition) {
		return position
	}
	segment := index.segments[index.segmentOfPosition[position]]
	if segment.ipDst != nil || segment.ipSrc != nil {
		return index.nextIPRule(position, segment, metadata)
	}
	if segment.exact == nil && segment.suffix == nil {
		return position
	}

	candidate := segment.end
	host := metadata.RuleHost()
	if hit, ok := segment.exact[host]; ok && hit >= position && hit < candidate {
		candidate = hit
	}
	if segment.suffix != nil {
		if hit := segment.suffix.match(host, position); hit >= 0 && hit < candidate {
			candidate = hit
		}
	}
	return candidate
}

// nextIPRule jumps to the first rule of the run whose prefix contains an
// address the packet already carries. A run with no match is skipped whole:
// the rules it skips would not have matched, and any later rule that needs a
// resolved address still triggers the lookup through its own match helper.
func (index *ruleIndex) nextIPRule(position int, segment ruleSegment, metadata *C.Metadata) int {
	candidate := -1
	if segment.ipDst != nil && metadata.DstIP.IsValid() {
		if hit := segment.ipDst.lookup(metadata.DstIP, position); hit >= 0 && (candidate < 0 || hit < candidate) {
			candidate = hit
		}
	}
	if segment.ipSrc != nil && metadata.SrcIP.IsValid() {
		if hit := segment.ipSrc.lookup(metadata.SrcIP, position); hit >= 0 && (candidate < 0 || hit < candidate) {
			candidate = hit
		}
	}
	if candidate < 0 {
		if !metadata.DstIP.IsValid() && !metadata.SrcIP.IsValid() {
			// neither address is known: the run must still run its
			// helper-needing rules so they can resolve lazily
			return position
		}
		return segment.end
	}
	return candidate
}

// usesHelperAt reports whether the rule at position needs the match helper.
// The flag is precomputed at index build time so the per-connection scan stops
// unwrapping wrappers to find out.
func (index *ruleIndex) usesHelperAt(position int, rules []C.Rule) bool {
	if index != nil && position < len(index.usesHelper) {
		return index.usesHelper[position]
	}
	return ruleUsesMatchHelper(rules[position])
}

func (index *ruleIndex) after(position int, metadata *C.Metadata) int {
	if next, ok := index.nextDuplicate[position]; ok {
		return next
	}
	return index.next(position+1, metadata)
}

func indexedIPPayload(rule C.Rule) (netip.Prefix, bool, bool) {
	for rule != nil {
		switch typed := rule.(type) {
		case *rulesCommon.IPCIDR:
			return typed.IPPrefix(), typed.IsSourceIPRule(), true
		case interface{ Unwrap() C.Rule }:
			rule = typed.Unwrap()
		default:
			return netip.Prefix{}, false, false
		}
	}
	return netip.Prefix{}, false, false
}

func buildSubRuleIndexes(source map[string][]C.Rule) map[string]*ruleIndex {
	if source == nil {
		return nil
	}
	result := make(map[string]*ruleIndex, len(source))
	for name, rules := range source {
		result[name] = buildRuleIndex(rules)
	}
	return result
}

func indexedDomainPayload(rule C.Rule) (string, bool, bool) {
	for rule != nil {
		switch typed := rule.(type) {
		case *rulesCommon.Domain:
			return typed.Payload(), true, true
		case *rulesCommon.DomainSuffix:
			return typed.Payload(), false, true
		case interface{ Unwrap() C.Rule }:
			rule = typed.Unwrap()
		default:
			return "", false, false
		}
	}
	return "", false, false
}

func insertSuffix(root *suffixNode, suffix string, position int) {
	node := root
	labels := strings.Split(suffix, ".")
	for index := len(labels) - 1; index >= 0; index-- {
		label := labels[index]
		if node.children == nil {
			node.children = make(map[string]*suffixNode)
		}
		child, ok := node.children[label]
		if !ok {
			child = &suffixNode{}
			node.children[label] = child
		}
		node = child
	}
	node.positions = append(node.positions, position)
}

func (node *suffixNode) match(host string, from int) int {
	hit := -1
	current := node
	for end := len(host); end > 0; {
		start := end - 1
		for start >= 0 && host[start] != '.' {
			start--
		}
		child, ok := current.children[host[start+1:end]]
		if !ok {
			break
		}
		current = child
		if position := positionAtOrAfter(current.positions, from); position >= 0 &&
			(hit < 0 || position < hit) {
			hit = position
		}
		end = start
	}
	return hit
}

func positionAtOrAfter(positions []int, target int) int {
	left, right := 0, len(positions)
	for left < right {
		middle := int(uint(left+right) >> 1)
		if positions[middle] < target {
			left = middle + 1
		} else {
			right = middle
		}
	}
	if left == len(positions) {
		return -1
	}
	return positions[left]
}
