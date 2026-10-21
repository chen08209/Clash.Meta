package process

import (
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindProcessModeText(t *testing.T) {
	tests := []struct {
		mode    FindProcessMode
		text    string
		aliases []string
	}{
		{FindProcessStrict, "strict", []string{"STRICT", "Strict"}},
		{FindProcessAlways, "always", []string{"ALWAYS"}},
		{FindProcessOff, "off", []string{"OFF"}},
	}
	for _, testCase := range tests {
		text, err := testCase.mode.MarshalText()
		require.NoError(t, err)
		assert.Equal(t, testCase.text, string(text))
		assert.Equal(t, testCase.text, testCase.mode.String())

		for _, alias := range append([]string{testCase.text}, testCase.aliases...) {
			var mode FindProcessMode
			require.NoError(t, mode.Set(alias), alias)
			assert.Equal(t, testCase.mode, mode, alias)
			require.NoError(t, mode.UnmarshalText([]byte(alias)), alias)
			assert.Equal(t, testCase.mode, mode, alias)
		}
	}

	var mode FindProcessMode
	require.Error(t, mode.Set("bogus"))
	require.Error(t, mode.UnmarshalText([]byte("")))
}

func TestFindProcessNameRejectsAnInvalidNetwork(t *testing.T) {
	// the platform finders validate the network before they touch the
	// connection table, so this never reads the host's tables
	_, _, err := FindProcessName("bogus", netip.MustParseAddr("127.0.0.1"), 12345)
	require.ErrorIs(t, err, ErrInvalidNetwork)

	// a failure is not cached, so the next attempt asks again
	_, _, err = FindProcessName("bogus", netip.MustParseAddr("127.0.0.1"), 12345)
	require.ErrorIs(t, err, ErrInvalidNetwork)
}

func TestFindPackageNameResolver(t *testing.T) {
	original := DefaultPackageNameResolver
	t.Cleanup(func() { DefaultPackageNameResolver = original })

	DefaultPackageNameResolver = nil
	_, err := FindPackageName(&C.Metadata{})
	require.ErrorIs(t, err, ErrPlatformNotSupport)

	DefaultPackageNameResolver = func(metadata *C.Metadata) (string, error) {
		return "com.example.app", nil
	}
	name, err := FindPackageName(&C.Metadata{})
	require.NoError(t, err)
	assert.Equal(t, "com.example.app", name)

	sentinel := assert.AnError
	DefaultPackageNameResolver = func(metadata *C.Metadata) (string, error) {
		return "", sentinel
	}
	_, err = FindPackageName(&C.Metadata{})
	require.ErrorIs(t, err, sentinel)
}
