package workloadoperator_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/workloadoperator"
	"github.com/coder/coder/v2/codersdk"
)

const shapeDeployment = `{
  "apiVersion": "apps/v1",
  "kind": "Deployment",
  "metadata": {"name": "shape", "uid": "dep-uid", "labels": {"coder.com/workload-shape": "true"}},
  "spec": {
    "replicas": 0,
    "selector": {"matchLabels": {"app": "shape"}},
    "template": {
      "metadata": {"labels": {"app": "shape", "team": "platform"}, "annotations": {"note": "kept"}},
      "spec": {
        "serviceAccountName": "powerful",
        "initContainers": [{"name": "init", "image": "alpine", "env": [{"name": "CODER_AGENT_TOKEN", "value": "shape-token"}]}],
        "containers": [{
          "name": "dev",
          "image": "alpine",
          "command": ["sh", "-c", "init script"],
          "env": [
            {"name": "CODER_AGENT_TOKEN", "value": "shape-token"},
            {"name": "KEEP", "value": "me"}
          ],
          "volumeMounts": [{"name": "home", "mountPath": "/home/coder"}]
        }],
        "volumes": [{"name": "home", "emptyDir": {}}]
      }
    }
  }
}`

func parse(t *testing.T, raw string) map[string]any {
	t.Helper()
	var obj map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &obj))
	return obj
}

func TestShapeAgentToken(t *testing.T) {
	t.Parallel()

	token, err := workloadoperator.ShapeAgentToken(parse(t, shapeDeployment))
	require.NoError(t, err)
	require.Equal(t, "shape-token", token)

	_, err = workloadoperator.ShapeAgentToken(parse(t, `{"spec":{"template":{"spec":{"containers":[{"name":"dev"}]}}}}`))
	require.Error(t, err)
}

func TestBuildWorkloadPod(t *testing.T) {
	t.Parallel()

	deployment := parse(t, shapeDeployment)
	workload := codersdk.WorkloadOperatorWorkload{
		ID:          uuid.New(),
		Name:        "workload-1a2b3c4d",
		Deployment:  "shape",
		WorkspaceID: uuid.New(),
	}
	pod, err := workloadoperator.BuildWorkloadPod(deployment, workload, workloadoperator.PodOptions{
		OperatorName:   "default",
		Audience:       "https://coder.example.com",
		ServiceAccount: "coder-workload",
	})
	require.NoError(t, err)

	// Decode into a typed view so the assertions read as a pod spec.
	raw, err := json.Marshal(pod)
	require.NoError(t, err)
	var got struct {
		Kind     string `json:"kind"`
		Metadata struct {
			Name            string            `json:"name"`
			Labels          map[string]string `json:"labels"`
			Annotations     map[string]string `json:"annotations"`
			OwnerReferences []struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
				UID  string `json:"uid"`
			} `json:"ownerReferences"`
		} `json:"metadata"`
		Spec struct {
			ServiceAccountName           string `json:"serviceAccountName"`
			AutomountServiceAccountToken *bool  `json:"automountServiceAccountToken"`
			InitContainers               []container
			Containers                   []container
			Volumes                      []struct {
				Name      string `json:"name"`
				Projected *struct {
					Sources []struct {
						ServiceAccountToken struct {
							Audience          string `json:"audience"`
							ExpirationSeconds int    `json:"expirationSeconds"`
							Path              string `json:"path"`
						} `json:"serviceAccountToken"`
					} `json:"sources"`
				} `json:"projected"`
			} `json:"volumes"`
		} `json:"spec"`
	}
	require.NoError(t, json.Unmarshal(raw, &got))

	require.Equal(t, "Pod", got.Kind)
	require.Equal(t, "workload-workload-1a2b3c4d", got.Metadata.Name)

	// The Deployment's selector must not match, or its ReplicaSet would
	// delete the pod. Labels outside the selector are kept.
	require.NotContains(t, got.Metadata.Labels, "app")
	require.Equal(t, "platform", got.Metadata.Labels["team"])
	require.Equal(t, workload.ID.String(), got.Metadata.Labels[workloadoperator.LabelWorkloadID])
	require.Equal(t, "default", got.Metadata.Labels[workloadoperator.LabelOperator])
	require.Equal(t, workload.WorkspaceID.String(), got.Metadata.Labels[workloadoperator.LabelWorkspaceID])
	require.Equal(t, "kept", got.Metadata.Annotations["note"])

	require.Len(t, got.Metadata.OwnerReferences, 1)
	require.Equal(t, "Deployment", got.Metadata.OwnerReferences[0].Kind)
	require.Equal(t, "dep-uid", got.Metadata.OwnerReferences[0].UID)

	// The pod runs as the permissionless workload account and holds no
	// Kubernetes API token.
	require.Equal(t, "coder-workload", got.Spec.ServiceAccountName)
	require.NotNil(t, got.Spec.AutomountServiceAccountToken)
	require.False(t, *got.Spec.AutomountServiceAccountToken)

	// The shape's agent token is gone from every container and replaced by
	// the pod's own identity token.
	for _, c := range append(got.Spec.InitContainers, got.Spec.Containers...) {
		env := map[string]string{}
		for _, e := range c.Env {
			env[e.Name] = e.Value
		}
		require.NotContains(t, env, "CODER_AGENT_TOKEN", c.Name)
		require.Equal(t, "/var/run/secrets/coder.com/identity/token", env["CODER_AGENT_POD_TOKEN_FILE"], c.Name)
		mounted := false
		for _, m := range c.VolumeMounts {
			if m.MountPath == "/var/run/secrets/coder.com/identity" {
				mounted = m.ReadOnly
			}
		}
		require.True(t, mounted, c.Name)
	}
	require.Equal(t, []string{"sh", "-c", "init script"}, got.Spec.Containers[0].Command, "the daemon command is cloned unchanged")

	require.Len(t, got.Spec.Volumes, 2)
	identity := got.Spec.Volumes[1]
	require.NotNil(t, identity.Projected)
	require.Equal(t, "https://coder.example.com", identity.Projected.Sources[0].ServiceAccountToken.Audience)
	require.Equal(t, 600, identity.Projected.Sources[0].ServiceAccountToken.ExpirationSeconds)

	// Building a pod must not modify the shape it was cloned from.
	require.Equal(t, parse(t, shapeDeployment), deployment)
}

type container struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
	Env     []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"env"`
	VolumeMounts []struct {
		Name      string `json:"name"`
		MountPath string `json:"mountPath"`
		ReadOnly  bool   `json:"readOnly"`
	} `json:"volumeMounts"`
}
