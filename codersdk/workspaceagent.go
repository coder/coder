package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"cloud.google.com/go/compute/metadata"
	"github.com/coder/coder/coderd"
	"golang.org/x/xerrors"
)

func (c *Client) WorkspaceAgentAuthenticateGoogleInstanceIdentity(ctx context.Context, serviceAccount string, gcpClient *metadata.Client) (coderd.WorkspaceAgentAuthenticateResponse, error) {
	if serviceAccount == "" {
		serviceAccount = "default"
	}
	if gcpClient == nil {
		gcpClient = metadata.NewClient(c.httpClient)
	}
	jwt, err := gcpClient.Get(fmt.Sprintf("instance/service-accounts/%s/identity?audience=coder&format=full", serviceAccount))
	if err != nil {
		return coderd.WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("get metadata identity: %w", err)
	}
	res, err := c.request(ctx, http.MethodPost, "/api/v2/workspaceagent/authenticate/google-instance-identity", coderd.WorkspaceAgentAuthenticateGoogleInstanceIdentityRequest{
		JSONWebToken: jwt,
	})
	if err != nil {
		return coderd.WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.WorkspaceAgentAuthenticateResponse{}, readBodyAsError(res)
	}
	var resp coderd.WorkspaceAgentAuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
