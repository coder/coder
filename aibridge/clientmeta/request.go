package clientmeta

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

// ExtractAgentFirewallHeaders validates and returns Agent Firewall correlation
// headers. Both must be present and valid, or both absent.
func ExtractAgentFirewallHeaders(r *http.Request) (*string, *int32, error) {
	rawSessionID := r.Header.Get(agplaibridge.HeaderAgentFirewallSessionID)
	rawSeqNumber := r.Header.Get(agplaibridge.HeaderAgentFirewallSequenceNumber)

	hasSessionID := rawSessionID != ""
	hasSeqNumber := rawSeqNumber != ""
	switch {
	case !hasSessionID && !hasSeqNumber:
		return nil, nil, nil
	case hasSessionID && !hasSeqNumber:
		return nil, nil, xerrors.New("agent firewall session ID header present without sequence number")
	case !hasSessionID && hasSeqNumber:
		return nil, nil, xerrors.New("agent firewall sequence number header present without session ID")
	}

	if _, err := uuid.Parse(rawSessionID); err != nil {
		return nil, nil, xerrors.Errorf("invalid agent firewall session ID %q: %w", rawSessionID, err)
	}
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
