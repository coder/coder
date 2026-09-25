// Package sdk2db provides common conversion routines from codersdk types to database types
package sdk2db

import (
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

func ProvisionerDaemonStatus(status codersdk.ProvisionerDaemonStatus) database.ProvisionerDaemonStatus {
	return database.ProvisionerDaemonStatus(status)
}

func ProvisionerDaemonStatuses(params []codersdk.ProvisionerDaemonStatus) []database.ProvisionerDaemonStatus {
	return slice.List(params, ProvisionerDaemonStatus)
}

// Returns what a `type:` filter matches: a web source, or the apps of a
// family. Unknown excludes the registered apps.
func ConnectionLogTypeFilter(t codersdk.ConnectionType) (source database.ConnectionSource, appNames, excludedAppNames []string) {
	switch {
	case t == "":
		return "", nil, nil
	case t == codersdk.ConnectionTypeUnknown:
		return "", nil, codersdk.KnownConnectionAppNames()
	case t.IsWeb():
		return database.ConnectionSource(t), nil, nil
	default:
		return "", t.AppNames(), nil
	}
}

// Prefers the app the agent reported. Agents without app_name send only the
// type, whose family is also a registered app name. Anything else is an
// unknown app, so the connection is still logged.
func ConnectionLogAppName(conn *agentproto.Connection) string {
	if appName := conn.GetAppName(); appName != "" {
		return codersdk.NormalizeAppName(appName)
	}
	switch conn.GetType() {
	case agentproto.Connection_SSH:
		return string(codersdk.AppFamilySSH)
	case agentproto.Connection_JETBRAINS:
		return string(codersdk.AppFamilyJetBrains)
	case agentproto.Connection_VSCODE:
		return string(codersdk.AppFamilyVSCode)
	case agentproto.Connection_RECONNECTING_PTY:
		return string(codersdk.AppFamilyReconnectingPTY)
	default:
		return string(codersdk.AppFamilyUnknown)
	}
}
