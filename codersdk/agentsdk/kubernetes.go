package agentsdk

import (
	"context"
	"net/http"
	"os"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

// DefaultKubernetesPodTokenPath is where a workload operator mounts the
// pod's projected service account token.
const DefaultKubernetesPodTokenPath = "/var/run/secrets/coder.com/identity/token" //nolint:gosec // A path, not a credential.

// KubernetesPodTokenProvider authenticates a daemon with its pod's projected
// service account token. The kubelet rotates the file, so it is read for
// every request and never cached.
// @typescript-ignore KubernetesPodTokenProvider
type KubernetesPodTokenProvider struct {
	Path string
}

// WithKubernetesPodToken authenticates as the pod the daemon runs in. No
// Coder-issued credential is involved.
func WithKubernetesPodToken(path string) SessionTokenSetup {
	if path == "" {
		path = DefaultKubernetesPodTokenPath
	}
	return func(_ *codersdk.Client) RefreshableSessionTokenProvider {
		return &KubernetesPodTokenProvider{Path: path}
	}
}

func (k *KubernetesPodTokenProvider) AsRequestOption() codersdk.RequestOption {
	token := k.GetSessionToken()
	return func(req *http.Request) {
		req.Header.Set(codersdk.SessionTokenHeader, token)
	}
}

func (k *KubernetesPodTokenProvider) SetDialOption(opts *websocket.DialOptions) {
	token := k.GetSessionToken()
	if opts.HTTPHeader == nil {
		opts.HTTPHeader = http.Header{}
	}
	if opts.HTTPHeader.Get(codersdk.SessionTokenHeader) == "" {
		opts.HTTPHeader.Set(codersdk.SessionTokenHeader, token)
	}
}

// GetSessionToken returns the current pod token, or an empty string when the
// file cannot be read.
func (k *KubernetesPodTokenProvider) GetSessionToken() string {
	raw, err := os.ReadFile(k.Path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// RefreshToken checks that the pod token is readable. The kubelet handles
// rotation.
func (k *KubernetesPodTokenProvider) RefreshToken(_ context.Context) error {
	if _, err := os.Stat(k.Path); err != nil {
		return xerrors.Errorf("stat pod token: %w", err)
	}
	return nil
}

// Identity returns the workload identity coderd derived for this daemon.
func (c *Client) Identity(ctx context.Context) (codersdk.WorkloadIdentity, error) {
	res, err := c.SDK.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/identity", nil)
	if err != nil {
		return codersdk.WorkloadIdentity{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return codersdk.WorkloadIdentity{}, codersdk.ReadBodyAsError(res)
	}
	var identity codersdk.WorkloadIdentity
	return identity, codersdk.ReadBodyAsJSON(res, &identity)
}
