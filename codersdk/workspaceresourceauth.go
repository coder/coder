package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/xerrors"
)

type GoogleInstanceIdentityToken struct {
	JSONWebToken string `json:"json_web_token" validate:"required"`
}

type AWSInstanceIdentityToken struct {
	Signature string `json:"signature" validate:"required"`
	Document  string `json:"document" validate:"required"`
}

// WorkspaceAgentAuthenticateResponse is returned when an instance ID
// has been exchanged for a session token.
type WorkspaceAgentAuthenticateResponse struct {
	SessionToken string `json:"session_token"`
}

// AuthWorkspaceGoogleInstanceIdentity uses the Google Compute Engine Metadata API to
// fetch a signed JWT, and exchange it for a session token for a workspace agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (c *Client) AuthWorkspaceGoogleInstanceIdentity(ctx context.Context, serviceAccount string, gcpClient *metadata.Client) (WorkspaceAgentAuthenticateResponse, error) {
	if serviceAccount == "" {
		// This is the default name specified by Google.
		serviceAccount = "default"
	}
	if gcpClient == nil {
		gcpClient = metadata.NewClient(c.HTTPClient)
	}
	// "format=full" is required, otherwise the responding payload will be missing "instance_id".
	jwt, err := gcpClient.Get(fmt.Sprintf("instance/service-accounts/%s/identity?audience=coder&format=full", serviceAccount))
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("get metadata identity: %w", err)
	}
	res, err := c.request(ctx, http.MethodPost, "/api/v2/workspaceresources/auth/google-instance-identity", GoogleInstanceIdentityToken{
		JSONWebToken: jwt,
	})
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentAuthenticateResponse{}, readBodyAsError(res)
	}
	var resp WorkspaceAgentAuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// AuthWorkspaceAWSInstanceIdentity uses the Amazon Metadata API to
// fetch a signed payload, and exchange it for a session token for a workspace agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (c *Client) AuthWorkspaceAWSInstanceIdentity(ctx context.Context) (WorkspaceAgentAuthenticateResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://169.254.169.254/latest/api/token", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	token, err := io.ReadAll(res.Body)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/signature", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	res, err = c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	signature, err := io.ReadAll(res.Body)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/document", nil)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	res, err = c.HTTPClient.Do(req)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	document, err := io.ReadAll(res.Body)
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	res, err = c.request(ctx, http.MethodPost, "/api/v2/workspaceresources/auth/aws-instance-identity", AWSInstanceIdentityToken{
		Signature: string(signature),
		Document:  string(document),
	})
	if err != nil {
		return WorkspaceAgentAuthenticateResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceAgentAuthenticateResponse{}, readBodyAsError(res)
	}
	var resp WorkspaceAgentAuthenticateResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
