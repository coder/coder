package workspacesdk

import (
	"context"
	"net/http"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// ACPConfigValue is a selectable value advertised by a harness.
type ACPConfigValue struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Group       string `json:"group,omitempty"`
	GroupName   string `json:"group_name,omitempty"`
}

// ACPConfigOption describes an ACP session select option.
type ACPConfigOption struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description,omitempty"`
	CurrentValue string           `json:"current_value"`
	Category     string           `json:"category,omitempty"`
	Values       []ACPConfigValue `json:"values"`
}

// ACPHarness is the public discovery result. Commands stay on the agent.
type ACPHarness struct {
	Slug          string            `json:"slug"`
	DisplayName   string            `json:"display_name"`
	LoadSession   bool              `json:"load_session"`
	ResumeSession bool              `json:"resume_session"`
	Steering      bool              `json:"steering"`
	ConfigOptions []ACPConfigOption `json:"config_options"`
	Error         string            `json:"error,omitempty"`
}

func acpRequest[T any](ctx context.Context, c *agentConn, method, path string, body any) (T, error) {
	var result T
	res, err := c.apiRequest(ctx, method, "/api/v0/acp"+path, body)
	if err != nil {
		return result, xerrors.Errorf("request ACP: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusAccepted {
		return result, codersdk.ReadBodyAsError(res)
	}
	return result, decodeAgentJSON(res, &result)
}

// ListACPHarnesses returns the cached agent-local harness catalog.
func (c *agentConn) ListACPHarnesses(ctx context.Context) ([]ACPHarness, error) {
	return acpRequest[[]ACPHarness](ctx, c, http.MethodGet, "/harnesses", nil)
}
