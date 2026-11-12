package podidentity_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/podidentity"
	"github.com/coder/coder/v2/coderd/x/podidentity/podidentitytest"
	"github.com/coder/coder/v2/testutil"
)

func TestAttestor(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	cluster := podidentitytest.NewCluster(t)
	attestor := podidentity.NewAttestor(cluster.Kube, cluster.Audience)

	// The cluster says which pod a token was issued to.
	pod := cluster.NewPod("alpha")
	identity, err := attestor.Attest(ctx, pod.Token)
	require.NoError(t, err)
	require.Equal(t, podidentity.Pod{
		Namespace: cluster.Namespace, ServiceAccount: "coder-workload", PodName: "alpha", PodUID: pod.UID,
	}, identity)
	require.Equal(t, "k8s-pod:"+pod.UID, identity.InstanceID())

	// A token the cluster does not know, such as a forged one.
	_, err = attestor.Attest(ctx, "made-up")
	require.ErrorIs(t, err, podidentity.ErrNotAttested)
	// A token of the same service account that is not bound to a pod,
	// such as one made with `kubectl create token`.
	_, err = attestor.Attest(ctx, cluster.NewServiceAccountToken())
	require.ErrorIs(t, err, podidentity.ErrNotAttested)
	require.ErrorContains(t, err, "not bound to a pod")
	// A pod's token for another audience, such as the Kubernetes API.
	_, err = podidentity.NewAttestor(cluster.Kube, "https://kubernetes.default.svc").Attest(ctx, pod.Token)
	require.ErrorIs(t, err, podidentity.ErrNotAttested)
}
