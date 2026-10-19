package provider

import (
	"strings"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/rules/common"
)

type classicalRuleIndex struct {
	segments          []classicalRuleSegment
	segmentOfPosition []int
}

type classicalRuleSegment struct {
	end    int
	exact  map[string]int
	suffix *classicalSuffixNode
}

type classicalSuffixNode struct {
	children  map[string]*classicalSuffixNode
	positions []int
}

func buildClassicalRuleIndex(rules []C.Rule) *classicalRuleIndex {
	if len(rules) == 0 {
		return nil
	}

	index := &classicalRuleIndex{
		segments:          make([]classicalRuleSegment, 0, 8),
		segmentOfPosition: make([]int, len(rules)),
	}
	for position := 0; position < len(rules); {
		_, _, indexable := classicalIndexedRule(rules[position])
		if !indexable {
			for position < len(rules) {
				if _, _, ok := classicalIndexedRule(rules[position]); ok {
					break
				}
				index.segmentOfPosition[position] = len(index.segments)
				position++
			}
			index.segments = append(index.segments, classicalRuleSegment{end: position})
			continue
		}

		segment := classicalRuleSegment{
			exact:  make(map[string]int),
			suffix: &classicalSuffixNode{},
		}
		for position < len(rules) {
			domain, exact, ok := classicalIndexedRule(rules[position])
			if !ok {
				break
			}
			if exact {
				if _, exists := segment.exact[domain]; !exists {
					segment.exact[domain] = position
				}
			} else {
				insertClassicalSuffix(segment.suffix, domain, position)
			}
			index.segmentOfPosition[position] = len(index.segments)
			position++
		}
		segment.end = position
		index.segments = append(index.segments, segment)
	}
	return index
}

func classicalIndexedRule(rule C.Rule) (payload string, exact bool, ok bool) {
	switch typed := rule.(type) {
	case *common.Domain:
		return typed.Payload(), true, true
	case *common.DomainSuffix:
		return typed.Payload(), false, indexableSuffixPayload(typed.Payload())
	default:
		return "", false, false
	}
}

func indexableSuffixPayload(suffix string) bool {
	return suffix != "" && !strings.Contains(suffix, "..") &&
		!strings.HasPrefix(suffix, ".") && !strings.HasSuffix(suffix, ".")
}

func (index *classicalRuleIndex) segment(position int) classicalRuleSegment {
	if index == nil || position < 0 || position >= len(index.segmentOfPosition) {
		return classicalRuleSegment{}
	}
	return index.segments[index.segmentOfPosition[position]]
}

func (index *classicalRuleIndex) next(position int, host string) int {
	segment := index.segment(position)
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

func insertClassicalSuffix(root *classicalSuffixNode, suffix string, position int) {
	node := root
	labels := strings.Split(suffix, ".")
	for labelIndex := len(labels) - 1; labelIndex >= 0; labelIndex-- {
		label := labels[labelIndex]
		child, ok := node.children[label]
		if !ok {
			child = &classicalSuffixNode{}
			if node.children == nil {
				node.children = make(map[string]*classicalSuffixNode)
			}
			node.children[label] = child
		}
		node = child
	}
	node.positions = append(node.positions, position)
}

func (node *classicalSuffixNode) match(host string, from int) int {
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
