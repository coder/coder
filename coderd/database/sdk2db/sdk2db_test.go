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

func TestConnectionLogFromAgentConnection(t *testing.T) {
	t.Parallel()

	const (
		ssh  = database.ConnectionLogMethodSSH
		pty  = database.ConnectionLogMethodReconnectingPTY
		port = database.ConnectionLogMethodPortForwarding
	)
	for _, tc := range []struct {
		name    string
		method  agentproto.Connection_Method
		typ     agentproto.Connection_Type
		appName string
		want    database.ConnectionLogMethod
		wantApp string
	}{
		{"SSHApp", agentproto.Connection_METHOD_SSH, 0, "Cursor", ssh, "cursor"},
		{"SSHWithoutApp", agentproto.Connection_METHOD_SSH, 0, "", ssh, ""},
		{"TypeIgnoredWithMethod", agentproto.Connection_METHOD_SSH, agentproto.Connection_VSCODE, "", ssh, ""},
		{"PTYApp", agentproto.Connection_METHOD_RECONNECTING_PTY, 0, "Some-App", pty, "some_app"},
		// A forward's column holds its destination, not an app.
		{"PortForwarding", agentproto.Connection_METHOD_PORT_FORWARDING, 0, "8080", port, ""},
		// Agents before API v2.13 send only the type.
		{"LegacyUnspecified", agentproto.Connection_METHOD_UNSPECIFIED, agentproto.Connection_TYPE_UNSPECIFIED, "", ssh, ""},
		{"LegacyVSCode", agentproto.Connection_METHOD_UNSPECIFIED, agentproto.Connection_VSCODE, "", ssh, "vscode"},
		{"LegacyPTY", agentproto.Connection_METHOD_UNSPECIFIED, agentproto.Connection_RECONNECTING_PTY, "", pty, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			method, appName, err := sdk2db.ConnectionLogFromAgentConnection(&agentproto.Connection{
				ConnectionMethod: tc.method,
				Type:             tc.typ,
				AppName:          tc.appName,
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, method)
			require.Equal(t, tc.wantApp, appName)
		})
	}

	// Unknown methods are never recorded as SSH.
	_, _, err := sdk2db.ConnectionLogFromAgentConnection(&agentproto.Connection{ConnectionMethod: agentproto.Connection_Method(42)})
	require.Error(t, err)
	_, _, err = sdk2db.ConnectionLogFromAgentConnection(&agentproto.Connection{Type: agentproto.Connection_Type(1000)})
	require.Error(t, err)
}
