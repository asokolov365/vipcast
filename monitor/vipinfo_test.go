package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseVIP_NoError(t *testing.T) {
	t.Parallel()

	f := func(addr, expected string) {
		t.Helper()

		vip, err := ParseVIP(addr)
		require.NoError(t, err)
		require.Equal(t, expected, vip)
	}
	f("1.2.3.4", "1.2.3.4/32")
	f("1.2.3.4/32", "1.2.3.4/32")
	f("::1", "::1/128")
	f("::1/128", "::1/128")
}

func TestParseVIP_Error(t *testing.T) {
	t.Parallel()

	f := func(addr string) {
		t.Helper()

		vip, err := ParseVIP(addr)
		require.Error(t, err)
		require.Equal(t, "", vip)
	}
	f("1.2.3.4/24")
	f("1.2.3/32")
	f("::1/64")
	f("foo")
	f("foo/32")
	f("/foo/bar/32")
	f("")
}

func TestParseBgpCommunities_NoError(t *testing.T) {
	t.Parallel()

	f := func(bgpCommString string, expected []string) {
		t.Helper()

		comms, err := ParseBgpCommunities(bgpCommString)
		require.NoError(t, err)
		require.Equal(t, expected, comms)
	}
	f("0", []string{"0"})
	f("4294967295", []string{"4294967295"})
	f("22697:10001", []string{"22697:10001"})
	f("22697:10001,22697:10002", []string{"22697:10001", "22697:10002"})
}

func TestParseBgpCommunities_Error(t *testing.T) {
	t.Parallel()

	f := func(bgpCommString string) {
		t.Helper()

		comms, err := ParseBgpCommunities(bgpCommString)
		require.Error(t, err)
		require.Equal(t, []string{}, comms)
	}
	f("")
	f("-1")
	f("foo")
	f("4294967296")
	f("22697:10001,22697:4294967296")
}
