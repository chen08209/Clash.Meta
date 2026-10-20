package socks5

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeUDPPacketIntoMatchesEncodeUDPPacket(t *testing.T) {
	for _, addrStr := range []string{"1.2.3.4:53", "255.255.255.255:65535", "[2001:db8::1]:5353"} {
		addr := netip.MustParseAddrPort(addrStr)
		payload := []byte("payload")

		want, err := EncodeUDPPacket(AddrFromStdAddrPort(addr), payload)
		require.NoError(t, err)

		buf := make([]byte, UDPPacketHeadroom+len(payload))
		got, err := EncodeUDPPacketInto(buf, addr, payload)
		require.NoError(t, err)
		require.Equal(t, want, got, addrStr)
	}
}

func TestEncodeUDPPacketIntoRejectsSmallBuffer(t *testing.T) {
	_, err := EncodeUDPPacketInto(make([]byte, 4), netip.MustParseAddrPort("1.2.3.4:53"), []byte("hello"))
	require.Error(t, err)
}

func TestEncodeUDPPacketIntoTakesAnEmptyPayload(t *testing.T) {
	addr := netip.MustParseAddrPort("1.2.3.4:53")
	buf := make([]byte, UDPPacketHeadroom)

	got, err := EncodeUDPPacketInto(buf, addr, nil)
	require.NoError(t, err)
	want, err := EncodeUDPPacket(AddrFromStdAddrPort(addr), nil)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func BenchmarkEncodeUDPPacketInto(b *testing.B) {
	addr := netip.MustParseAddrPort("1.2.3.4:53")
	payload := []byte("payload")
	buf := make([]byte, UDPPacketHeadroom+len(payload))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := EncodeUDPPacketInto(buf, addr, payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeUDPPacket(b *testing.B) {
	addr := AddrFromStdAddrPort(netip.MustParseAddrPort("1.2.3.4:53"))
	payload := []byte("payload")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := EncodeUDPPacket(addr, payload); err != nil {
			b.Fatal(err)
		}
	}
}
