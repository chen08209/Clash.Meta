//go:build go1.24

package maphash

import "hash/maphash"

func Comparable[T comparable](seed Seed, v T) uint64 {
	return maphash.Comparable(seed, v)
}

func WriteComparable[T comparable](h *Hash, x T) {
	maphash.WriteComparable(h, x)
}

const testComparableAllocations = true

// ComparableAllocationFree reports whether Comparable can run without
// allocating; the stdlib implementation never allocates.
const ComparableAllocationFree = true
