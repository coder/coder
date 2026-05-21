package agentssh

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

type sshServerMetrics struct {
	failedConnectionsTotal prometheus.Counter
	sftpConnectionsTotal   prometheus.Counter
	sftpServerErrors       prometheus.Counter
	x11HandlerErrors       *prometheus.CounterVec
	sessionsTotal          *prometheus.CounterVec
	sessionErrors          *prometheus.CounterVec
}

func newSSHServerMetrics(registerer prometheus.Registerer) *sshServerMetrics {
	failedConnectionsTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "failed_connections_total",
	})
	registerer.MustRegister(failedConnectionsTotal)

	sftpConnectionsTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "sftp_connections_total",
	})
	registerer.MustRegister(sftpConnectionsTotal)

	sftpServerErrors := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "sftp_server_errors_total",
	})
	registerer.MustRegister(sftpServerErrors)

	x11HandlerErrors := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "agent",
			Subsystem: "x11_handler",
			Name:      "errors_total",
		},
		[]string{"error_type"},
	)
	registerer.MustRegister(x11HandlerErrors)

	sessionsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "agent",
			Subsystem: "sessions",
			Name:      "total",
		},
		[]string{"magic_type", "pty"},
	)
	registerer.MustRegister(sessionsTotal)

	sessionErrors := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "agent",
			Subsystem: "sessions",
			Name:      "errors_total",
		},
		[]string{"magic_type", "pty", "error_type"},
	)
	registerer.MustRegister(sessionErrors)

	return &sshServerMetrics{
		failedConnectionsTotal: failedConnectionsTotal,
		sftpConnectionsTotal:   sftpConnectionsTotal,
		sftpServerErrors:       sftpServerErrors,
		x11HandlerErrors:       x11HandlerErrors,
		sessionsTotal:          sessionsTotal,
		sessionErrors:          sessionErrors,
	}
}

func magicTypeMetricLabel(magicType MagicSessionType) string {
	switch magicType {
	case MagicSessionTypeVSCode:
	case MagicSessionTypeJetBrains:
	case MagicSessionTypeSSH:
	case MagicSessionTypeUnknown:
	default:
		magicType = MagicSessionTypeUnknown
	}
	// Always be case insensitive
	return strings.ToLower(string(magicType))
}
