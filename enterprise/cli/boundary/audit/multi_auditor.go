//nolint:revive,gocritic,errname,unconvert
package audit

import (
	"context"
	"log/slog"
	"os"

	"golang.org/x/xerrors"
)

// MultiAuditor wraps multiple auditors and sends audit events to all of them.
type MultiAuditor struct {
	auditors []Auditor
}

// NewMultiAuditor creates a new MultiAuditor that sends to all provided auditors.
func NewMultiAuditor(auditors ...Auditor) *MultiAuditor {
	return &MultiAuditor{auditors: auditors}
}

// AuditRequest sends the request to all wrapped auditors.
func (m *MultiAuditor) AuditRequest(req Request) {
	for _, a := range m.auditors {
		a.AuditRequest(req)
	}
}

// SetupAuditor creates and configures the appropriate auditors based on the
// provided configuration. It always includes a LogAuditor for stderr logging,
// and conditionally adds a SocketAuditor if audit logs are enabled and the
// workspace agent's log proxy socket exists.
func SetupAuditor(ctx context.Context, logger *slog.Logger, disableAuditLogs bool, logProxySocketPath string) (Auditor, error) {
	stderrAuditor := NewLogAuditor(logger)
	auditors := []Auditor{stderrAuditor}

	if !disableAuditLogs {
		if logProxySocketPath == "" {
			return nil, xerrors.New("log proxy socket path is undefined")
		}
		// Since boundary is separately versioned from a Coder deployment, it's possible
		// Coder is on an older version that will not create the socket and listen for
		// the audit logs. Here we check for the socket to determine if the workspace
		// agent is on a new enough version to prevent boundary application log spam from
		// trying to connect to the agent. This assumes the agent will run and start the
		// log proxy server before boundary runs.
		_, err := os.Stat(logProxySocketPath)
		if err != nil && !os.IsNotExist(err) {
			return nil, xerrors.Errorf("failed to stat log proxy socket: %v", err)
		}
		agentWillProxy := !os.IsNotExist(err)
		if agentWillProxy {
			socketAuditor := NewSocketAuditor(logger, logProxySocketPath)
			go socketAuditor.Loop(ctx)
			auditors = append(auditors, socketAuditor)
		} else {
			logger.Warn("Audit logs are disabled; workspace agent has not created log proxy socket",
				"socket", logProxySocketPath)
		}
	} else {
		logger.Warn("Audit logs are disabled by configuration")
	}

	return NewMultiAuditor(auditors...), nil
}
