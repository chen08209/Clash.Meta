package v4

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/require"
)

func TestAddressAddrPortRoundTrip(t *testing.T) {
	for _, addrStr := range []string{"1.2.3.4:5353", "[2001:db8::1]:5353"} {
		want := netip.MustParseAddrPort(addrStr)
		require.Equal(t, want, NewAddressAddrPort(want).AddrPort(), addrStr)
	}
}

func TestAddressAddrPortRejectsADomainAddress(t *testing.T) {
	domain := NewAddress(&C.Metadata{Host: "example.com", DstPort: 5353})
	require.False(t, domain.AddrPort().IsValid())
}
