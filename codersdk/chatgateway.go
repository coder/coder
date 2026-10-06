package codersdk

import (
	"strconv"
	"strings"

	"golang.org/x/net/http/httpguts"
	"golang.org/x/xerrors"
)

// ValidateAIGateway checks standalone Agents routing configuration without
// contacting the endpoint, which may be temporarily unavailable during startup.
func (c ChatConfig) ValidateAIGateway() error {
	configured := c.AIGatewayURL.String() != ""
	key := c.AIGatewayKey.Value()
	if configured != (key != "") {
		return xerrors.New("--chat-ai-gateway-url and --chat-ai-gateway-key must be configured together")
	}
	if !configured {
		return nil
	}
	u := c.AIGatewayURL.Value()
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Opaque != "" {
		return xerrors.New("--chat-ai-gateway-url must be an absolute HTTP(S) URL with a host")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return xerrors.New("--chat-ai-gateway-url must not contain credentials, a query, or a fragment")
	}
	if u.Port() != "" {
		port, err := strconv.ParseUint(u.Port(), 10, 16)
		if err != nil || port == 0 {
			return xerrors.New("--chat-ai-gateway-url port must be between 1 and 65535")
		}
	}
	if strings.TrimSpace(key) != key || !httpguts.ValidHeaderFieldValue(key) {
		return xerrors.New("--chat-ai-gateway-key must be a non-empty Gateway key without surrounding whitespace or invalid header characters")
	}
	return nil
}
