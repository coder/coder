package tests

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// LoadChart loads the chart from the parent directory.
func LoadChart() (*Chart, error) {
	ch, err := loader.LoadDir("..")
	if err != nil {
		return nil, xerrors.Errorf("failed to load chart: %w", err)
	}
	if ch == nil {
		return nil, xerrors.New("chart should not be nil")
	}

	originalValues, err := mapToValues(ch.Values)
	if err != nil {
		return nil, xerrors.Errorf("failed to convert values to map: %w", err)
	}

	return &Chart{
		chart:          ch,
		Metadata:       ch.Metadata,
		Template:       ch.Templates,
		Files:          ch.Files,
		OriginalValues: originalValues,
	}, nil
}

// Chart is a wrapper around helm.sh/helm/v3/pkg/chart.Chart.
type Chart struct {
	chart          *chart.Chart
	Metadata       *chart.Metadata
	Template       []*chart.File
	Files          []*chart.File
	OriginalValues *Values
}

// Validate validates the chart.
func (c *Chart) Validate() error {
	return c.chart.Validate()
}

// Render renders the chart with the given values.
func (c *Chart) Render(mut func(*Values), opts *chartutil.ReleaseOptions, caps *chartutil.Capabilities) ([]runtime.Object, error) {
	manifests, err := renderManifests(c.chart, c.OriginalValues, mut, opts, caps)
	if err != nil {
		return nil, xerrors.Errorf("failed to render manifests: %w", err)
	}

	// As a special case, ignore any .txt files (e.g. NOTES.txt)
	for key := range manifests {
		if filepath.Ext(key) == ".txt" {
			delete(manifests, key)
		}
	}

	objs, err := loadObjectsFromManifests(manifests)
	if err != nil {
		return nil, xerrors.Errorf("failed to load objects from manifests: %w", err)
	}
	return objs, nil
}

// Values is the top-level values struct for the Coder chart.
type Values struct {
	Coder          CoderValues            `json:"coder" yaml:"coder"`
	ExtraTemplates map[string]interface{} `json:"extraTemplates" yaml:"extraTemplates"`
}

// Values must implement fmt.Stringer
var _ = fmt.Stringer(&Values{})

// String returns a JSON representation of v.
func (v *Values) String() string {
	var sb strings.Builder
	enc := json.NewEncoder(&sb)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return sb.String()
}

// CoderValues contains the values for the Coder deployment.
type CoderValues struct {
	Affinity        corev1.Affinity             `json:"affinity" yaml:"affinity"`
	Annotations     map[string]string           `json:"annotations" yaml:"annotations"`
	Certs           CoderCertsValues            `json:"certs" yaml:"certs"`
	Env             []corev1.EnvVar             `json:"env" yaml:"env"`
	Image           CoderImageValues            `json:"image" yaml:"image"`
	InitContainers  []corev1.Container          `json:"initContainers" yaml:"initContainers"`
	NodeSelector    map[string]string           `json:"nodeSelector" yaml:"nodeSelector"`
	Resources       corev1.ResourceRequirements `json:"resources" yaml:"resources"`
	ReplicaCount    int                         `json:"replicaCount" yaml:"replicaCount"`
	SecurityContext corev1.PodSecurityContext   `json:"securityContext" yaml:"securityContext"`
	Service         CoderServiceValues          `json:"service" yaml:"service"`
	ServiceAccount  CoderServiceAccountValues   `json:"serviceAccount" yaml:"serviceAccount"`
	TLS             CoderTLSValues              `json:"tls" yaml:"tls"`
	Tolerations     corev1.Toleration           `json:"tolerations" yaml:"tolerations"`
	Volumes         []corev1.Volume             `json:"volumes" yaml:"volumes"`
	VolumeMounts    []corev1.VolumeMount        `json:"volumeMounts" yaml:"volumeMounts"`
}

// CoderCertsValues contains the values for the Coder certs.
type CoderCertsValues struct {
	Secrets []CoderCertsValuesSecret `json:"secrets" yaml:"secrets"`
}

// CoderCertsValuesSecret contains the values for a Coder cert secret.
type CoderCertsValuesSecret struct {
	Key  string `json:"key" yaml:"key"`
	Name string `json:"name" yaml:"name"`
}

// CoderImageValues contains the values for the Coder image.
type CoderImageValues struct {
	Repo        string   `json:"repo" yaml:"repo"`
	Tag         string   `json:"tag" yaml:"tag"`
	PullPolicy  string   `json:"pullPolicy" yaml:"pullPolicy"`
	PullSecrets []string `json:"pullSecrets" yaml:"pullSecrets"`
}

// CoderServiceValues contains the values for the Coder service.
type CoderServiceValues struct {
	Annotations           map[string]string `json:"annotations" yaml:"annotations"`
	Enable                bool              `json:"enable" yaml:"enable"`
	ExternalTrafficPolicy string            `json:"externalTrafficPolicy" yaml:"externalTrafficPolicy"`
	LoadBalancerIP        string            `json:"loadBalancerIP" yaml:"loadBalancerIP"`
	SessionAffinity       string            `json:"sessionAffinity" yaml:"sessionAffinity"`
	Type                  string            `json:"type" yaml:"type"`
}

// CoderIngressValues contains the values for the Coder ingress.
type CoderIngressValues struct {
	Annotations  map[string]string     `json:"annotations" yaml:"annotations"`
	ClassName    string                `json:"className" yaml:"className"`
	Enable       bool                  `json:"enable" yaml:"enable"`
	Host         string                `json:"host" yaml:"host"`
	TLS          CoderIngressTLSValues `json:"tls" yaml:"tls"`
	WildcardHost string                `json:"wildcardHost" yaml:"wildcardHost"`
}

// CoderIngressTLSValues contains the values for the Coder ingress TLS.
type CoderIngressTLSValues struct {
	Enable             bool   `json:"enable" yaml:"enable"`
	SecretName         string `json:"secretName" yaml:"secretName"`
	WildcardSecretName string `json:"wildcardSecretName" yaml:"wildcardSecretName"`
}

// CoderServiceAccountValues contains the values for the Coder service account.
type CoderServiceAccountValues struct {
	Annotations    map[string]string `json:"annotations" yaml:"annotations"`
	Name           string            `json:"name" yaml:"name"`
	WorkspacePerms bool              `json:"workspacePerms" yaml:"workspacePerms"`
}

// CoderTLSValues contains the values for the Coder TLS secret names.
type CoderTLSValues struct {
	SecretNames []string `json:"secretNames" yaml:"secretNames"`
}
