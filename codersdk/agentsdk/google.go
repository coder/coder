package agentsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

type GoogleInstanceIdentityToken struct {
	JSONWebToken string `json:"json_web_token" validate:"required"`
}

// GoogleSessionTokenExchanger exchanges a Google instance JWT document for a Coder session token.
// @typescript-ignore GoogleSessionTokenExchanger
type GoogleSessionTokenExchanger struct {
	serviceAccount string
	gcpClient      *metadata.Client
	client         *codersdk.Client
}

func WithGoogleInstanceIdentity(serviceAccount string, gcpClient *metadata.Client) SessionTokenSetup {
	return func(client *codersdk.Client) RefreshableSessionTokenProvider {
		return &InstanceIdentitySessionTokenProvider{
			TokenExchanger: &GoogleSessionTokenExchanger{
				client:         client,
				gcpClient:      gcpClient,
				serviceAccount: serviceAccount,
			},
		}
	}
}

// exchange uses the Google Compute Engine Metadata API to fetch a signed JWT, and exchange it for a session token for a
// workspace agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (g *GoogleSessionTokenExchanger) exchange(ctx context.Context) (AuthenticateResponse, error) {
	if g.serviceAccount == "" {
		// This is the default name specified by Google.
		g.serviceAccount = "default"
	}
	gcpClient := metadata.NewClient(g.client.HTTPClient)
	if g.gcpClient != nil {
		gcpClient = g.gcpClient
	}

	// "format=full" is required, otherwise the responding payload will be missing "instance_id".
	jwt, err := gcpClient.Get(fmt.Sprintf("instance/service-accounts/%s/identity?audience=coder&format=full", g.serviceAccount))
	if err != nil {
		return AuthenticateResponse{}, xerrors.Errorf("get metadata identity: %w", err)
	}
	// request without the token to avoid re-entering this function
	res, err := g.client.RequestWithoutSessionToken(ctx, http.MethodPost, "/api/v2/workspaceagents/google-instance-identity", GoogleInstanceIdentityToken{
		JSONWebToken: jwt,
	})
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
