// Package podidentity lets a workspace daemon prove which Kubernetes pod it
// runs in. The daemon presents the token the kubelet obtained for its pod.
// coderd checks the token against the signing keys of the pod's cluster,
// which an operator in that cluster registered with coderd. coderd itself
// never talks to a cluster and holds no credential for one.
//
// A template registers the pod's UID as the instance ID of the workspace's
// agent, with coder_agent_instance, so no credential of Coder's is in the
// pod's spec.
package podidentity

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/xerrors"
)

const (
	// InstanceIDPrefix starts the instance ID of a workspace daemon whose
	// identity is a Kubernetes pod. The pod's UID follows.
	InstanceIDPrefix = "k8s-pod:"
	// IssuerTTL is how long a cluster's registration is believed without
	// its operator renewing it. An operator that is gone can no longer say
	// that the cluster's keys are what they were.
	IssuerTTL = 2 * time.Minute
	// IssuerChannel is where coderd replicas tell each other about a
	// cluster that an operator registered.
	IssuerChannel = "pod_identity_issuer"
	// tokenLeeway tolerates clock skew between coderd and a cluster.
	tokenLeeway = 30 * time.Second
)

// tokenAlgorithms are the signature algorithms Kubernetes API servers use
// for service account tokens.
var tokenAlgorithms = []jose.SignatureAlgorithm{
	jose.RS256, jose.RS384, jose.RS512,
	jose.ES256, jose.ES384, jose.ES512,
	jose.PS256, jose.PS384, jose.PS512,
}

// ErrNotAttested means no registered cluster confirms the token as a pod's.
var ErrNotAttested = xerrors.New("no registered cluster confirms the token as a pod's")

// RegisteredChannel is where it is announced that a workspace build
// registered a pod as a resource. A pod's daemon starts before the build
// that makes the pod is over, and waits for this.
func RegisteredChannel(instanceID string) string {
	return "k8s_pod_registered:" + strings.TrimPrefix(instanceID, InstanceIDPrefix)
}

// Issuer is a cluster that issues pod tokens, as its operator described it.
type Issuer struct {
	// Name is the operator's name for the cluster.
	Name string `json:"name"`
	// URL is the issuer of the cluster's tokens. Clusters often share one.
	URL string `json:"url"`
	// Keys are the public keys the cluster signs its tokens with.
	Keys jose.JSONWebKeySet `json:"keys"`
	// Namespaces are the namespaces whose pods may be workspace daemons.
	Namespaces []string `json:"namespaces"`
	// SeenAt is when the operator last registered the cluster.
	SeenAt time.Time `json:"seen_at"`
}

// Pod is what a cluster's keys confirm about the bearer of a pod's token.
type Pod struct {
	// Issuer is the name of the registered cluster that vouches.
	Issuer         string
	Namespace      string
	ServiceAccount string
	PodName        string
	PodUID         string
}

// InstanceID is the instance ID of the agent that runs in the pod.
func (p Pod) InstanceID() string {
	return InstanceIDPrefix + p.PodUID
}

// Registry holds the clusters whose pods coderd believes.
type Registry struct {
	// audience is what a pod's token must be issued for.
	audience string

	mu      sync.Mutex
	issuers map[string]Issuer
}

// NewRegistry returns a registry that accepts tokens issued for audience.
func NewRegistry(audience string) *Registry {
	return &Registry{audience: audience, issuers: make(map[string]Issuer)}
}

// Audience is what pod tokens must be issued for.
func (r *Registry) Audience() string {
	return r.audience
}

// Put registers a cluster, or renews its registration.
func (r *Registry) Put(issuer Issuer) error {
	if issuer.Name == "" || issuer.URL == "" {
		return xerrors.New("a cluster needs a name and the issuer of its tokens")
	}
	if len(issuer.Namespaces) == 0 || slices.Contains(issuer.Namespaces, "") {
		return xerrors.New("a cluster needs the namespaces whose pods may be workspace daemons")
	}
	keys := jose.JSONWebKeySet{}
	for _, key := range issuer.Keys.Keys {
		// Only what a token can be checked with is kept.
		if key.IsPublic() && key.Valid() {
			keys.Keys = append(keys.Keys, key)
		}
	}
	if len(keys.Keys) == 0 {
		return xerrors.New("a cluster needs at least one public signing key")
	}
	issuer.Keys = keys
	r.mu.Lock()
	defer r.mu.Unlock()
	r.issuers[issuer.Name] = issuer
	return nil
}

// Issuers returns the clusters whose registration is current at now, by
// name.
func (r *Registry) Issuers(now time.Time) []Issuer {
	r.mu.Lock()
	defer r.mu.Unlock()
	live := make([]Issuer, 0, len(r.issuers))
	for name, issuer := range r.issuers {
		if now.Sub(issuer.SeenAt) > IssuerTTL {
			delete(r.issuers, name)
			continue
		}
		live = append(live, issuer)
	}
	sort.Slice(live, func(i, j int) bool { return live[i].Name < live[j].Name })
	return live
}

type kubernetesClaims struct {
	Kubernetes struct {
		Namespace string `json:"namespace"`
		Pod       struct {
			Name string `json:"name"`
			UID  string `json:"uid"`
		} `json:"pod"`
		ServiceAccount struct {
			Name string `json:"name"`
		} `json:"serviceaccount"`
	} `json:"kubernetes.io"`
}

// Attest returns the pod a token was issued to. The token must be signed
// by a registered cluster, be issued for coderd, be unexpired, and be bound
// to a pod in a namespace the cluster's operator named. Nothing the token
// says is believed before its signature is.
func (r *Registry) Attest(raw string, now time.Time) (Pod, error) {
	token, err := jwt.ParseSigned(raw, tokenAlgorithms)
	if err != nil {
		return Pod{}, xerrors.Errorf("%w: it is not a signed token", ErrNotAttested)
	}
	keyID := ""
	if len(token.Headers) > 0 {
		keyID = token.Headers[0].KeyID
	}

	var (
		standard jwt.Claims
		claims   kubernetesClaims
		issuer   Issuer
		verified bool
	)
	// Clusters often share an issuer URL. Whose token this is is decided
	// by whose key signed it.
search:
	for _, candidate := range r.Issuers(now) {
		keys := candidate.Keys.Keys
		if byID := candidate.Keys.Key(keyID); keyID != "" && len(byID) > 0 {
			keys = byID
		}
		for _, key := range keys {
			if err := token.Claims(key.Key, &standard, &claims); err == nil {
				issuer, verified = candidate, true
				break search
			}
		}
	}
	if !verified {
		return Pod{}, xerrors.Errorf("%w: no registered cluster signed it", ErrNotAttested)
	}
	if standard.Expiry == nil {
		return Pod{}, xerrors.Errorf("%w: it does not expire", ErrNotAttested)
	}
	if err := standard.ValidateWithLeeway(jwt.Expected{
		Issuer:      issuer.URL,
		AnyAudience: jwt.Audience{r.audience},
		Time:        now,
	}, tokenLeeway); err != nil {
		return Pod{}, xerrors.Errorf("%w: %s", ErrNotAttested, err)
	}
	pod := Pod{
		Issuer:         issuer.Name,
		Namespace:      claims.Kubernetes.Namespace,
		ServiceAccount: claims.Kubernetes.ServiceAccount.Name,
		PodName:        claims.Kubernetes.Pod.Name,
		PodUID:         claims.Kubernetes.Pod.UID,
	}
	// A token that is not bound to a pod is some other credential of the
	// service account, for example one made with `kubectl create token`.
	if pod.PodUID == "" || pod.PodName == "" {
		return Pod{}, xerrors.Errorf("%w: it is not bound to a pod", ErrNotAttested)
	}
	if !slices.Contains(issuer.Namespaces, pod.Namespace) {
		return Pod{}, xerrors.Errorf("%w: pods of namespace %q are not accepted from cluster %q", ErrNotAttested, pod.Namespace, issuer.Name)
	}
	return pod, nil
}

// MarshalIssuer encodes a cluster for IssuerChannel.
func MarshalIssuer(issuer Issuer) ([]byte, error) {
	return json.Marshal(issuer)
}

// UnmarshalIssuer decodes a cluster from IssuerChannel.
func UnmarshalIssuer(raw []byte) (Issuer, error) {
	var issuer Issuer
	err := json.Unmarshal(raw, &issuer)
	return issuer, err
}
