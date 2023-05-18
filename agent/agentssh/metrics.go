package agentssh

import (
	"fmt"

	"tailscale.com/util/clientmetric"
)

var (
	// SSH callbacks
	metricConnectionFailedCallback      = clientmetric.NewCounter("ssh_connection_failed_callback")
	metricLocalPortForwardingCallback   = clientmetric.NewCounter("ssh_local_port_forwarding_callback")
	metricPtyCallback                   = clientmetric.NewCounter("ssh_pty_callback")
	metricReversePortForwardingCallback = clientmetric.NewCounter("ssh_reverse_port_forwarding_callback")
	metricX11Callback                   = clientmetric.NewCounter("ssh_x11_callback")

	// SFTP
	metricSftpHandler     = clientmetric.NewCounter("ssh_sftp_handler")
	metricSftpServerError = clientmetric.NewCounter("ssh_sftp_server_error")

	// X11
	metricX11SocketDirError  = clientmetric.NewCounter("ssh_x11_socket_dir_error")
	metricX11XauthorityError = clientmetric.NewCounter("ssh_x11_xauthority_error")
)

var (
	sessionMetrics = map[string]sessionMetricsObject{}
)

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
