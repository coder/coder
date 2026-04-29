package xreplicasync_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/enterprise/coderd/x/xreplicasync"
)

func TestRouteURLFromReplicaHostname_ConstructorValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		scheme string
		port   int
	}{
		{"empty scheme", "", 6222},
		{"http scheme", "http", 6222},
		{"zero port", "nats", 0},
		{"negative port", "tls", -1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := xreplicasync.RouteURLFromReplicaHostname(tc.scheme, tc.port)
			require.Error(t, err)
		})
	}
}

func TestRouteURLFromReplicaHostname_Success(t *testing.T) {
	t.Parallel()

	for _, scheme := range []string{"nats", "tls"} {
		scheme := scheme
		t.Run(scheme, func(t *testing.T) {
			t.Parallel()
			fn, err := xreplicasync.RouteURLFromReplicaHostname(scheme, 6222)
			require.NoError(t, err)
			got, err := fn(database.Replica{ID: uuid.New(), Hostname: "host"})
			require.NoError(t, err)
			require.Equal(t, scheme+"://host:6222", got)
		})
	}
}

func TestRouteURLFromReplicaHostname_EmptyHostname(t *testing.T) {
	t.Parallel()

	fn, err := xreplicasync.RouteURLFromReplicaHostname("nats", 6222)
	require.NoError(t, err)
	_, err = fn(database.Replica{ID: uuid.New(), Hostname: ""})
	require.Error(t, err)
}

func TestRouteURLFromRelayAddress_ConstructorValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		scheme string
		port   int
	}{
		{"empty scheme", "", 6222},
		{"http scheme", "http", 6222},
		{"zero port", "nats", 0},
		{"negative port", "tls", -1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := xreplicasync.RouteURLFromRelayAddress(tc.scheme, tc.port)
			require.Error(t, err)
		})
	}
}

func TestRouteURLFromRelayAddress_Success(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		scheme string
		relay  string
		want   string
	}{
		{"hostname with port", "tls", "http://example.com:8080", "tls://example.com:6222"},
		{"hostname without port", "nats", "http://10.0.0.1", "nats://10.0.0.1:6222"},
		{"https relay", "tls", "https://node-1.internal:9443", "tls://node-1.internal:6222"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fn, err := xreplicasync.RouteURLFromRelayAddress(tc.scheme, 6222)
			require.NoError(t, err)
			got, err := fn(database.Replica{ID: uuid.New(), RelayAddress: tc.relay})
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestRouteURLFromRelayAddress_Errors(t *testing.T) {
	t.Parallel()

	fn, err := xreplicasync.RouteURLFromRelayAddress("nats", 6222)
	require.NoError(t, err)

	_, err = fn(database.Replica{ID: uuid.New(), RelayAddress: ""})
	require.Error(t, err)

	_, err = fn(database.Replica{ID: uuid.New(), RelayAddress: "://bad-url"})
	require.Error(t, err)

	// Parses without error but yields an empty hostname.
	_, err = fn(database.Replica{ID: uuid.New(), RelayAddress: "/relative/path"})
	require.Error(t, err)
}
