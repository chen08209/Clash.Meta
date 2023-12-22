//go:build !android

package dns

const SystemDNSPlaceholder = "system"

func UpdateIsolateHandler(resolver *Resolver, mapper *ResolverEnhancer) {
}
