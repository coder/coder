package clientmeta

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"golang.org/x/net/http/httpguts"
	"golang.org/x/xerrors"

	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
)

// HasConnectionUpgrade reports whether the Connection header requests a
// protocol switch.
func HasConnectionUpgrade(r *http.Request) bool {
	return httpguts.HeaderValuesContainsToken(r.Header.Values("Connection"), "upgrade")
}

// IsWebSocketUpgrade reports whether r is a WebSocket opening handshake.
func IsWebSocketUpgrade(r *http.Request) bool {
	return r.Method == http.MethodGet &&
		httpguts.HeaderValuesContainsToken(r.Header.Values("Connection"), "upgrade") &&
		httpguts.HeaderValuesContainsToken(r.Header.Values("Upgrade"), "websocket")
}

// ExtractAgentFirewallHeaders reads and parses the Agent Firewall
// correlation headers from the request. Both headers must be present
// together with a valid UUID session ID and a non-negative int32
// sequence number, or both must be absent. Partial or malformed headers
// return an error so the caller can reject the request (fail closed).
func ExtractAgentFirewallHeaders(r *http.Request) (*string, *int32, error) {
	rawSessionID := r.Header.Get(agplaibridge.HeaderAgentFirewallSessionID)
	rawSeqNumber := r.Header.Get(agplaibridge.HeaderAgentFirewallSequenceNumber)

	hasSessionID := rawSessionID != ""
	hasSeqNumber := rawSeqNumber != ""
	switch {
	case !hasSessionID && !hasSeqNumber:
		// Neither header present; request did not traverse Agent Firewall.
		return nil, nil, nil
	case hasSessionID && !hasSeqNumber:
		return nil, nil, xerrors.New("agent firewall session ID header present without sequence number")
	case !hasSessionID && hasSeqNumber:
		return nil, nil, xerrors.New("agent firewall sequence number header present without session ID")
	}

	// Both headers present; validate the session ID is a UUID. Storing an
	// invalid value would silently drop the firewall correlation to NULL
	// downstream, so reject it here instead.
	if _, err := uuid.Parse(rawSessionID); err != nil {
		return nil, nil, xerrors.New("agent firewall session ID must be a UUID")
	}
	n, err := strconv.ParseInt(rawSeqNumber, 10, 32)
	if err != nil {
		return nil, nil, xerrors.New("agent firewall sequence number must be a base-10 int32")
	}
	if n < 0 {
		return nil, nil, xerrors.New("agent firewall sequence number must be non-negative")
	}
	n32 := int32(n)
	return &rawSessionID, &n32, nil
}
