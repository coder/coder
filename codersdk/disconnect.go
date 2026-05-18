package codersdk

import "cdr.dev/slog/v3"

// SlogDisconnectDetail is the slog field for the free-form, human-readable
// detail string that supplements the structured reason. Use it for
// "exited with code 137" style information that does not fit a category.
func SlogDisconnectDetail(detail string) slog.Field {
	return slog.F("disconnect_detail", detail)
}

// DisconnectReason categorizes why a workspace connection ended. It is
// emitted as a slog field at every disconnect log site so operators can
// filter and aggregate disconnects without parsing free-form reason
// strings.
//
// The set of values intentionally stays small. Use DisconnectReasonUnknown
// when no other value applies; do not invent ad-hoc strings. Add a new
// constant here (and update its godoc) when a new disconnect class is
// genuinely distinct from the existing ones.
type DisconnectReason string

func (r DisconnectReason) SlogField() slog.Field {
	if r == "" {
		return slog.F("disconnect_reason", "unknown")
	}
	return slog.F("disconnect_reason", r)
}

func (r DisconnectReason) SlogExpectedField() slog.Field {
	return slog.F("disconnect_expected", r.Expected())
}

const (
	// DisconnectReasonUnknown is the zero value. Use it when the disconnect
	// path cannot determine a more specific reason. Treat any disconnect
	// logged with this value as a bug to investigate, not as a normal
	// outcome.
	DisconnectReasonUnknown DisconnectReason = ""

	// DisconnectReasonGraceful indicates the connection ended cleanly: the
	// remote side acknowledged a Disconnect message, an SSH session exited
	// with status 0, or a PTY closed without error. This is the expected
	// "happy path" outcome.
	DisconnectReasonGraceful DisconnectReason = "graceful"

	// DisconnectReasonClientClosed indicates the client side closed the
	// connection without an error (for example, the user closed their
	// terminal or quit their IDE). The session ran to a natural end from
	// the client's perspective.
	DisconnectReasonClientClosed DisconnectReason = "client_closed"

	// DisconnectReasonServerShutdown indicates the workspace agent or
	// coderd is shutting down and is closing connections as part of that
	// shutdown. The connection itself was healthy; the process is going
	// away.
	DisconnectReasonServerShutdown DisconnectReason = "server_shutdown"

	// DisconnectReasonNetworkError indicates the transport failed: an EOF,
	// a read or write error, a context cancellation caused by a timeout,
	// or a similar I/O failure. The connection did not end cleanly.
	DisconnectReasonNetworkError DisconnectReason = "network_error"

	// DisconnectReasonProtocolError indicates a dRPC, SSH, or tailnet
	// protocol violation by the peer. Distinct from network errors because
	// the bytes flowed but the contents were unparsable or unexpected.
	DisconnectReasonProtocolError DisconnectReason = "protocol_error"

	// DisconnectReasonWorkspaceStopped indicates the workspace itself was
	// stopped or deleted while the connection was open, so coderd closed
	// outstanding sessions on its behalf.
	DisconnectReasonWorkspaceStopped DisconnectReason = "workspace_stopped"

	// DisconnectReasonControlPlaneLost indicates the agent or client lost
	// its coordination RPC to coderd. The data plane (peer-to-peer or
	// DERP) may still be functional; this records the control plane
	// outcome specifically.
	DisconnectReasonControlPlaneLost DisconnectReason = "control_plane_lost"
)

// Valid reports whether r is a known DisconnectReason. The zero value
// (DisconnectReasonUnknown) is considered valid since it is the explicit
// "no information" reason.
func (r DisconnectReason) Valid() bool {
	switch r {
	case DisconnectReasonUnknown,
		DisconnectReasonGraceful,
		DisconnectReasonClientClosed,
		DisconnectReasonServerShutdown,
		DisconnectReasonNetworkError,
		DisconnectReasonProtocolError,
		DisconnectReasonWorkspaceStopped,
		DisconnectReasonControlPlaneLost:
		return true
	default:
		return false
	}
}

// Expected reports whether a disconnect with this reason is part of
// normal operation. Operators can use this to split dashboards or alerts
// into "expected" and "investigate" buckets without enumerating every
// reason.
func (r DisconnectReason) Expected() bool {
	switch r {
	case DisconnectReasonGraceful,
		DisconnectReasonClientClosed,
		DisconnectReasonServerShutdown,
		DisconnectReasonWorkspaceStopped:
		return true
	case DisconnectReasonUnknown,
		DisconnectReasonNetworkError,
		DisconnectReasonProtocolError,
		DisconnectReasonControlPlaneLost:
		return false
	default:
		// Unknown reason values are treated as not expected so that
		// new emit sites that forget to classify themselves surface
		// in the "investigate" bucket by default.
		return false
	}
}

// DisconnectInitiator identifies which side caused the disconnect. It
// pairs with DisconnectReason: the reason describes what happened, the
// initiator describes who started it.
type DisconnectInitiator string

const (
	// DisconnectInitiatorUnknown means the disconnect site cannot
	// attribute the close to a specific side. Avoid this where possible.
	DisconnectInitiatorUnknown DisconnectInitiator = ""

	// DisconnectInitiatorClient means the user-facing side (CLI, VS Code
	// extension, JetBrains plugin, Coder Desktop) closed the connection.
	DisconnectInitiatorClient DisconnectInitiator = "client"

	// DisconnectInitiatorAgent means the workspace agent closed the
	// connection.
	DisconnectInitiatorAgent DisconnectInitiator = "agent"

	// DisconnectInitiatorServer means coderd (or a workspace proxy)
	// closed the connection.
	DisconnectInitiatorServer DisconnectInitiator = "server"

	// DisconnectInitiatorNetwork means an underlying network or transport
	// fault caused the close. Neither end deliberately initiated it.
	DisconnectInitiatorNetwork DisconnectInitiator = "network"
)

func (i DisconnectInitiator) SlogField() slog.Field {
	return slog.F("disconnect_initiator", i)
}

// Valid reports whether i is a known DisconnectInitiator.
func (i DisconnectInitiator) Valid() bool {
	switch i {
	case DisconnectInitiatorUnknown,
		DisconnectInitiatorClient,
		DisconnectInitiatorAgent,
		DisconnectInitiatorServer,
		DisconnectInitiatorNetwork:
		return true
	default:
		return false
	}
}

// ConnectionDirection identifies which layer a disconnect log belongs to.
// It tells operators at a glance whether a log is about the control plane
// (server to agent) or the data plane (agent to client).
type ConnectionDirection string

func (d ConnectionDirection) SlogField() slog.Field {
	return slog.F("connect_type", d)
}

const (
	// ConnectionDirectionServerToAgent is the control-plane connection
	// between coderd and the workspace agent (coordination RPC, DERP map
	// subscriber, agent runLoop).
	ConnectionDirectionServerToAgent ConnectionDirection = "server_to_agent"

	// ConnectionDirectionAgentToClient is a data-plane session between
	// the workspace agent and a user's client (SSH, reconnecting PTY,
	// JetBrains port-forwarding).
	ConnectionDirectionAgentToClient ConnectionDirection = "agent_to_client"

	// ConnectionDirectionClientToServer is a connection from a user's
	// client to coderd (e.g. the CLI's WebSocket to the coordinator).
	// Not yet instrumented.
	ConnectionDirectionClientToServer ConnectionDirection = "client_to_server"
)

// ConnectionMethod describes the network path a workspace connection
// took at the moment a disconnect log was emitted. It is intended for
// observability only; do not switch behavior on it.
type ConnectionMethod string

func (m ConnectionMethod) SlogField() slog.Field {
	return slog.F("connection_method", m)
}

const (
	// ConnectionMethodUnknown means the disconnect site does not have
	// the information to determine the connection path.
	ConnectionMethodUnknown ConnectionMethod = ""

	// ConnectionMethodDirect means the peers were communicating over a
	// direct, peer-to-peer connection (NAT-traversed via STUN).
	ConnectionMethodDirect ConnectionMethod = "direct"

	// ConnectionMethodDERP means the peers were communicating through a
	// DERP relay rather than directly.
	ConnectionMethodDERP ConnectionMethod = "derp"
)

// Valid reports whether m is a known ConnectionMethod.
func (m ConnectionMethod) Valid() bool {
	switch m {
	case ConnectionMethodUnknown,
		ConnectionMethodDirect,
		ConnectionMethodDERP:
		return true
	default:
		return false
	}
}
