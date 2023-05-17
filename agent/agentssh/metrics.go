package agentssh

import "tailscale.com/util/clientmetric"

var (
	// SSH callbacks
	metricConnectionFailedCallback      = clientmetric.NewCounter("ssh_connection_failed_callback")
	metricLocalPortForwardingCallback   = clientmetric.NewCounter("ssh_local_port_forwarding_callback")
	metricPtyCallback                   = clientmetric.NewCounter("ssh_pty_callback")
	metricReversePortForwardingCallback = clientmetric.NewCounter("ssh_reverse_port_forwarding_callback")
	metricX11Callback                   = clientmetric.NewCounter("ssh_x11_callback")

	// Agent sessions
	metricsAgentCreateCommandError = clientmetric.NewCounter("ssh_agent_create_command_error")
	metricsAgentListenerError      = clientmetric.NewCounter("ssh_agent_listener_error")
	metricsStartPTYSession         = clientmetric.NewCounter("ssh_agent_start_pty_session")
	metricsStartNonPTYSession      = clientmetric.NewCounter("ssh_agent_start_non_pty_session")
	metricsSessionError            = clientmetric.NewCounter("ssh_session_error")

	// Non-PTY sessions
	metricsNonPTYStdinPipeError   = clientmetric.NewCounter("ssh_non_pty_stdin_pipe_error")
	metricsNonPTYStdinIoCopyError = clientmetric.NewCounter("ssh_non_pty_stdin_io_copy_error")
	metricsNonPTYCmdStartError    = clientmetric.NewCounter("ssh_non_pty_cmd_start_error")

	// PTY sessions
	metricsPTYMotdError         = clientmetric.NewCounter("ssh_pty_motd_error")
	metricsPTYCmdStartError     = clientmetric.NewCounter("ssh_pty_cmd_start_error")
	metricsPTYCloseError        = clientmetric.NewCounter("ssh_pty_close_error")
	metricsPTYResizeError       = clientmetric.NewCounter("ssh_pty_resize_error")
	metricsPTYInputIoCopyError  = clientmetric.NewCounter("ssh_pty_input_io_copy_error")
	metricsPTYOutputIoCopyError = clientmetric.NewCounter("ssh_pty_output_io_copy_error")
	metricsPTYWaitError         = clientmetric.NewCounter("ssh_pty_wait_error")

	// SFTP
	metricSftpHandler     = clientmetric.NewCounter("ssh_sftp_handler")
	metricSftpServerError = clientmetric.NewCounter("ssh_sftp_server_error")

	// X11
	metricX11SocketDirError  = clientmetric.NewCounter("ssh_x11_socket_dir_error")
	metricX11XauthorityError = clientmetric.NewCounter("ssh_x11_xauthority_error")
)
