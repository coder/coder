package podidentity_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/podidentity"
	"github.com/coder/coder/v2/coderd/x/podidentity/podidentitytest"
)

func TestRegistry(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cluster := podidentitytest.NewCluster(t)
	registry := cluster.Registry()

	// The cluster's keys say which pod a token was issued to.
	pod := cluster.NewPod("alpha")
	attested, err := registry.Attest(pod.Token, now)
	require.NoError(t, err)
	require.Equal(t, podidentity.Pod{
		Issuer: cluster.Name, Namespace: podidentitytest.Namespace, ServiceAccount: podidentitytest.ServiceAccount,
		PodName: "alpha", PodUID: pod.UID,
	}, attested)
	require.Equal(t, "k8s-pod:"+pod.UID, attested.InstanceID())

	for name, token := range map[string]string{
		"not a token":            "made-up",
		"signed by a stranger":   podidentitytest.NewCluster(t).NewPod("alpha").Token,
		"for another audience":   cluster.Token("alpha", pod.UID, podidentitytest.TokenOptions{Audience: "https://kubernetes.default.svc"}),
		"from another namespace": cluster.Token("alpha", pod.UID, podidentitytest.TokenOptions{Namespace: "elsewhere"}),
		"expired":                cluster.Token("alpha", pod.UID, podidentitytest.TokenOptions{Expiry: now.Add(-time.Hour)}),
		"not bound to a pod":     cluster.Token("", "", podidentitytest.TokenOptions{NotPodBound: true}),
	} {
		_, err := registry.Attest(token, now)
		require.ErrorIs(t, err, podidentity.ErrNotAttested, name)
	}

	// Two clusters with the same issuer URL are told apart by their keys.
	other := podidentitytest.NewCluster(t)
	require.NoError(t, registry.Put(other.Issuer(now)))
	attested, err = registry.Attest(other.NewPod("beta").Token, now)
	require.NoError(t, err)
	require.Equal(t, other.Name, attested.Issuer)
	attested, err = registry.Attest(pod.Token, now)
	require.NoError(t, err)
	require.Equal(t, cluster.Name, attested.Issuer)

	// A cluster whose operator is gone is no longer believed.
	later := now.Add(podidentity.IssuerTTL + time.Second)
	_, err = registry.Attest(cluster.Token("alpha", pod.UID, podidentitytest.TokenOptions{Expiry: later.Add(time.Hour)}), later)
	require.ErrorIs(t, err, podidentity.ErrNotAttested)
	require.Empty(t, registry.Issuers(later))

	// What a cluster is registered with is checked.
	require.Error(t, registry.Put(podidentity.Issuer{Name: "no-issuer", Namespaces: []string{"x"}, Keys: cluster.Keys()}))
	require.Error(t, registry.Put(podidentity.Issuer{Name: "x", URL: podidentitytest.IssuerURL, Keys: cluster.Keys()}))
	require.Error(t, registry.Put(podidentity.Issuer{Name: "x", URL: podidentitytest.IssuerURL, Namespaces: []string{"x"}}))
}
