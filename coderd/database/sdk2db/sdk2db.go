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

// ConnectionLogFromAgentConnection returns the method and app name to log for
// an agent connection report.
func ConnectionLogFromAgentConnection(conn *agentproto.Connection) (database.ConnectionLogMethod, string, error) {
	appName := conn.GetAppName()
	if appName != "" {
		appName = codersdk.NormalizeAppName(appName)
	}
	switch method := conn.GetConnectionMethod(); method {
	case agentproto.Connection_METHOD_UNSPECIFIED:
		return connectionLogFromAgentType(conn.GetType())
	case agentproto.Connection_METHOD_SSH:
		return database.ConnectionLogMethodSSH, appName, nil
	case agentproto.Connection_METHOD_RECONNECTING_PTY:
		return database.ConnectionLogMethodReconnectingPTY, appName, nil
	case agentproto.Connection_METHOD_PORT_FORWARDING:
		// app_name_or_port holds a forward's destination, not a client app.
		return database.ConnectionLogMethodPortForwarding, "", nil
	default:
		return "", "", xerrors.Errorf("unsupported agent connection method %d", method)
	}
}

// connectionLogFromAgentType maps the type sent by agents before API v2.13.
// SSH handlers reported unfamiliar apps as unspecified.
func connectionLogFromAgentType(typ agentproto.Connection_Type) (database.ConnectionLogMethod, string, error) {
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
