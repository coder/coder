package workspacesdk

import (
	"context"
	"net/http"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// ACPHarness is the public discovery result. Commands stay on the agent.
type ACPHarness struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Error       string `json:"error,omitempty"`
}

// ListACPHarnesses returns the cached agent-local harness catalog.
func (c *agentConn) ListACPHarnesses(ctx context.Context) ([]ACPHarness, error) {
	var result []ACPHarness
	res, err := c.apiRequest(ctx, http.MethodGet, "/api/v0/acp/harnesses", nil)
	if err != nil {
		return nil, xerrors.Errorf("list ACP harnesses: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, codersdk.ReadBodyAsError(res)
	}
	return result, decodeAgentJSON(res, &result)
}
