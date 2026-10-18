package utils

import (
	"hash/maphash"
	"net/netip"
	"unsafe"
)

var globalSeed = maphash.MakeSeed()

func GlobalID(material string) (id [8]byte) {
	*(*uint64)(unsafe.Pointer(&id[0])) = maphash.String(globalSeed, material)
	return
}

func MapHash(material string) uint64 {
	return maphash.String(globalSeed, material)
}

// MapHashAddrPort hashes an address:port pair without formatting it first
func MapHashAddrPort(addrPort netip.AddrPort) uint64 {
	const (
		fnvOffset64 = 14695981039346656037
		fnvPrime64  = 1099511628211
	)
	hash := uint64(fnvOffset64)
	if addrPort.Addr().Is4() {
		for _, octet := range addrPort.Addr().As4() {
			hash = (hash ^ uint64(octet)) * fnvPrime64
		}
	} else {
		for _, octet := range addrPort.Addr().As16() {
			hash = (hash ^ uint64(octet)) * fnvPrime64
		}
	}
	return (hash ^ uint64(addrPort.Port())) * fnvPrime64
}
