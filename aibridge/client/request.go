package client

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"golang.org/x/net/http/httpguts"
	"golang.org/x/xerrors"

	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
)

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
func ExtractAgentFirewallHeaders(r *http.Request) (sessionID *string, seqNumber *int32, err error) {
	rawSessionID := r.Header.Get(agplaibridge.HeaderAgentFirewallSessionID)
	rawSeqNumber := r.Header.Get(agplaibridge.HeaderAgentFirewallSequenceNumber)

	hasSessionID := rawSessionID != ""
	hasSeqNumber := rawSeqNumber != ""

	switch {
	case !hasSessionID && !hasSeqNumber:
		// Neither header present; request did not traverse Agent Firewall.
		return nil, nil, nil
	case hasSessionID && !hasSeqNumber:
		return nil, nil, xerrors.Errorf("agent firewall session ID header present without sequence number")
	case !hasSessionID && hasSeqNumber:
		return nil, nil, xerrors.Errorf("agent firewall sequence number header present without session ID")
	}

	// Both headers present; validate the session ID is a UUID. Storing an
	// invalid value would silently drop the firewall correlation to NULL
	// downstream, so reject it here instead.
	if _, parseErr := uuid.Parse(rawSessionID); parseErr != nil {
		return nil, nil, xerrors.Errorf("invalid agent firewall session ID %q: %w", rawSessionID, parseErr)
	}

	// Parse the sequence number.
	n, err := strconv.ParseInt(rawSeqNumber, 10, 32)
	if err != nil {
		return nil, nil, xerrors.Errorf("invalid agent firewall sequence number %q: %w", rawSeqNumber, err)
	}
	if n < 0 {
		return nil, nil, xerrors.Errorf("invalid agent firewall sequence number %q: must be non-negative", rawSeqNumber)
	}

	n32 := int32(n)
	return &rawSessionID, &n32, nil
}
