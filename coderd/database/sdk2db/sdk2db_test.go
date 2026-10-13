package sdk2db_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/sdk2db"
	"github.com/coder/coder/v2/codersdk"
)

func TestProvisionerDaemonStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  codersdk.ProvisionerDaemonStatus
		expect database.ProvisionerDaemonStatus
	}{
		{"busy", codersdk.ProvisionerDaemonBusy, database.ProvisionerDaemonStatusBusy},
		{"offline", codersdk.ProvisionerDaemonOffline, database.ProvisionerDaemonStatusOffline},
		{"idle", codersdk.ProvisionerDaemonIdle, database.ProvisionerDaemonStatusIdle},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sdk2db.ProvisionerDaemonStatus(tc.input)
			if !got.Valid() {
				t.Errorf("ProvisionerDaemonStatus(%v) returned invalid status", tc.input)
			}
			if got != tc.expect {
				t.Errorf("ProvisionerDaemonStatus(%v) = %v; want %v", tc.input, got, tc.expect)
			}
		})
	}
}

func TestConnectionLogFromAgentType(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		typ     agentproto.Connection_Type
		method  database.ConnectionLogMethod
		appName string
	}{
		{agentproto.Connection_SSH, database.ConnectionLogMethodSSH, ""},
		// SSH handlers reported unfamiliar apps as unspecified.
		{agentproto.Connection_TYPE_UNSPECIFIED, database.ConnectionLogMethodSSH, ""},
		{agentproto.Connection_VSCODE, database.ConnectionLogMethodSSH, "vscode"},
		{agentproto.Connection_JETBRAINS, database.ConnectionLogMethodSSH, "jetbrains"},
		{agentproto.Connection_RECONNECTING_PTY, database.ConnectionLogMethodReconnectingPTY, ""},
	} {
		method, appName, err := sdk2db.ConnectionLogFromAgentType(tc.typ)
		require.NoError(t, err, tc.typ)
		require.Equal(t, tc.method, method, tc.typ)
		require.Equal(t, tc.appName, appName, tc.typ)
	}

	_, _, err := sdk2db.ConnectionLogFromAgentType(agentproto.Connection_Type(1000))
	require.Error(t, err)
}
