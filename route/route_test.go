package route

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseVIP_NoError(t *testing.T) {
	t.Parallel()

	f := func(addr, expected string) {
		t.Helper()

		cidr, err := ParseVIP(addr)
		require.NoError(t, err)
		require.Equal(t, expected, cidr.String())
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

		cidr, err := ParseVIP(addr)
		require.Error(t, err)
		require.Nil(t, cidr)
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

	f := func(bgpCommString string, expected Communities) {
		t.Helper()

		comms, err := ParseBgpCommunities(bgpCommString)
		require.NoError(t, err)
		require.Equal(t, expected, comms)
	}
	f("", Communities{})
	f("0", Communities{uint32(0)})
	f("4294967295", Communities{uint32(4294967295)})
	f("22697:10001", Communities{(uint32(22697)<<16 | uint32(10001))})
	f("22697:10001,22697:10002", Communities{(uint32(22697)<<16 | uint32(10001)), (uint32(22697)<<16 | uint32(10002))})
}

func TestParseBgpCommunities_Error(t *testing.T) {
	t.Parallel()

	f := func(bgpCommString string) {
		t.Helper()

		comms, err := ParseBgpCommunities(bgpCommString)
		require.Error(t, err)
		require.Equal(t, Communities{}, comms)
	}
	f("-1")
	f("foo")
	f("4294967296")
	f("22697:10001:10002")
	f("22697:10001,22697:4294967296")
}
