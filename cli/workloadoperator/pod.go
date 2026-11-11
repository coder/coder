// Package workloadoperator runs runtime workloads for Coder workspaces in a
// Kubernetes namespace. It is experimental.
//
// A template declares a shape: a Deployment with zero replicas that holds the
// pod template of a workspace daemon. For every workload coderd asks for,
// the operator creates one pod from that template. The pod authenticates to
// coderd with its own projected service account token, so no Coder
// credential is written into any pod.
package workloadoperator

import (
	"encoding/json"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

const (
	// LabelShape marks a Deployment as a workload shape.
	LabelShape = "coder.com/workload-shape"
	// LabelOperator names the operator that manages a workload pod.
	LabelOperator = "coder.com/workload-operator"
	// LabelWorkloadID is the Coder workload a pod runs.
	LabelWorkloadID = "coder.com/workload-id"
	// LabelWorkloadName is the workload's agent name.
	LabelWorkloadName = "coder.com/workload-name"
	// LabelWorkspaceID is the workspace a workload pod belongs to.
	LabelWorkspaceID = "coder.com/workspace-id"
	// LabelShapeDeployment is the Deployment a workload pod was cloned from.
	LabelShapeDeployment = "coder.com/workload-shape-deployment"

	identityVolumeName = "coder-workload-identity"
	// podTokenExpirySeconds is the shortest lifetime Kubernetes allows for
	// a projected token. A copied token stops working within this time.
	podTokenExpirySeconds = 600

	// These are the names of environment variables, not credentials.
	agentTokenEnv     = "CODER_AGENT_TOKEN"          //nolint:gosec
	agentTokenFileEnv = "CODER_AGENT_TOKEN_FILE"     //nolint:gosec
	podTokenFileEnv   = "CODER_AGENT_POD_TOKEN_FILE" //nolint:gosec
)

// PodOptions are the operator settings applied to every workload pod.
type PodOptions struct {
	OperatorName string
	// Audience is the audience of the pod's identity token. coderd rejects
	// tokens issued for anything else, including the Kubernetes API.
	Audience string
	// ServiceAccount runs workload pods. It must have no permissions.
	ServiceAccount string
}

type object = map[string]any

func nested(obj object, path ...string) (object, bool) {
	current := obj
	for _, key := range path {
		next, ok := current[key].(object)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func deepCopy(obj object) (object, error) {
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var out object
	return out, json.Unmarshal(raw, &out)
}

// containers returns every container of a pod spec, init containers
// included.
func containers(spec object) []object {
	var out []object
	for _, key := range []string{"initContainers", "containers"} {
		list, _ := spec[key].([]any)
		for _, item := range list {
			if container, ok := item.(object); ok {
				out = append(out, container)
			}
		}
	}
	return out
}

// ShapeAgentToken returns the Coder agent token a shape's pod template
// carries. The token identifies which workspace agent the shape defines.
func ShapeAgentToken(deployment object) (string, error) {
	spec, ok := nested(deployment, "spec", "template", "spec")
	if !ok {
		return "", xerrors.New("deployment has no pod template")
	}
	for _, container := range containers(spec) {
		env, _ := container["env"].([]any)
		for _, item := range env {
			entry, ok := item.(object)
			if !ok || entry["name"] != agentTokenEnv {
				continue
			}
			value, _ := entry["value"].(string)
			if value == "" {
				return "", xerrors.Errorf("%s must be set as a literal value", agentTokenEnv)
			}
			return value, nil
		}
	}
	return "", xerrors.Errorf("no container sets %s", agentTokenEnv)
}

// PodName returns the name of the pod that runs a workload.
func PodName(workload codersdk.WorkloadOperatorWorkload) string {
	return "workload-" + workload.Name
}

// BuildWorkloadPod clones a shape's pod template into a pod for one
// workload. The clone differs from the template in identity only: the
// shape's agent token is removed, and the pod gets a token that is bound to
// itself.
func BuildWorkloadPod(deployment object, workload codersdk.WorkloadOperatorWorkload, opts PodOptions) (object, error) {
	deploymentMeta, ok := nested(deployment, "metadata")
	if !ok {
		return nil, xerrors.New("deployment has no metadata")
	}
	deploymentName, _ := deploymentMeta["name"].(string)
	deploymentUID, _ := deploymentMeta["uid"].(string)
	if deploymentName == "" || deploymentUID == "" {
		return nil, xerrors.New("deployment has no name or uid")
	}
	template, ok := nested(deployment, "spec", "template")
	if !ok {
		return nil, xerrors.New("deployment has no pod template")
	}
	template, err := deepCopy(template)
	if err != nil {
		return nil, xerrors.Errorf("copy pod template: %w", err)
	}
	spec, ok := nested(template, "spec")
	if !ok {
		return nil, xerrors.New("pod template has no spec")
	}

	// The Deployment's ReplicaSet deletes pods that match its selector
	// beyond its replica count of zero, so the pod must not match.
	labels := object{}
	selector, _ := nested(deployment, "spec", "selector")
	matchLabels, _ := nested(selector, "matchLabels")
	_, hasExpressions := selector["matchExpressions"]
	if templateLabels, ok := nested(template, "metadata", "labels"); ok && !hasExpressions {
		for key, value := range templateLabels {
			if _, selected := matchLabels[key]; !selected {
				labels[key] = value
			}
		}
	}
	labels[LabelOperator] = opts.OperatorName
	labels[LabelWorkloadID] = workload.ID.String()
	labels[LabelWorkloadName] = workload.Name
	labels[LabelWorkspaceID] = workload.WorkspaceID.String()
	labels[LabelShapeDeployment] = deploymentName

	metadata := object{
		"name":   PodName(workload),
		"labels": labels,
		// The owner reference makes Kubernetes delete the pod when the
		// workspace stops and the template removes the Deployment.
		"ownerReferences": []any{object{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       deploymentName,
			"uid":        deploymentUID,
		}},
	}
	if annotations, ok := nested(template, "metadata", "annotations"); ok {
		metadata["annotations"] = annotations
	}

	volumes, _ := spec["volumes"].([]any)
	for _, item := range volumes {
		if volume, ok := item.(object); ok && volume["name"] == identityVolumeName {
			return nil, xerrors.Errorf("pod template already has a volume named %q", identityVolumeName)
		}
	}
	spec["volumes"] = append(volumes, object{
		"name": identityVolumeName,
		"projected": object{
			"sources": []any{object{
				"serviceAccountToken": object{
					"audience":          opts.Audience,
					"expirationSeconds": podTokenExpirySeconds,
					"path":              "token",
				},
			}},
		},
	})
	// The pod's only credential is the identity token above. It must not
	// hold a token for the Kubernetes API.
	spec["serviceAccountName"] = opts.ServiceAccount
	delete(spec, "serviceAccount")
	spec["automountServiceAccountToken"] = false

	tokenDir := agentsdk.DefaultKubernetesPodTokenPath[:len(agentsdk.DefaultKubernetesPodTokenPath)-len("/token")]
	for _, container := range containers(spec) {
		env, _ := container["env"].([]any)
		kept := make([]any, 0, len(env)+1)
		for _, item := range env {
			entry, ok := item.(object)
			if ok && (entry["name"] == agentTokenEnv || entry["name"] == agentTokenFileEnv || entry["name"] == podTokenFileEnv) {
				continue
			}
			kept = append(kept, item)
		}
		container["env"] = append(kept, object{
			"name":  podTokenFileEnv,
			"value": agentsdk.DefaultKubernetesPodTokenPath,
		})
		mounts, _ := container["volumeMounts"].([]any)
		container["volumeMounts"] = append(mounts, object{
			"name":      identityVolumeName,
			"mountPath": tokenDir,
			"readOnly":  true,
		})
	}

	return object{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   metadata,
		"spec":       spec,
	}, nil
}
