package agentsdk

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coder/coder/v2/codersdk"
)

type AzureInstanceIdentityToken struct {
	Signature string `json:"signature" validate:"required"`
	Encoding  string `json:"encoding" validate:"required"`
	// AgentName optionally selects a specific agent when multiple
	// agents share the same instance identity. An empty string is
	// treated as unspecified.
	AgentName string `json:"agent_name,omitempty"`
}

// AzureSessionTokenExchanger exchanges Azure attested metadata for a Coder session token.
// @typescript-ignore AzureSessionTokenExchanger
type AzureSessionTokenExchanger struct {
	client    *codersdk.Client
	agentName string
}

func WithAzureInstanceIdentity(opts ...InstanceIdentityOption) SessionTokenSetup {
	cfg := applyInstanceIdentityOptions(opts)
	return func(client *codersdk.Client) RefreshableSessionTokenProvider {
		return &InstanceIdentitySessionTokenProvider{
			TokenExchanger: &AzureSessionTokenExchanger{client: client, agentName: cfg.AgentName},
		}
	}
}

// AuthWorkspaceAzureInstanceIdentity uses the Azure Instance Metadata Service to
// fetch a signed payload, and exchange it for a session token for a workspace agent.
func (a *AzureSessionTokenExchanger) exchange(ctx context.Context) (AuthenticateResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/metadata/attested/document?api-version=2020-09-01", nil)
	if err != nil {
		return AuthenticateResponse{}, nil
	}
	req.Header.Set("Metadata", "true")
	res, err := a.client.HTTPClient.Do(req)
	if err != nil {
		return AuthenticateResponse{}, err
	}
	defer res.Body.Close()

	var token AzureInstanceIdentityToken
	err = json.NewDecoder(res.Body).Decode(&token)
	if err != nil {
		return AuthenticateResponse{}, err
	}
	token.AgentName = a.agentName

	res, err = a.client.RequestWithoutSessionToken(ctx, http.MethodPost, "/api/v2/workspaceagents/azure-instance-identity", token)
	if err != nil {
		return AuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AuthenticateResponse{}, codersdk.ReadBodyAsError(res)
	}
	var resp AuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
