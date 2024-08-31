package cluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolvePeers(t *testing.T) {
	t.Parallel()

	addrs, err := resolvePeers(context.Background(), []string{"127.0.0.1:8179"}, "127.0.0.1", false)
	require.NoError(t, err)
	require.NotEmpty(t, addrs)

	addrs, err = resolvePeers(context.Background(), []string{"www.google.com:80"}, "127.0.0.1", true)
	require.NoError(t, err)
	require.NotEmpty(t, addrs)

	addrs, err = resolvePeers(context.Background(), []string{"dns+srv://consul.service.chi1.consul"}, "127.0.0.1", true)
	require.NoError(t, err)
	require.NotEmpty(t, addrs)

	addrs, err = resolvePeers(context.Background(), []string{""}, "127.0.0.1", false)
	require.Error(t, err)
	require.Empty(t, addrs)

	addrs, err = resolvePeers(context.Background(), []string{"www.google.com"}, "127.0.0.1", false)
	require.Error(t, err)
	require.Empty(t, addrs)

	addrs, err = resolvePeers(context.Background(), []string{":80"}, "127.0.0.1", false)
	require.Error(t, err)
	require.Empty(t, addrs)

	addrs, err = resolvePeers(context.Background(), []string{"dns+srv://_sip._tcp.example.com"}, "127.0.0.1", false)
	require.Error(t, err)
	require.Empty(t, addrs)
}
