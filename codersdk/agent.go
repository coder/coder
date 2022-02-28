package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd"
)

// AuthenticateAgentWithGoogleCloudIdentity uses the Google Compute Engine Metadata API to
// fetch a signed JWT, and exchange it for a session token for a workspace agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (c *Client) AuthenticateAgentWithGoogleCloudIdentity(ctx context.Context, serviceAccount string, gcpClient *metadata.Client) (coderd.AgentAuthResponse, error) {
	if serviceAccount == "" {
		// This is the default name specified by Google.
		serviceAccount = "default"
	}
	if gcpClient == nil {
		gcpClient = metadata.NewClient(c.httpClient)
	}
	// "format=full" is required, otherwise the responding payload will be missing "instance_id".
	jwt, err := gcpClient.Get(fmt.Sprintf("instance/service-accounts/%s/identity?audience=coder&format=full", serviceAccount))
	if err != nil {
		return coderd.AgentAuthResponse{}, xerrors.Errorf("get metadata identity: %w", err)
	}
	res, err := c.request(ctx, http.MethodPost, "/api/v2/agent/authenticate/google-instance-identity", coderd.GoogleInstanceIdentityToken{
		JSONWebToken: jwt,
	})
	if err != nil {
		return coderd.AgentAuthResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.AgentAuthResponse{}, readBodyAsError(res)
	}
	var resp coderd.AgentAuthResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
