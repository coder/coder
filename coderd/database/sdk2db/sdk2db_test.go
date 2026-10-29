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

func TestConnectionLogFromAgentConnection(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		conn    *agentproto.Connection
		method  database.ConnectionLogMethod
		appName string
	}{
		{"SSH", &agentproto.Connection{ConnectionMethod: agentproto.Connection_METHOD_SSH}, database.ConnectionLogMethodSSH, ""},
		{"SSHApp", &agentproto.Connection{ConnectionMethod: agentproto.Connection_METHOD_SSH, AppName: "Some-IDE"}, database.ConnectionLogMethodSSH, "some_ide"},
		{"SSHIgnoresType", &agentproto.Connection{ConnectionMethod: agentproto.Connection_METHOD_SSH, Type: agentproto.Connection_VSCODE}, database.ConnectionLogMethodSSH, ""},
		{"ReconnectingPTY", &agentproto.Connection{ConnectionMethod: agentproto.Connection_METHOD_RECONNECTING_PTY}, database.ConnectionLogMethodReconnectingPTY, ""},
		{"PortForwarding", &agentproto.Connection{ConnectionMethod: agentproto.Connection_METHOD_PORT_FORWARDING, AppName: "cursor"}, database.ConnectionLogMethodPortForwarding, ""},
		{"LegacySSH", &agentproto.Connection{Type: agentproto.Connection_SSH}, database.ConnectionLogMethodSSH, ""},
		{"LegacyUnspecified", &agentproto.Connection{}, database.ConnectionLogMethodSSH, ""},
		{"LegacyVSCode", &agentproto.Connection{Type: agentproto.Connection_VSCODE}, database.ConnectionLogMethodSSH, "vscode"},
		{"LegacyJetBrains", &agentproto.Connection{Type: agentproto.Connection_JETBRAINS}, database.ConnectionLogMethodSSH, "jetbrains"},
		{"LegacyReconnectingPTY", &agentproto.Connection{Type: agentproto.Connection_RECONNECTING_PTY}, database.ConnectionLogMethodReconnectingPTY, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			method, appName, err := sdk2db.ConnectionLogFromAgentConnection(tc.conn)
			require.NoError(t, err)
			require.Equal(t, tc.method, method)
			require.Equal(t, tc.appName, appName)
		})
	}

	for _, conn := range []*agentproto.Connection{
		{ConnectionMethod: agentproto.Connection_Method(1000)},
		{Type: agentproto.Connection_Type(1000)},
	} {
		_, _, err := sdk2db.ConnectionLogFromAgentConnection(conn)
		require.Error(t, err, conn)
	}
}
