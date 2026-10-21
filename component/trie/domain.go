package trie

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	wildcard        = "*"
	dotWildcard     = ""
	complexWildcard = "+"
	domainStep      = "."
)

// ErrInvalidDomain means a domain pattern is invalid.
var ErrInvalidDomain = errors.New("invalid domain")

// DomainTrie contains the main logic for adding and searching nodes for domain segments.
// support wildcard domain (e.g *.google.com)
type DomainTrie[T any] struct {
	root *Node[T]
}

// ValidAndSplitDomain lower-cases and splits a domain into its dot-separated
// parts, reporting whether it is a well-formed pattern. It returns an error for
// a trailing dot ("a.com."), leading/trailing whitespace, or an empty segment,
// as well as for misplaced wildcards (see below). The error describes why the
// invalid Clash-style domain pattern was rejected.
func ValidAndSplitDomain(domain string) ([]string, error) {
	normalized, err := normalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	return strings.Split(normalized, domainStep), nil
}

func normalizeDomain(domain string) (string, error) {
	originalDomain := domain
	if domain == "" {
		return "", invalidDomainError(originalDomain, "domain is empty")
	}
	if domain[len(domain)-1] == '.' {
		return "", invalidDomainError(originalDomain, "trailing dot is not allowed")
	}
	if r, _ := utf8.DecodeRuneInString(domain); unicode.IsSpace(r) {
		return "", invalidDomainError(originalDomain, "leading whitespace is not allowed")
	}
	if r, _ := utf8.DecodeLastRuneInString(domain); unicode.IsSpace(r) {
		return "", invalidDomainError(originalDomain, "trailing whitespace is not allowed")
	}

	normalized := strings.ToLower(domain)
	labelCount := 0
	labelStart := 0
	for index := 0; index <= len(normalized); index++ {
		if index < len(normalized) && normalized[index] != '.' {
			continue
		}
		label := normalized[labelStart:index]
		labelNumber := labelCount + 1
		if label == "" && labelCount > 0 {
			return "", invalidDomainError(
				originalDomain,
				fmt.Sprintf("label %d is empty", labelNumber),
			)
		}
		if strings.Contains(label, complexWildcard) {
			switch {
			case label != complexWildcard:
				return "", invalidDomainError(
					originalDomain,
					fmt.Sprintf(
						"%q wildcard must occupy the entire label %d",
						complexWildcard,
						labelNumber,
					),
				)
			case labelCount != 0:
				return "", invalidDomainError(
					originalDomain,
					fmt.Sprintf(
						"%q wildcard is only allowed in the first label",
						complexWildcard,
					),
				)
			}
		}
		if strings.Contains(label, wildcard) && label != wildcard {
			return "", invalidDomainError(
				originalDomain,
				fmt.Sprintf("%q wildcard must occupy the entire label %d", wildcard, labelNumber),
			)
		}
		labelCount++
		labelStart = index + 1
	}
	if normalized == complexWildcard {
		return "", invalidDomainError(
			originalDomain,
			fmt.Sprintf("%q wildcard must be followed by another label", complexWildcard),
		)
	}
	return normalized, nil
}

func invalidDomainError(domain, reason string) error {
	return fmt.Errorf("%w %q: %s", ErrInvalidDomain, domain, reason)
}

// Insert adds a node to the trie.
// Support
// 1. www.example.com
// 2. *.example.com
// 3. subdomain.*.example.com
// 4. .example.com
// 5. +.example.com
func (t *DomainTrie[T]) Insert(domain string, data T) error {
	parts, err := ValidAndSplitDomain(domain)
	if err != nil {
		return err
	}

	if parts[0] == complexWildcard {
		t.insert(parts[1:], data)
		parts[0] = dotWildcard
		t.insert(parts, data)
	} else {
		t.insert(parts, data)
	}

	return nil
}

func (t *DomainTrie[T]) insert(parts []string, data T) {
	node := t.root
	// reverse storage domain part to save space
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		node = node.getOrNewChild(part)
	}

	node.setData(data)
}

// Search is the most important part of the Trie.
// Priority as:
// 1. static part
// 2. wildcard domain
// 2. dot wildcard domain
func (t *DomainTrie[T]) Search(domain string) *Node[T] {
	domainEnd := len(domain)
	if !isCanonicalSearchDomain(domain) {
		normalized, err := normalizeDomain(domain)
		if err != nil || normalized[0] == '.' {
			return nil
		}
		domain = normalized
		domainEnd = len(normalized)
	}

	n := t.searchFrom(t.root, domain, domainEnd)

	if n.isEmpty() {
		return nil
	}

	return n
}

func isCanonicalSearchDomain(domain string) bool {
	if domain == "" || domain[0] == '.' || domain[len(domain)-1] == '.' {
		return false
	}

	dot := false
	for index := 0; index < len(domain); index++ {
		switch c := domain[index]; {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
			dot = false
		case c == '.':
			if dot {
				return false
			}
			dot = true
		default:
			return false
		}
	}
	return true
}

func (t *DomainTrie[T]) searchFrom(
	node *Node[T],
	domain string,
	end int,
) *Node[T] {
	if end == 0 {
		return node
	}
	dot := strings.LastIndexByte(domain[:end], '.')
	label := domain[dot+1 : end]
	nextEnd := dot
	if nextEnd < 0 {
		nextEnd = 0
	}

	if child := node.getChild(label); child != nil {
		if match := t.searchFrom(child, domain, nextEnd); !match.isEmpty() {
			return match
		}
	}
	if child := node.getChild(wildcard); child != nil {
		if match := t.searchFrom(child, domain, nextEnd); !match.isEmpty() {
			return match
		}
	}
	return node.getChild(dotWildcard)
}

func (t *DomainTrie[T]) search(node *Node[T], parts []string) *Node[T] {
	if len(parts) == 0 {
		return node
	}

	if c := node.getChild(parts[len(parts)-1]); c != nil {
		if n := t.search(c, parts[:len(parts)-1]); !n.isEmpty() {
			return n
		}
	}

	if c := node.getChild(wildcard); c != nil {
		if n := t.search(c, parts[:len(parts)-1]); !n.isEmpty() {
			return n
		}
	}

	return node.getChild(dotWildcard)
}

func (t *DomainTrie[T]) Optimize() {
	t.root.optimize()
}

// Foreach iterates over the stored domain patterns and their associated values
// in unspecified order. Patterns use lowercase labels, "*" for a single-label
// wildcard, and a leading "." for subdomain-only matching. Exact and suffix-only
// patterns are separate entries; "+." shorthand is not emitted. Each pattern
// and its value can be passed to Insert independently to reproduce the mappings.
func (t *DomainTrie[T]) Foreach(fn func(domain string, data T) bool) {
	for key, data := range t.root.getChildren() {
		recursion([]string{key}, data, fn)
		if !data.isEmpty() {
			if !fn(joinDomain([]string{key}), data.data) {
				return
			}
		}
	}
}

func (t *DomainTrie[T]) IsEmpty() bool {
	if t == nil || t.root == nil {
		return true
	}
	return len(t.root.getChildren()) == 0
}

func recursion[T any](items []string, node *Node[T], fn func(domain string, data T) bool) bool {
	for key, data := range node.getChildren() {
		newItems := append([]string{key}, items...)
		if !data.isEmpty() {
			domain := joinDomain(newItems)
			if !fn(domain, data.Data()) {
				return false
			}
		}
		if !recursion(newItems, data, fn) {
			return false
		}
	}
	return true
}

func joinDomain(items []string) string {
	return strings.Join(items, domainStep)
}

// New returns a new, empty Trie.
func New[T any]() *DomainTrie[T] {
	return &DomainTrie[T]{root: newNode[T]()}
}
