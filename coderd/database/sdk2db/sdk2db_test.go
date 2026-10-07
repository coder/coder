package sdk2db_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
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

// A row matched by a type filter reads back as that type.
func TestConnectionLogTypeFilter(t *testing.T) {
	t.Parallel()

	sshFamilies := slices.Concat(codersdk.ConnectionTypeVSCode.AppNames(), codersdk.ConnectionTypeJetBrains.AppNames())
	for _, tc := range []struct {
		typ              codersdk.ConnectionType
		method           string
		appNames         []string
		excludedAppNames []string
	}{
		{typ: ""},
		{typ: codersdk.ConnectionTypeSSH, method: "ssh", excludedAppNames: sshFamilies},
		{typ: codersdk.ConnectionTypeVSCode, method: "ssh", appNames: codersdk.ConnectionTypeVSCode.AppNames()},
		{typ: codersdk.ConnectionTypeJetBrains, method: "ssh", appNames: codersdk.ConnectionTypeJetBrains.AppNames()},
		{typ: codersdk.ConnectionTypeReconnectingPTY, method: "reconnecting_pty"},
		{typ: codersdk.ConnectionTypeWorkspaceApp, method: "workspace_app"},
		{typ: codersdk.ConnectionTypePortForwarding, method: "port_forwarding"},
		{typ: codersdk.ConnectionTypeTunnel, method: "tunnel"},
	} {
		method, appNames, excludedAppNames := sdk2db.ConnectionLogTypeFilter(tc.typ)
		require.Equal(t, tc.method, method, tc.typ)
		require.ElementsMatch(t, tc.appNames, appNames, tc.typ)
		require.ElementsMatch(t, tc.excludedAppNames, excludedAppNames, tc.typ)
	}

	for _, typ := range codersdk.FilterableConnectionTypes() {
		method, appNames, excludedAppNames := sdk2db.ConnectionLogTypeFilter(typ)
		require.True(t, database.ConnectionLogMethod(method).Valid(), typ)
		switch {
		case len(appNames) > 0:
			for _, appName := range appNames {
				require.Equal(t, string(typ), db2sdk.ConnectionLogType(database.ConnectionLogMethod(method), appName), appName)
			}
		case len(excludedAppNames) > 0:
			// Plain SSH also matches absent and unregistered identities.
			for _, appName := range []string{"", "ssh", "zed", "an_unregistered_ide"} {
				require.NotContains(t, excludedAppNames, appName)
				require.Equal(t, string(typ), db2sdk.ConnectionLogType(database.ConnectionLogMethod(method), appName), appName)
			}
			for _, appName := range excludedAppNames {
				require.NotEqual(t, string(typ), db2sdk.ConnectionLogType(database.ConnectionLogMethod(method), appName), appName)
			}
		default:
			require.Equal(t, string(typ), db2sdk.ConnectionLogType(database.ConnectionLogMethod(method), ""), typ)
		}
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
