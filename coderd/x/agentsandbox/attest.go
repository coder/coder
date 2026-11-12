package agentsandbox

import (
	"context"
	"crypto/sha256"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"
)

// InstanceIDPrefix starts the instance ID of a workspace daemon whose
// identity is a Kubernetes pod. The pod's UID follows.
const InstanceIDPrefix = "k8s-pod:"

// ClaimedChannel is where it is announced that a workspace build registered
// a pod as a resource, so that the pod's daemon need not ask again and
// again whether anybody has claimed it.
func ClaimedChannel(instanceID string) string {
	return "k8s_pod_claimed:" + strings.TrimPrefix(instanceID, InstanceIDPrefix)
}

// PodIdentity is what a cluster says about the bearer of a pod's token.
type PodIdentity struct {
	Namespace      string
	ServiceAccount string
	PodName        string
	PodUID         string
	NodeName       string
}

// InstanceID is the instance ID of the workspace daemon in the pod.
func (p PodIdentity) InstanceID() string {
	return InstanceIDPrefix + p.PodUID
}

// reviewCacheTTL bounds how long the cluster's answer about a token is
// reused. A daemon that waits to be claimed asks again and again with the
// same token.
const reviewCacheTTL = 30 * time.Second

// Attestor asks a cluster whose token something is.
type Attestor struct {
	kube *KubeClient
	// audience is what tokens must have been issued for. A token for the
	// Kubernetes API itself, or for another service, is refused.
	audience string

	mu    sync.Mutex
	cache map[[sha256.Size]byte]cachedReview
}

type cachedReview struct {
	identity PodIdentity
	expires  time.Time
}

// NewAttestor returns an attestor for tokens with the given audience.
func NewAttestor(kube *KubeClient, audience string) *Attestor {
	return &Attestor{kube: kube, audience: audience, cache: make(map[[sha256.Size]byte]cachedReview)}
}

// Audience is the audience pod tokens must be issued for.
func (a *Attestor) Audience() string {
	return a.audience
}

type tokenReview struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Spec       struct {
		Token     string   `json:"token"`
		Audiences []string `json:"audiences"`
	} `json:"spec"`
	Status struct {
		Authenticated bool     `json:"authenticated"`
		Audiences     []string `json:"audiences"`
		Error         string   `json:"error"`
		User          struct {
			Username string              `json:"username"`
			Extra    map[string][]string `json:"extra"`
		} `json:"user"`
	} `json:"status"`
}

// ErrNotAttested means the cluster did not confirm the token as a pod's.
var ErrNotAttested = xerrors.New("the cluster did not confirm the token as a pod's")

// Attest returns the pod a token was issued to. The cluster checks the
// signature, the expiry and the audience, and that the pod still exists.
func (a *Attestor) Attest(ctx context.Context, token string) (PodIdentity, error) {
	key := sha256.Sum256([]byte(token))
	now := time.Now()
	a.mu.Lock()
	for k, cached := range a.cache {
		if now.After(cached.expires) {
			delete(a.cache, k)
		}
	}
	cached, ok := a.cache[key]
	a.mu.Unlock()
	if ok {
		return cached.identity, nil
	}

	var review tokenReview
	review.APIVersion, review.Kind = "authentication.k8s.io/v1", "TokenReview"
	review.Spec.Token, review.Spec.Audiences = token, []string{a.audience}
	if err := a.kube.Do(ctx, http.MethodPost, "/apis/authentication.k8s.io/v1/tokenreviews", review, &review); err != nil {
		return PodIdentity{}, xerrors.Errorf("review token: %w", err)
	}
	if !review.Status.Authenticated {
		return PodIdentity{}, xerrors.Errorf("%w: %s", ErrNotAttested, review.Status.Error)
	}
	// A token that is not bound to a pod is some other credential of the
	// service account, for example one made with `kubectl create token`.
	extra := func(key string) string {
		if values := review.Status.User.Extra["authentication.kubernetes.io/"+key]; len(values) == 1 {
			return values[0]
		}
		return ""
	}
	identity := PodIdentity{PodName: extra("pod-name"), PodUID: extra("pod-uid"), NodeName: extra("node-name")}
	if identity.PodUID == "" || identity.PodName == "" {
		return PodIdentity{}, xerrors.Errorf("%w: the token is not bound to a pod", ErrNotAttested)
	}
	// system:serviceaccount:<namespace>:<name>
	identity.Namespace, identity.ServiceAccount, ok = parseServiceAccountUsername(review.Status.User.Username)
	if !ok {
		return PodIdentity{}, xerrors.Errorf("%w: %q is not a service account", ErrNotAttested, review.Status.User.Username)
	}

	a.mu.Lock()
	a.cache[key] = cachedReview{identity: identity, expires: now.Add(reviewCacheTTL)}
	a.mu.Unlock()
	return identity, nil
}

func parseServiceAccountUsername(username string) (namespace, name string, ok bool) {
	rest, ok := strings.CutPrefix(username, "system:serviceaccount:")
	if !ok {
		return "", "", false
	}
	namespace, name, ok = strings.Cut(rest, ":")
	return namespace, name, ok && namespace != "" && name != ""
}
