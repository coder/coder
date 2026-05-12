package database_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func TestChatACLValueNilMapReturnsEmptyObject(t *testing.T) {
	t.Parallel()

	var acl database.ChatACL

	value, err := acl.Value()
	require.NoError(t, err)

	raw, ok := value.([]byte)
	require.True(t, ok)
	require.JSONEq(t, `{}`, string(raw))
}

func TestChatACLScanAndValueRoundTrip(t *testing.T) {
	t.Parallel()

	want := database.ChatACL{
		"user": {
			Permissions: []policy.Action{policy.ActionRead, policy.ActionSSH},
		},
	}

	value, err := want.Value()
	require.NoError(t, err)

	raw, ok := value.([]byte)
	require.True(t, ok)

	cases := []struct {
		name string
		src  any
	}{
		{name: "bytes", src: raw},
		{name: "string", src: string(raw)},
		{name: "raw_message", src: json.RawMessage(raw)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got database.ChatACL
			err := got.Scan(tc.src)
			require.NoError(t, err)
			require.Equal(t, want, got)

			roundTrip, err := got.Value()
			require.NoError(t, err)

			roundTripRaw, ok := roundTrip.([]byte)
			require.True(t, ok)
			require.JSONEq(t, string(raw), string(roundTripRaw))
		})
	}
}

func TestChatACLScanNilErrors(t *testing.T) {
	t.Parallel()

	var acl database.ChatACL

	err := acl.Scan(nil)
	require.Error(t, err)
}
