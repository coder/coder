package agentssh

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"tailscale.com/util/clientmetric"
)

type sshServerMetrics struct {
	// SSH callbacks
	connectionFailedCallback      prometheus.Counter
	localPortForwardingCallback   prometheus.Counter
	ptyCallback                   prometheus.Counter
	reversePortForwardingCallback prometheus.Counter
	x11Callback                   prometheus.Counter

	// SFTP
	sftpHandler     prometheus.Counter
	sftpServerError prometheus.Counter

	// X11
	x11SocketDirError  prometheus.Counter
	x11XauthorityError prometheus.Counter
}

func newSSHServerMetrics(registerer prometheus.Registerer) *sshServerMetrics {
	connectionFailedCallback := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "connection_failed_callback",
	})
	registerer.MustRegister(connectionFailedCallback)

	localPortForwardingCallback := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "local_port_forwarding_callback",
	})
	registerer.MustRegister(localPortForwardingCallback)

	ptyCallback := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "pty_callback",
	})
	registerer.MustRegister(ptyCallback)

	reversePortForwardingCallback := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "reverse_port_forwarding_callback",
	})
	registerer.MustRegister(reversePortForwardingCallback)

	x11Callback := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "x11_callback",
	})
	registerer.MustRegister(x11Callback)

	sftpHandler := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "sftp_handler",
	})
	registerer.MustRegister(sftpHandler)

	sftpServerError := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "sftp_server_error",
	})
	registerer.MustRegister(sftpServerError)

	x11SocketDirError := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "x11_socket_dir_error",
	})
	registerer.MustRegister(x11SocketDirError)

	x11XauthorityError := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agent", Subsystem: "ssh_server", Name: "x11_xauthority_error",
	})
	registerer.MustRegister(x11XauthorityError)

	return &sshServerMetrics{
		connectionFailedCallback:      connectionFailedCallback,
		localPortForwardingCallback:   localPortForwardingCallback,
		ptyCallback:                   ptyCallback,
		reversePortForwardingCallback: reversePortForwardingCallback,
		x11Callback:                   x11Callback,
		sftpHandler:                   sftpHandler,
		sftpServerError:               sftpServerError,
		x11SocketDirError:             x11SocketDirError,
		x11XauthorityError:            x11XauthorityError,
	}
}

var sessionMetrics = map[string]sessionMetricsObject{}

type sessionMetricsObject struct {
	// Agent sessions
	agentCreateCommandError *clientmetric.Metric
	agentListenerError      *clientmetric.Metric
	startPTYSession         *clientmetric.Metric
	startNonPTYSession      *clientmetric.Metric
	sessionError            *clientmetric.Metric

	// Non-PTY sessions
	nonPTYStdinPipeError   *clientmetric.Metric
	nonPTYStdinIoCopyError *clientmetric.Metric
	nonPTYCmdStartError    *clientmetric.Metric

	// PTY sessions
	ptyMotdError         *clientmetric.Metric
	ptyCmdStartError     *clientmetric.Metric
	ptyCloseError        *clientmetric.Metric
	ptyResizeError       *clientmetric.Metric
	ptyInputIoCopyError  *clientmetric.Metric
	ptyOutputIoCopyError *clientmetric.Metric
	ptyWaitError         *clientmetric.Metric
}

func init() {
	for _, magicType := range []string{MagicSessionTypeVSCode, MagicSessionTypeJetBrains, "ssh", "unknown"} {
		sessionMetrics[magicType] = sessionMetricsObject{
			agentCreateCommandError: clientmetric.NewCounter(fmt.Sprintf("ssh_agent_%s_create_command_error", magicType)),
			agentListenerError:      clientmetric.NewCounter(fmt.Sprintf("ssh_agent_%s_listener_error", magicType)),
			startPTYSession:         clientmetric.NewCounter(fmt.Sprintf("ssh_agent_%s_start_pty_session", magicType)),
			startNonPTYSession:      clientmetric.NewCounter(fmt.Sprintf("ssh_agent_%s_start_non_pty_session", magicType)),
			sessionError:            clientmetric.NewCounter(fmt.Sprintf("ssh_agent_%s_session_error", magicType)),

			nonPTYStdinPipeError:   clientmetric.NewCounter(fmt.Sprintf("ssh_server_%s_non_pty_stdin_pipe_error", magicType)),
			nonPTYStdinIoCopyError: clientmetric.NewCounter(fmt.Sprintf("ssh_server_%s_non_pty_stdin_io_copy_error", magicType)),
			nonPTYCmdStartError:    clientmetric.NewCounter(fmt.Sprintf("ssh_server_%s_non_pty_cmd_start_error", magicType)),

			ptyMotdError:         clientmetric.NewCounter(fmt.Sprintf("ssh_server_%s_pty_motd_error", magicType)),
			ptyCmdStartError:     clientmetric.NewCounter(fmt.Sprintf("ssh_server_%s_pty_cmd_start_error", magicType)),
			ptyCloseError:        clientmetric.NewCounter(fmt.Sprintf("ssh_server_%s_pty_close_error", magicType)),
			ptyResizeError:       clientmetric.NewCounter(fmt.Sprintf("ssh_server_%s_pty_resize_error", magicType)),
			ptyInputIoCopyError:  clientmetric.NewCounter(fmt.Sprintf("ssh_server_%s_pty_input_io_copy_error", magicType)),
			ptyOutputIoCopyError: clientmetric.NewCounter(fmt.Sprintf("ssh_server_%s_pty_output_io_copy_error", magicType)),
			ptyWaitError:         clientmetric.NewCounter(fmt.Sprintf("ssh_server_%s_pty_wait_error", magicType)),
		}
	}
}

func metricsForSession(magicType string) sessionMetricsObject {
	switch magicType {
	case MagicSessionTypeVSCode:
	case MagicSessionTypeJetBrains:
	case "":
		magicType = "ssh"
	default:
		magicType = "unknown"
	}
	return sessionMetrics[magicType]
}
