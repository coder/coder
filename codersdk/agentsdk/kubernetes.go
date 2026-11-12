package agentsdk

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// DefaultKubernetesPodTokenPath is where a pod's template mounts the token
// that identifies the pod to Coder: a projected service account token with
// Coder as its audience. The kubelet obtains it for the pod and keeps it
// fresh.
const DefaultKubernetesPodTokenPath = "/var/run/secrets/coder.com/identity/token" //nolint:gosec // A path, not a credential.

// kubernetesPodIdentityWait is how long coderd is asked to hold a request
// from a pod that no workspace has claimed yet.
const kubernetesPodIdentityWait = 25 * time.Second

// KubernetesPodIdentityToken is what a workspace daemon in a Kubernetes pod
// sends to prove which pod it is.
type KubernetesPodIdentityToken struct {
	// Token is the pod's projected service account token.
	Token string `json:"token" validate:"required"`
	// WaitSeconds asks coderd to hold the request for up to this long when
	// no workspace has claimed the pod yet. A pod from a warm pool runs
	// its daemon before it belongs to anybody.
	WaitSeconds int `json:"wait_seconds,omitempty"`
	// AgentName optionally selects a specific agent when multiple
	// agents share the same instance identity. An empty string is
	// treated as unspecified.
	AgentName string `json:"agent_name,omitempty"`
}

// KubernetesSessionTokenExchanger exchanges a pod's token for a Coder
// session token.
// @typescript-ignore KubernetesSessionTokenExchanger
type KubernetesSessionTokenExchanger struct {
	client    *codersdk.Client
	tokenFile string
	agentName string
}

// WithKubernetesPodIdentity authenticates a workspace daemon as the pod it
// runs in. An empty tokenFile uses DefaultKubernetesPodTokenPath.
func WithKubernetesPodIdentity(tokenFile string, opts ...InstanceIdentityOption) SessionTokenSetup {
	cfg := applyInstanceIdentityOptions(opts)
	if tokenFile == "" {
		tokenFile = DefaultKubernetesPodTokenPath
	}
	return func(client *codersdk.Client) RefreshableSessionTokenProvider {
		return &InstanceIdentitySessionTokenProvider{
			TokenExchanger: &KubernetesSessionTokenExchanger{client: client, tokenFile: tokenFile, agentName: cfg.AgentName},
		}
	}
}

// exchange presents the pod's token until a workspace has claimed the pod.
// The pod must be registered as a resource in the latest build of a
// workspace.
func (k *KubernetesSessionTokenExchanger) exchange(ctx context.Context) (AuthenticateResponse, error) {
	for {
		started := time.Now()
		// The kubelet replaces the file before the token expires.
		raw, err := os.ReadFile(k.tokenFile)
		if err != nil {
			return AuthenticateResponse{}, xerrors.Errorf("read pod token: %w", err)
		}
		resp, claimed, err := k.exchangeOnce(ctx, strings.TrimSpace(string(raw)))
		if claimed && err == nil {
			return resp, nil
		}
		// A refusal is final. Anything else, such as coderd restarting,
		// is waited out here: the daemon's own retry backs off to many
		// seconds, which a pod that waits to be claimed cannot afford.
		var refused *codersdk.Error
		if err != nil && (ctx.Err() != nil || (xerrors.As(err, &refused) &&
			(refused.StatusCode() < http.StatusInternalServerError || refused.StatusCode() == http.StatusNotImplemented))) {
			return AuthenticateResponse{}, err
		}
		// Not claimed yet. coderd normally holds the request, so asking
		// again at once is the wait. Do not spin if it answered early.
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 3*time.Second {
			return AuthenticateResponse{}, xerrors.New("no workspace has claimed this pod yet")
		}
		if rest := time.Second - time.Since(started); rest > 0 {
			select {
			case <-ctx.Done():
				return AuthenticateResponse{}, ctx.Err()
			case <-time.After(rest):
			}
		}
	}
}

func (k *KubernetesSessionTokenExchanger) exchangeOnce(ctx context.Context, token string) (AuthenticateResponse, bool, error) {
	wait := kubernetesPodIdentityWait
	if deadline, ok := ctx.Deadline(); ok {
		// The caller cannot wait for a claim that long. Ask coderd to
		// answer in time.
		wait = max(0, min(wait, time.Until(deadline)-2*time.Second))
	}
	ctx, cancel := context.WithTimeout(ctx, wait+15*time.Second)
	defer cancel()
	// request without the token to avoid re-entering this function
	res, err := k.client.RequestWithoutSessionToken(ctx, http.MethodPost, "/api/v2/workspaceagents/kubernetes-pod-identity", KubernetesPodIdentityToken{
		Token:       token,
		WaitSeconds: int(wait.Seconds()),
		AgentName:   k.agentName,
	})
	if err != nil {
		return AuthenticateResponse{}, false, err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
		var resp AuthenticateResponse
		return resp, true, codersdk.ReadBodyAsJSON(res, &resp)
	case http.StatusNotFound:
		// The cluster confirmed the pod, and no workspace has claimed it.
		return AuthenticateResponse{}, false, nil
	default:
		return AuthenticateResponse{}, false, codersdk.ReadBodyAsError(res)
	}
}
