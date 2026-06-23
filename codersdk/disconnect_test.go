package codersdk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestDisconnectReason_Valid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		reason codersdk.DisconnectReason
		valid  bool
	}{
		{codersdk.DisconnectReasonUnknown, true},
		{codersdk.DisconnectReasonGraceful, true},
		{codersdk.DisconnectReasonClientClosed, true},
		{codersdk.DisconnectReasonServerShutdown, true},
		{codersdk.DisconnectReasonNetworkError, true},
		{codersdk.DisconnectReasonProtocolError, true},
		{codersdk.DisconnectReasonWorkspaceStopped, true},
		{codersdk.DisconnectReasonControlPlaneLost, true},
		{codersdk.DisconnectReason("not_a_real_reason"), false},
	}

	for _, c := range cases {
		require.Equal(t, c.valid, c.reason.Valid(), "reason=%q", c.reason)
	}
}

func TestDisconnectReason_Expected(t *testing.T) {
	t.Parallel()

	expected := map[codersdk.DisconnectReason]bool{
		codersdk.DisconnectReasonGraceful:         true,
		codersdk.DisconnectReasonClientClosed:     true,
		codersdk.DisconnectReasonServerShutdown:   true,
		codersdk.DisconnectReasonWorkspaceStopped: true,

		codersdk.DisconnectReasonUnknown:          false,
		codersdk.DisconnectReasonNetworkError:     false,
		codersdk.DisconnectReasonProtocolError:    false,
		codersdk.DisconnectReasonControlPlaneLost: false,
	}

	for reason, want := range expected {
		require.Equal(t, want, reason.Expected(), "reason=%q", reason)
	}

	// Unknown values default to not-expected so that uncategorized
	// emit sites surface in the "investigate" bucket.
	require.False(t, codersdk.DisconnectReason("not_a_real_reason").Expected())
}

func TestDisconnectInitiator_Valid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		initiator codersdk.DisconnectInitiator
		valid     bool
	}{
		{codersdk.DisconnectInitiatorUnknown, true},
		{codersdk.DisconnectInitiatorClient, true},
		{codersdk.DisconnectInitiatorAgent, true},
		{codersdk.DisconnectInitiatorServer, true},
		{codersdk.DisconnectInitiatorNetwork, true},
		{codersdk.DisconnectInitiator("nobody"), false},
	}

	for _, c := range cases {
		require.Equal(t, c.valid, c.initiator.Valid(), "initiator=%q", c.initiator)
	}
}

func TestConnectionMethod_Valid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		method codersdk.ConnectionMethod
		valid  bool
	}{
		{codersdk.ConnectionMethodUnknown, true},
		{codersdk.ConnectionMethodDirect, true},
		{codersdk.ConnectionMethodDERP, true},
		{codersdk.ConnectionMethod("magic"), false},
	}

	for _, c := range cases {
		require.Equal(t, c.valid, c.method.Valid(), "method=%q", c.method)
	}
}
