package agentssh

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

type sshServerMetrics struct {
	failedConnectionsTotal prometheus.Counter

	// SFTP
	sftpConnectionsTotal prometheus.Counter
	sftpServerErrors     prometheus.Counter

	// X11
	x11SocketDirError  prometheus.Counter
	x11HostnameError   prometheus.Counter
	x11XauthorityError prometheus.Counter

	sessions sessionMetrics
}

type sessionMetricsObject struct {
	// Agent sessions
	agentCreateCommandError prometheus.Counter
	agentListenerError      prometheus.Counter
	startPTYSession         prometheus.Counter
	startNonPTYSession      prometheus.Counter

	// Non-PTY sessions
	nonPTYStdinPipeError   prometheus.Counter
	nonPTYStdinIoCopyError prometheus.Counter
	nonPTYCmdStartError    prometheus.Counter

	// PTY sessions
	ptyMotdError         prometheus.Counter
	ptyCmdStartError     prometheus.Counter
	ptyCloseError        prometheus.Counter
	ptyResizeError       prometheus.Counter
	ptyInputIoCopyError  prometheus.Counter
	ptyOutputIoCopyError prometheus.Counter
	ptyWaitError         prometheus.Counter
}

type sessionMetrics map[string]sessionMetricsObject

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

	x11HostnameError := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "x11_hostname_error",
	})
	registerer.MustRegister(x11HostnameError)

	x11SocketDirError := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "x11_socket_dir_error",
	})
	registerer.MustRegister(x11SocketDirError)

	x11XauthorityError := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "x11_xauthority_error",
	})
	registerer.MustRegister(x11XauthorityError)

	sessions := newSessionMetrics(registerer)

	return &sshServerMetrics{
		failedConnectionsTotal: failedConnectionsTotal,
		sftpConnectionsTotal:   sftpConnectionsTotal,
		sftpServerErrors:       sftpServerErrors,
		x11HostnameError:       x11HostnameError,
		x11SocketDirError:      x11SocketDirError,
		x11XauthorityError:     x11XauthorityError,

		sessions: sessions,
	}
}

func newSessionMetrics(registerer prometheus.Registerer) sessionMetrics {
	sm := sessionMetrics{}
	for _, magicType := range []string{MagicSessionTypeVSCode, MagicSessionTypeJetBrains, "ssh", "unknown"} {
		agentCreateCommandError := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "create_command_error",
		})
		registerer.MustRegister(agentCreateCommandError)

		agentListenerError := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "listener_error",
		})
		registerer.MustRegister(agentListenerError)

		startPTYSession := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "start_pty_session",
		})
		registerer.MustRegister(startPTYSession)

		startNonPTYSession := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "start_non_pty_session",
		})
		registerer.MustRegister(startNonPTYSession)

		nonPTYStdinPipeError := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "non_pty_stdin_pipe_error",
		})
		registerer.MustRegister(nonPTYStdinPipeError)

		nonPTYStdinIoCopyError := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "non_pty_io_copy_error",
		})
		registerer.MustRegister(nonPTYStdinIoCopyError)

		nonPTYCmdStartError := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "non_pty_io_start_error",
		})
		registerer.MustRegister(nonPTYCmdStartError)

		ptyMotdError := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "pty_motd_error",
		})
		registerer.MustRegister(ptyMotdError)

		ptyCmdStartError := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "pty_cmd_start_error",
		})
		registerer.MustRegister(ptyCmdStartError)

		ptyCloseError := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "pty_close_error",
		})
		registerer.MustRegister(ptyCloseError)

		ptyResizeError := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "pty_resize_error",
		})
		registerer.MustRegister(ptyResizeError)

		ptyInputIoCopyError := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "pty_input_io_copy_error",
		})
		registerer.MustRegister(ptyInputIoCopyError)

		ptyOutputIoCopyError := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "pty_output_io_copy_error",
		})
		registerer.MustRegister(ptyOutputIoCopyError)

		ptyWaitError := prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "agent", Subsystem: fmt.Sprintf("sessions_%s", magicType), Name: "pty_wait_error",
		})
		registerer.MustRegister(ptyWaitError)

		sm[magicType] = sessionMetricsObject{
			agentCreateCommandError: agentCreateCommandError,
			agentListenerError:      agentListenerError,
			startPTYSession:         startPTYSession,
			startNonPTYSession:      startNonPTYSession,

			nonPTYStdinPipeError:   nonPTYStdinPipeError,
			nonPTYStdinIoCopyError: nonPTYStdinIoCopyError,
			nonPTYCmdStartError:    nonPTYCmdStartError,

			ptyMotdError:         ptyMotdError,
			ptyCmdStartError:     ptyCmdStartError,
			ptyCloseError:        ptyCloseError,
			ptyResizeError:       ptyResizeError,
			ptyInputIoCopyError:  ptyInputIoCopyError,
			ptyOutputIoCopyError: ptyOutputIoCopyError,
			ptyWaitError:         ptyWaitError,
		}
	}
	return sm
}

func metricsForSession(m sessionMetrics, magicType string) sessionMetricsObject {
	switch magicType {
	case MagicSessionTypeVSCode:
	case MagicSessionTypeJetBrains:
	case "":
		magicType = "ssh"
	default:
		magicType = "unknown"
	}
	return m[magicType]
}
