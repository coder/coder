// Package sdk2db provides common conversion routines from codersdk types to database types
package sdk2db

import (
	"golang.org/x/xerrors"

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

// ConnectionLogFromAgentType recovers the method and app name from legacy
// reports.
// An unspecified legacy type was used by SSH handlers for unfamiliar apps.
func ConnectionLogFromAgentType(typ agentproto.Connection_Type) (database.ConnectionLogMethod, string, error) {
	switch typ {
	case agentproto.Connection_SSH, agentproto.Connection_TYPE_UNSPECIFIED:
		return database.ConnectionLogMethodSSH, "", nil
	case agentproto.Connection_JETBRAINS:
		return database.ConnectionLogMethodSSH, "jetbrains", nil
	case agentproto.Connection_VSCODE:
		return database.ConnectionLogMethodSSH, "vscode", nil
	case agentproto.Connection_RECONNECTING_PTY:
		return database.ConnectionLogMethodReconnectingPTY, "", nil
	default:
		return "", "", xerrors.Errorf("unsupported agent connection type %d", typ)
	}
}

// ConnectionLogFromAgentConnection returns the method and app to log for an
// agent report. Unknown methods are rejected so they are never logged as SSH.
func ConnectionLogFromAgentConnection(conn *agentproto.Connection) (database.ConnectionLogMethod, string, error) {
	var method database.ConnectionLogMethod
	switch m := conn.GetConnectionMethod(); m {
	case agentproto.Connection_METHOD_UNSPECIFIED:
		// Agents before API v2.13 send only the type.
		return ConnectionLogFromAgentType(conn.GetType())
	case agentproto.Connection_METHOD_SSH:
		method = database.ConnectionLogMethodSSH
	case agentproto.Connection_METHOD_RECONNECTING_PTY:
		method = database.ConnectionLogMethodReconnectingPTY
	case agentproto.Connection_METHOD_PORT_FORWARDING:
		// app_name_or_port holds a forward's destination, not a client app.
		return database.ConnectionLogMethodPortForwarding, "", nil
	default:
		return "", "", xerrors.Errorf("unsupported agent connection method %d", m)
	}
	if appName := conn.GetAppName(); appName != "" {
		return method, codersdk.NormalizeAppName(appName), nil
	}
	return method, "", nil
}
