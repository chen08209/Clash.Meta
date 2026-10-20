package process

import (
	"errors"
	"net/netip"
	"time"

	"github.com/metacubex/mihomo/common/lru"

	C "github.com/metacubex/mihomo/constant"
)

var (
	ErrInvalidNetwork     = errors.New("invalid network")
	ErrPlatformNotSupport = errors.New("not support on this platform")
	ErrNotFound           = errors.New("process not found")
)

const (
	TCP = "tcp"
	UDP = "udp"
)

func FindProcessName(network string, srcIP netip.Addr, srcPort int) (uint32, string, error) {
	return lookupProcessName(processCache, processCacheTTL, findProcessName, network, srcIP, srcPort)
}

// A burst of connections from one app resolves to the same socket row over and
// over, and every miss costs a full system connection-table scan plus a process
// handle query. The cache keys by the local socket and expires quickly, so a
// rebound port is only stale for the TTL window.
const (
	processCacheTTL        = 2 * time.Second
	processCacheMaxEntries = 4096
)

type processCacheKey struct {
	network string
	ip      netip.Addr
	port    uint16
}

type processCacheEntry struct {
	uid  uint32
	path string
	at   time.Time
}

var processCache = lru.New(lru.WithSize[processCacheKey, processCacheEntry](processCacheMaxEntries))

func lookupProcessName(
	cache *lru.LruCache[processCacheKey, processCacheEntry],
	ttl time.Duration,
	lookup func(network string, srcIP netip.Addr, srcPort int) (uint32, string, error),
	network string,
	srcIP netip.Addr,
	srcPort int,
) (uint32, string, error) {
	key := processCacheKey{network: network, ip: srcIP.Unmap(), port: uint16(srcPort)}
	if entry, ok := cache.Get(key); ok && time.Since(entry.at) < ttl {
		return entry.uid, entry.path, nil
	}

	uid, path, err := lookup(network, srcIP, srcPort)
	if err == nil {
		cache.Set(key, processCacheEntry{uid: uid, path: path, at: time.Now()})
	}
	return uid, path, err
}

type PackageNameResolver func(metadata *C.Metadata) (string, error)

// DefaultPackageNameResolver
// never change type traits because it's used in CFMA
var DefaultPackageNameResolver PackageNameResolver

func FindPackageName(metadata *C.Metadata) (string, error) {
	if resolver := DefaultPackageNameResolver; resolver != nil {
		return resolver(metadata)
	}
	return "", ErrPlatformNotSupport
}
