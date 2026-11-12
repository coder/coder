// Package agentsandboxtest provides a stand-in for a cluster that runs the
// Agent Sandbox controller.
package agentsandboxtest

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/agentsandbox"
)

// Pod is a pod of the fake cluster.
type Pod struct {
	Name string
	UID  string
	// Token is the token the kubelet would have obtained for the pod.
	Token string
}

// Cluster is a fake Kubernetes API server. It answers token reviews, and
// plays the Agent Sandbox controller: a claim is bound to a new pod at once.
type Cluster struct {
	t      testing.TB
	server *httptest.Server
	// Kube is a client for the fake.
	Kube *agentsandbox.KubeClient
	// Audience is what the fake's pod tokens are issued for.
	Audience string
	// Namespace is the only namespace of the fake.
	Namespace string

	mu sync.Mutex
	// tokens maps a token to the user the cluster reports for it.
	tokens map[string]map[string]any
	claims map[string]*claim
	// Stranger makes the next claim's pod belong to no sandbox, as a pod
	// that somebody else created under the same name would.
	Stranger bool
}

type claim struct {
	uid, sandboxUID string
	pod             Pod
	stranger        bool
}

// NewCluster starts a fake cluster.
func NewCluster(t testing.TB) *Cluster {
	t.Helper()
	c := &Cluster{
		t: t, Audience: "https://coder.example.com", Namespace: "sandboxes",
		tokens: make(map[string]map[string]any), claims: make(map[string]*claim),
	}
	c.server = httptest.NewTLSServer(http.HandlerFunc(c.serve))
	t.Cleanup(c.server.Close)

	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.crt")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: c.server.Certificate().Raw,
	}), 0o600))
	token := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(token, []byte("coder-server-token\n"), 0o600))
	kube, err := agentsandbox.NewKubeClient(agentsandbox.KubeConfig{URL: c.server.URL, TokenFile: token, CAFile: ca})
	require.NoError(t, err)
	c.Kube = kube
	return c
}

// AsCluster returns the fake as coderd is configured with it.
func (c *Cluster) AsCluster() *agentsandbox.Cluster {
	return &agentsandbox.Cluster{Kube: c.Kube, Namespace: c.Namespace, Attestor: agentsandbox.NewAttestor(c.Kube, c.Audience)}
}

// NewPod adds a pod that no claim has, with a token of its own.
func (c *Cluster) NewPod(name string) Pod {
	pod := Pod{Name: name, UID: uuid.NewString(), Token: "pod-token-" + uuid.NewString()}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens[pod.Token] = map[string]any{
		"username": "system:serviceaccount:" + c.Namespace + ":coder-workload",
		"extra": map[string][]string{
			"authentication.kubernetes.io/pod-name": {pod.Name},
			"authentication.kubernetes.io/pod-uid":  {pod.UID},
		},
	}
	return pod
}

// NewServiceAccountToken adds a token of the workload service account that
// is not bound to any pod.
func (c *Cluster) NewServiceAccountToken() string {
	token := "sa-token-" + uuid.NewString()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens[token] = map[string]any{"username": "system:serviceaccount:" + c.Namespace + ":coder-workload"}
	return token
}

// ClaimedPod returns the pod a claim was bound to.
func (c *Cluster) ClaimedPod(claimName string) (Pod, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cl, ok := c.claims[claimName]
	if !ok {
		return Pod{}, false
	}
	return cl.pod, true
}

func (c *Cluster) serve(rw http.ResponseWriter, r *http.Request) {
	write := func(status int, body any) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(status)
		_ = json.NewEncoder(rw).Encode(body)
	}
	if r.Header.Get("Authorization") != "Bearer coder-server-token" {
		write(http.StatusUnauthorized, map[string]any{"message": "Unauthorized"})
		return
	}
	claimsPath := "/apis/extensions.agents.x-k8s.io/v1beta1/namespaces/" + c.Namespace + "/sandboxclaims"
	sandboxesPath := "/apis/agents.x-k8s.io/v1beta1/namespaces/" + c.Namespace + "/sandboxes/"
	podsPath := "/api/v1/namespaces/" + c.Namespace + "/pods/"

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/apis/authentication.k8s.io/v1/tokenreviews":
		var review struct {
			Spec struct {
				Token     string   `json:"token"`
				Audiences []string `json:"audiences"`
			} `json:"spec"`
		}
		_ = json.NewDecoder(r.Body).Decode(&review)
		c.mu.Lock()
		user, ok := c.tokens[review.Spec.Token]
		c.mu.Unlock()
		status := map[string]any{"authenticated": false, "error": "invalid bearer token"}
		if ok && len(review.Spec.Audiences) == 1 && review.Spec.Audiences[0] == c.Audience {
			status = map[string]any{"authenticated": true, "audiences": review.Spec.Audiences, "user": user}
		}
		write(http.StatusCreated, map[string]any{"status": status})

	case r.Method == http.MethodPost && r.URL.Path == claimsPath:
		var body struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		c.mu.Lock()
		_, exists := c.claims[body.Metadata.Name]
		stranger := c.Stranger
		c.Stranger = false
		c.mu.Unlock()
		if exists {
			write(http.StatusConflict, map[string]any{"message": "sandboxclaims already exists"})
			return
		}
		pod := c.NewPod("warm-" + uuid.NewString()[:8])
		cl := &claim{uid: uuid.NewString(), sandboxUID: uuid.NewString(), pod: pod, stranger: stranger}
		c.mu.Lock()
		c.claims[body.Metadata.Name] = cl
		c.mu.Unlock()
		// The controller has not looked at the claim yet.
		write(http.StatusCreated, map[string]any{"metadata": map[string]any{"name": body.Metadata.Name, "uid": cl.uid}})

	case strings.HasPrefix(r.URL.Path, claimsPath+"/"):
		name := strings.TrimPrefix(r.URL.Path, claimsPath+"/")
		c.mu.Lock()
		cl, ok := c.claims[name]
		if ok && r.Method == http.MethodDelete {
			delete(c.claims, name)
			delete(c.tokens, cl.pod.Token)
		}
		c.mu.Unlock()
		if !ok {
			write(http.StatusNotFound, map[string]any{"message": "sandboxclaims \"" + name + "\" not found"})
			return
		}
		write(http.StatusOK, map[string]any{
			"metadata": map[string]any{"name": name, "uid": cl.uid},
			"status": map[string]any{
				"conditions": []map[string]any{{"type": "Ready", "status": "True", "reason": "DependenciesReady"}},
				"sandbox":    map[string]any{"name": cl.pod.Name},
			},
		})

	case r.Method == http.MethodGet && (strings.HasPrefix(r.URL.Path, sandboxesPath) || strings.HasPrefix(r.URL.Path, podsPath)):
		isPod := strings.HasPrefix(r.URL.Path, podsPath)
		name := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, sandboxesPath), podsPath)
		c.mu.Lock()
		var found *claim
		for _, cl := range c.claims {
			if cl.pod.Name == name {
				found = cl
			}
		}
		c.mu.Unlock()
		switch {
		case found == nil:
			write(http.StatusNotFound, map[string]any{"message": "not found"})
		case !isPod:
			write(http.StatusOK, map[string]any{"metadata": map[string]any{
				"name": name, "uid": found.sandboxUID,
				"labels":          map[string]string{"agents.x-k8s.io/launch-type": "warm"},
				"ownerReferences": []map[string]any{{"kind": "SandboxClaim", "name": "claim", "uid": found.uid}},
			}})
		default:
			owner := found.sandboxUID
			if found.stranger {
				owner = uuid.NewString()
			}
			write(http.StatusOK, map[string]any{"metadata": map[string]any{
				"name": name, "uid": found.pod.UID,
				"ownerReferences": []map[string]any{{"kind": "Sandbox", "name": name, "uid": owner}},
			}})
		}

	default:
		write(http.StatusNotFound, map[string]any{"message": "the fake cluster has no " + r.Method + " " + r.URL.Path})
	}
}
