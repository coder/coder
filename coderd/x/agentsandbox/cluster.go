package agentsandbox

import (
	"golang.org/x/xerrors"
)

// Environment variables that tell coderd where sandboxes run. They are not
// deployment options yet.
const (
	// EnvNamespace is the namespace sandboxes are claimed in. Setting it
	// turns sandbox workspaces on.
	EnvNamespace = "CODER_SANDBOX_KUBE_NAMESPACE"
	// EnvKubeURL, EnvKubeTokenFile and EnvKubeCAFile say how to reach the
	// cluster. Without them coderd uses the cluster it runs in.
	EnvKubeURL       = "CODER_SANDBOX_KUBE_URL"
	EnvKubeTokenFile = "CODER_SANDBOX_KUBE_TOKEN_FILE" //nolint:gosec // The name of a variable.
	EnvKubeCAFile    = "CODER_SANDBOX_KUBE_CA_FILE"
	// EnvAudience is the audience pod tokens must be issued for. It
	// defaults to the access URL.
	EnvAudience = "CODER_SANDBOX_AUDIENCE"
)

// Cluster is the cluster Coder claims sandboxes in.
type Cluster struct {
	Kube *KubeClient
	// Namespace is where sandboxes are claimed. Templates cannot choose
	// another.
	Namespace string
	// Attestor tells which pod a token belongs to.
	Attestor *Attestor
}

// ClusterFromEnv reads the cluster from the environment. It returns nil
// when no namespace is set.
func ClusterFromEnv(getenv func(string) string, accessURL string) (*Cluster, error) {
	namespace := getenv(EnvNamespace)
	if namespace == "" {
		return nil, nil //nolint:nilnil // Not configured is not an error.
	}
	kube, err := NewKubeClient(KubeConfig{
		URL:       getenv(EnvKubeURL),
		TokenFile: getenv(EnvKubeTokenFile),
		CAFile:    getenv(EnvKubeCAFile),
	})
	if err != nil {
		return nil, xerrors.Errorf("%s is set, but the cluster cannot be reached: %w", EnvNamespace, err)
	}
	audience := getenv(EnvAudience)
	if audience == "" {
		audience = accessURL
	}
	return &Cluster{Kube: kube, Namespace: namespace, Attestor: NewAttestor(kube, audience)}, nil
}
