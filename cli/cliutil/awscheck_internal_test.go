package cliutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/testutil"
)

func TestIPV4Check(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpapi.Write(context.Background(), w, http.StatusOK, awsIPRangesResponse{
			IPV4Prefixes: []awsIPv4Prefix{
				{
					Prefix: "3.24.0.0/14",
				},
				{
					Prefix: "15.230.15.29/32",
				},
				{
					Prefix: "47.128.82.100/31",
				},
			},
			IPV6Prefixes: []awsIPv6Prefix{
				{
					Prefix: "2600:9000:5206::/48",
				},
				{
					Prefix: "2406:da70:8800::/40",
				},
				{
					Prefix: "2600:1f68:5000::/40",
				},
			},
		})
	}))
	t.Cleanup(srv.Close)
	ctx := testutil.Context(t, testutil.WaitShort)
	ranges, err := FetchAWSIPRanges(ctx, srv.URL)
	require.NoError(t, err)

	t.Run("Private/IPV4", func(t *testing.T) {
		t.Parallel()
		ip, err := netip.ParseAddr("192.168.0.1")
		require.NoError(t, err)
		isAws := ranges.CheckIP(ip)
		require.False(t, isAws)
	})

	t.Run("AWS/IPV4", func(t *testing.T) {
		t.Parallel()
		ip, err := netip.ParseAddr("3.25.61.113")
		require.NoError(t, err)
		isAws := ranges.CheckIP(ip)
		require.True(t, isAws)
	})

	t.Run("NonAWS/IPV4", func(t *testing.T) {
		t.Parallel()
		ip, err := netip.ParseAddr("159.196.123.40")
		require.NoError(t, err)
		isAws := ranges.CheckIP(ip)
		require.False(t, isAws)
	})

	t.Run("Private/IPV6", func(t *testing.T) {
		t.Parallel()
		ip, err := netip.ParseAddr("::1")
		require.NoError(t, err)
		isAws := ranges.CheckIP(ip)
		require.False(t, isAws)
	})

	t.Run("AWS/IPV6", func(t *testing.T) {
		t.Parallel()
		ip, err := netip.ParseAddr("2600:9000:5206:0001:0000:0000:0000:0001")
		require.NoError(t, err)
		isAws := ranges.CheckIP(ip)
		require.True(t, isAws)
	})

	t.Run("NonAWS/IPV6", func(t *testing.T) {
		t.Parallel()
		ip, err := netip.ParseAddr("2403:5807:885f:0:a544:49d4:58f8:aedf")
		require.NoError(t, err)
		isAws := ranges.CheckIP(ip)
		require.False(t, isAws)
	})
}
