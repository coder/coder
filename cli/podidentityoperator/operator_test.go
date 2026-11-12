package podidentityoperator_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/podidentityoperator"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/x/podidentity"
	"github.com/coder/coder/v2/coderd/x/podidentity/podidentitytest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

// TestOperator runs the operator against a stand-in for a cluster's API
// and a real coderd. coderd is given nothing of the cluster's.
func TestOperator(t *testing.T) {
	t.Parallel()

	cluster := podidentitytest.NewCluster(t)
	// What every service account of a cluster may read: the issuer of its
	// tokens and the public keys they are signed with.
	api := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer the-operators-service-account" {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(rw).Encode(map[string]string{
				"issuer": podidentitytest.IssuerURL, "jwks_uri": "https://somewhere.else.example.com/openid/v1/jwks",
			})
		case "/openid/v1/jwks":
			_ = json.NewEncoder(rw).Encode(cluster.Keys())
		default:
			rw.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(api.Close)
	tokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("the-operators-service-account\n"), 0o600))

	client := coderdtest.New(t, &coderdtest.Options{
		PodIdentity:            podidentity.NewRegistry(podidentitytest.Audience),
		PodIdentityOperatorKey: "operators-only",
	})
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitShort)
	present := func(token string) int {
		res, err := client.RequestWithoutSessionToken(ctx, http.MethodPost, "/api/v2/workspaceagents/kubernetes-pod-identity",
			agentsdk.KubernetesPodIdentityToken{Token: token})
		require.NoError(t, err)
		defer res.Body.Close()
		return res.StatusCode
	}
	pod := cluster.NewPod("pod-1")
	require.Equal(t, http.StatusUnauthorized, present(pod.Token))

	options := podidentityoperator.Options{
		Logger:        testutil.Logger(t),
		Coder:         codersdk.New(client.URL),
		OperatorKey:   "operators-only",
		Name:          "cluster-a",
		Namespaces:    []string{podidentitytest.Namespace},
		KubeURL:       api.URL,
		KubeTokenFile: tokenFile,
	}
	operator, err := podidentityoperator.New(options)
	require.NoError(t, err)
	registered, err := operator.Sync(ctx)
	require.NoError(t, err)
	require.Equal(t, "cluster-a", registered.Name)
	require.Equal(t, podidentitytest.IssuerURL, registered.IssuerURL)

	// coderd now believes the cluster about its pods, and about no others.
	require.Equal(t, http.StatusNotFound, present(pod.Token), "a pod of the cluster, which no workspace build has registered")
	require.Equal(t, http.StatusUnauthorized, present(podidentitytest.NewCluster(t).NewPod("pod-1").Token))
	issuers, err := client.PodIdentityIssuers(ctx)
	require.NoError(t, err)
	require.Len(t, issuers, 1)
	require.Equal(t, registered.KeyIDs, issuers[0].KeyIDs)

	// An operator with the wrong key registers nothing.
	options.OperatorKey = "guessed"
	options.Name = "cluster-b"
	impostor, err := podidentityoperator.New(options)
	require.NoError(t, err)
	_, err = impostor.Sync(ctx)
	require.Error(t, err)
	issuers, err = client.PodIdentityIssuers(ctx)
	require.NoError(t, err)
	require.Len(t, issuers, 1)
}
