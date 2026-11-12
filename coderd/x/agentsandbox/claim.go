package agentsandbox

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/xerrors"
)

const (
	claimsAPI    = "/apis/extensions.agents.x-k8s.io/v1beta1"
	sandboxesAPI = "/apis/agents.x-k8s.io/v1beta1"
)

// ClaimedPod is the pod a sandbox claim was given.
type ClaimedPod struct {
	Namespace string
	// ClaimName names the SandboxClaim, and so the claim on the pod.
	ClaimName string
	Sandbox   string
	PodName   string
	// PodUID is what the cluster puts into the pod's tokens. It is how
	// Coder recognizes the pod's daemon.
	PodUID string
	// Warm reports whether the pod was running before it was claimed.
	Warm bool
}

// InstanceID is the instance ID of the workspace daemon in the pod.
func (p ClaimedPod) InstanceID() string {
	return InstanceIDPrefix + p.PodUID
}

type objectMeta struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace,omitempty"`
	UID             string            `json:"uid,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`
	OwnerReferences []struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
		UID  string `json:"uid"`
	} `json:"ownerReferences,omitempty"`
}

func (m objectMeta) ownedBy(kind, uid string) bool {
	for _, owner := range m.OwnerReferences {
		if owner.Kind == kind && owner.UID == uid {
			return true
		}
	}
	return false
}

type condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type sandboxClaim struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   objectMeta `json:"metadata"`
	Spec       struct {
		WarmPoolRef struct {
			Name string `json:"name"`
		} `json:"warmPoolRef"`
	} `json:"spec"`
	Status struct {
		Conditions []condition `json:"conditions"`
		Sandbox    struct {
			Name string `json:"name"`
		} `json:"sandbox"`
	} `json:"status"`
}

func (c sandboxClaim) ready() (bool, string) {
	for _, cond := range c.Status.Conditions {
		if cond.Type == "Ready" {
			return cond.Status == "True" && c.Status.Sandbox.Name != "", cond.Reason + ": " + cond.Message
		}
	}
	return false, "the claim has not been looked at yet"
}

type object struct {
	Metadata objectMeta `json:"metadata"`
}

func claimPath(namespace, name string) string {
	path := claimsAPI + "/namespaces/" + url.PathEscape(namespace) + "/sandboxclaims"
	if name != "" {
		path += "/" + url.PathEscape(name)
	}
	return path
}

// ClaimSandbox claims a sandbox from a warm pool and waits until the claim
// has a ready pod. Claiming again under the same name returns the same pod.
//
// The pod is found by following ownership from the claim, which only the
// Agent Sandbox controller writes. Labels and annotations on a pod are not
// evidence of anything.
func (c *KubeClient) ClaimSandbox(ctx context.Context, namespace, name, warmPool string) (ClaimedPod, error) {
	var claim sandboxClaim
	claim.APIVersion, claim.Kind = "extensions.agents.x-k8s.io/v1beta1", "SandboxClaim"
	claim.Metadata.Name = name
	claim.Spec.WarmPoolRef.Name = warmPool
	err := c.Do(ctx, http.MethodPost, claimPath(namespace, ""), claim, &claim)
	var status *StatusError
	if xerrors.As(err, &status) && status.Code == http.StatusConflict {
		err = c.Do(ctx, http.MethodGet, claimPath(namespace, name), nil, &claim)
	}
	if err != nil {
		return ClaimedPod{}, xerrors.Errorf("claim a sandbox from pool %q: %w", warmPool, err)
	}

	// A warm claim is bound within a fraction of a second, so look often
	// at first. A claim on an empty pool waits for a pod to start.
	started := time.Now()
	for {
		ready, why := claim.ready()
		if ready {
			break
		}
		wait := 20 * time.Millisecond
		if time.Since(started) > 2*time.Second {
			wait = 250 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return ClaimedPod{}, xerrors.Errorf("sandbox claim %q is not ready (%s): %w", name, why, ctx.Err())
		case <-time.After(wait):
		}
		if err := c.Do(ctx, http.MethodGet, claimPath(namespace, name), nil, &claim); err != nil {
			return ClaimedPod{}, xerrors.Errorf("read sandbox claim %q: %w", name, err)
		}
	}

	var sandbox, pod object
	sandboxName := claim.Status.Sandbox.Name
	path := sandboxesAPI + "/namespaces/" + url.PathEscape(namespace) + "/sandboxes/" + url.PathEscape(sandboxName)
	if err := c.Do(ctx, http.MethodGet, path, nil, &sandbox); err != nil {
		return ClaimedPod{}, xerrors.Errorf("read sandbox %q: %w", sandboxName, err)
	}
	if !sandbox.Metadata.ownedBy("SandboxClaim", claim.Metadata.UID) {
		return ClaimedPod{}, xerrors.Errorf("sandbox %q does not belong to claim %q", sandboxName, name)
	}
	// A sandbox's pod has the sandbox's name.
	path = "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods/" + url.PathEscape(sandboxName)
	if err := c.Do(ctx, http.MethodGet, path, nil, &pod); err != nil {
		return ClaimedPod{}, xerrors.Errorf("read the pod of sandbox %q: %w", sandboxName, err)
	}
	if !pod.Metadata.ownedBy("Sandbox", sandbox.Metadata.UID) {
		return ClaimedPod{}, xerrors.Errorf("pod %q does not belong to sandbox %q", pod.Metadata.Name, sandboxName)
	}
	return ClaimedPod{
		Namespace: namespace,
		ClaimName: name,
		Sandbox:   sandboxName,
		PodName:   pod.Metadata.Name,
		PodUID:    pod.Metadata.UID,
		Warm:      sandbox.Metadata.Labels["agents.x-k8s.io/launch-type"] == "warm",
	}, nil
}

// ReleaseSandbox deletes a claim, and with it the sandbox and its pod. A
// claim that does not exist is not an error.
func (c *KubeClient) ReleaseSandbox(ctx context.Context, namespace, name string) error {
	err := c.Do(ctx, http.MethodDelete, claimPath(namespace, name), nil, nil)
	if err != nil && !IsNotFound(err) {
		return xerrors.Errorf("delete sandbox claim %q: %w", name, err)
	}
	return nil
}
