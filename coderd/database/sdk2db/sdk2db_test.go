package sdk2db_test

import (
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

	for _, typ := range codersdk.FilterableConnectionTypes() {
		source, appNames, excludedAppNames := sdk2db.ConnectionLogTypeFilter(typ)
		switch {
		case source != "":
			require.Equal(t, typ, db2sdk.ConnectionLogType(source, ""), typ)
		case typ == codersdk.ConnectionTypeUnknown:
			require.NotContains(t, excludedAppNames, "an_unregistered_ide")
			require.Equal(t, typ, db2sdk.ConnectionLogType(database.ConnectionSourceAgent, "an_unregistered_ide"), typ)
		default:
			require.NotEmpty(t, appNames, typ)
			for _, appName := range appNames {
				require.Equal(t, typ, db2sdk.ConnectionLogType(database.ConnectionSourceAgent, appName), appName)
			}
		}
	}
}

func TestConnectionLogAppName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		appName string
		typ     agentproto.Connection_Type
		want    string
	}{
		{"AppNamePreferred", "cursor", agentproto.Connection_VSCODE, "cursor"},
		{"AppNameNormalized", "  VS-Code  ", agentproto.Connection_VSCODE, "vs_code"},
		// Agents without app_name send only the type.
		{"TypeFallback", "", agentproto.Connection_JETBRAINS, "jetbrains"},
		{"WebTerminal", "", agentproto.Connection_RECONNECTING_PTY, "reconnecting_pty"},
		{"Unspecified", "", agentproto.Connection_TYPE_UNSPECIFIED, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, sdk2db.ConnectionLogAppName(&agentproto.Connection{
				AppName: tc.appName,
				Type:    tc.typ,
			}))
		})
	}
}
