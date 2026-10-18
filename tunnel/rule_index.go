package tunnel

import (
	"strings"

	C "github.com/metacubex/mihomo/constant"
	rulesCommon "github.com/metacubex/mihomo/rules/common"
)

type ruleIndex struct {
	segments          []ruleSegment
	segmentOfPosition []int
	nextDuplicate     map[int]int
}

type ruleSegment struct {
	end    int
	exact  map[string]int
	suffix *suffixNode
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
	}
	for position := 0; position < len(rules); {
		if _, _, ok := indexedDomainPayload(rules[position]); ok {
			segment := ruleSegment{
				exact:  make(map[string]int),
				suffix: &suffixNode{},
			}
			last := make(map[string]int)
			for position < len(rules) {
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

		for position < len(rules) {
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

func (index *ruleIndex) next(position int, host string) int {
	if index == nil || position >= len(index.segmentOfPosition) {
		return position
	}
	segment := index.segments[index.segmentOfPosition[position]]
	if segment.exact == nil && segment.suffix == nil {
		return position
	}

	candidate := segment.end
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

func (index *ruleIndex) after(position int, host string) int {
	if next, ok := index.nextDuplicate[position]; ok {
		return next
	}
	return index.next(position+1, host)
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
