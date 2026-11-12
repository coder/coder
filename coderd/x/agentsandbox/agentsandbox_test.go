package agentsandbox_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/agentsandbox"
	"github.com/coder/coder/v2/coderd/x/agentsandbox/agentsandboxtest"
	"github.com/coder/coder/v2/testutil"
)

func TestAttestor(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	cluster := agentsandboxtest.NewCluster(t)
	attestor := agentsandbox.NewAttestor(cluster.Kube, cluster.Audience)

	// The cluster says which pod a token was issued to.
	pod := cluster.NewPod("alpha")
	identity, err := attestor.Attest(ctx, pod.Token)
	require.NoError(t, err)
	require.Equal(t, agentsandbox.PodIdentity{
		Namespace: cluster.Namespace, ServiceAccount: "coder-workload", PodName: "alpha", PodUID: pod.UID,
	}, identity)
	require.Equal(t, "k8s-pod:"+pod.UID, identity.InstanceID())

	// A token the cluster does not know, such as a forged one.
	_, err = attestor.Attest(ctx, "made-up")
	require.ErrorIs(t, err, agentsandbox.ErrNotAttested)
	// A token of the same service account that is not bound to a pod,
	// such as one made with `kubectl create token`.
	_, err = attestor.Attest(ctx, cluster.NewServiceAccountToken())
	require.ErrorIs(t, err, agentsandbox.ErrNotAttested)
	require.ErrorContains(t, err, "not bound to a pod")
	// A pod's token for another audience, such as the Kubernetes API.
	_, err = agentsandbox.NewAttestor(cluster.Kube, "https://kubernetes.default.svc").Attest(ctx, pod.Token)
	require.ErrorIs(t, err, agentsandbox.ErrNotAttested)
}

func TestClaimSandbox(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	cluster := agentsandboxtest.NewCluster(t)

	claimed, err := cluster.Kube.ClaimSandbox(ctx, cluster.Namespace, "coder-1", "pool")
	require.NoError(t, err)
	pod, ok := cluster.ClaimedPod("coder-1")
	require.True(t, ok)
	require.Equal(t, agentsandbox.ClaimedPod{
		Namespace: cluster.Namespace, ClaimName: "coder-1", Sandbox: pod.Name, PodName: pod.Name, PodUID: pod.UID, Warm: true,
	}, claimed)
	require.Equal(t, "k8s-pod:"+pod.UID, claimed.InstanceID())

	// Claiming again under the same name yields the same pod.
	again, err := cluster.Kube.ClaimSandbox(ctx, cluster.Namespace, "coder-1", "pool")
	require.NoError(t, err)
	require.Equal(t, claimed, again)

	// A pod that merely has the sandbox's name is not the sandbox's pod.
	cluster.Stranger = true
	_, err = cluster.Kube.ClaimSandbox(ctx, cluster.Namespace, "coder-2", "pool")
	require.ErrorContains(t, err, "does not belong to sandbox")

	// Giving a claim up deletes it. Doing so twice is fine.
	require.NoError(t, cluster.Kube.ReleaseSandbox(ctx, cluster.Namespace, "coder-1"))
	_, ok = cluster.ClaimedPod("coder-1")
	require.False(t, ok)
	require.NoError(t, cluster.Kube.ReleaseSandbox(ctx, cluster.Namespace, "coder-1"))
}
