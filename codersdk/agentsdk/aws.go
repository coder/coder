package agentsdk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

type AWSInstanceIdentityToken struct {
	Signature string `json:"signature" validate:"required"`
	Document  string `json:"document" validate:"required"`
}

// AWSSessionTokenExchanger exchanges AWS instance metadata for a Coder session token.
// @typescript-ignore AWSSessionTokenExchanger
type AWSSessionTokenExchanger struct {
	client *codersdk.Client
}

func WithAWSInstanceIdentity() SessionTokenSetup {
	return func(client *codersdk.Client) RefreshableSessionTokenProvider {
		return &InstanceIdentitySessionTokenProvider{
			TokenExchanger: &AWSSessionTokenExchanger{client: client},
		}
	}
}

// exchange uses the Amazon Metadata API to fetch a signed payload, and exchange it for a session token for a workspace
// agent.
//
// The requesting instance must be registered as a resource in the latest history for a workspace.
func (a *AWSSessionTokenExchanger) exchange(ctx context.Context) (AuthenticateResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://169.254.169.254/latest/api/token", nil)
	if err != nil {
		return AuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")
	res, err := a.client.HTTPClient.Do(req)
	if err != nil {
		return AuthenticateResponse{}, err
	}
	defer res.Body.Close()
	token, err := io.ReadAll(res.Body)
	if err != nil {
		return AuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/signature", nil)
	if err != nil {
		return AuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	res, err = a.client.HTTPClient.Do(req)
	if err != nil {
		return AuthenticateResponse{}, err
	}
	defer res.Body.Close()
	signature, err := io.ReadAll(res.Body)
	if err != nil {
		return AuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://169.254.169.254/latest/dynamic/instance-identity/document", nil)
	if err != nil {
		return AuthenticateResponse{}, nil
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	res, err = a.client.HTTPClient.Do(req)
	if err != nil {
		return AuthenticateResponse{}, err
	}
	defer res.Body.Close()
	document, err := io.ReadAll(res.Body)
	if err != nil {
		return AuthenticateResponse{}, xerrors.Errorf("read token: %w", err)
	}

	// request without the token to avoid re-entering this function
	res, err = a.client.RequestWithoutSessionToken(ctx, http.MethodPost, "/api/v2/workspaceagents/aws-instance-identity", AWSInstanceIdentityToken{
		Signature: string(signature),
		Document:  string(document),
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
