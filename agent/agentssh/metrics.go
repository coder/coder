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
	metricAgentCreateCommandError = clientmetric.NewCounter("ssh_agent_create_command_error")
	metricAgentListenerError      = clientmetric.NewCounter("ssh_agent_listener_error")
	metricStartPTYSession         = clientmetric.NewCounter("ssh_agent_start_pty_session")
	metricStartNonPTYSession      = clientmetric.NewCounter("ssh_agent_start_non_pty_session")
	metricSessionError            = clientmetric.NewCounter("ssh_session_error")

	// Non-PTY sessions
	metricNonPTYStdinPipeError   = clientmetric.NewCounter("ssh_non_pty_stdin_pipe_error")
	metricNonPTYStdinIoCopyError = clientmetric.NewCounter("ssh_non_pty_stdin_io_copy_error")
	metricNonPTYCmdStartError    = clientmetric.NewCounter("ssh_non_pty_cmd_start_error")

	// PTY sessions
	metricPTYMotdError         = clientmetric.NewCounter("ssh_pty_motd_error")
	metricPTYCmdStartError     = clientmetric.NewCounter("ssh_pty_cmd_start_error")
	metricPTYCloseError        = clientmetric.NewCounter("ssh_pty_close_error")
	metricPTYResizeError       = clientmetric.NewCounter("ssh_pty_resize_error")
	metricPTYInputIoCopyError  = clientmetric.NewCounter("ssh_pty_input_io_copy_error")
	metricPTYOutputIoCopyError = clientmetric.NewCounter("ssh_pty_output_io_copy_error")
	metricPTYWaitError         = clientmetric.NewCounter("ssh_pty_wait_error")

	// SFTP
	metricSftpHandler     = clientmetric.NewCounter("ssh_sftp_handler")
	metricSftpServerError = clientmetric.NewCounter("ssh_sftp_server_error")

	// X11
	metricX11SocketDirError  = clientmetric.NewCounter("ssh_x11_socket_dir_error")
	metricX11XauthorityError = clientmetric.NewCounter("ssh_x11_xauthority_error")
)
