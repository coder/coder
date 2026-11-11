package workloads_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/workloads"
	"github.com/coder/coder/v2/coderd/x/workloads/workloadstest"
	"github.com/coder/quartz"
)

const (
	testIssuer    = workloadstest.Issuer
	testAudience  = workloadstest.Audience
	testNamespace = workloadstest.Namespace
	podName       = "workload-a"
	podUID        = "11111111-1111-1111-1111-111111111111"
)

func TestVerifyPodToken(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	trusted := workloadstest.NewCluster(t)
	other := workloadstest.NewCluster(t)

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		claims, err := workloads.VerifyPodToken(trusted.Sign(workloadstest.ValidPodToken(now, podName, podUID)), trusted.Keys(), testIssuer, testAudience, testNamespace, now)
		require.NoError(t, err)
		require.Equal(t, "workload-a", claims.PodName)
		require.Equal(t, "11111111-1111-1111-1111-111111111111", claims.PodUID)
		require.Equal(t, testNamespace, claims.Namespace)
		require.Equal(t, "coder-workload", claims.ServiceAccount)
		require.Equal(t, "k8s-pod:11111111-1111-1111-1111-111111111111", claims.InstanceID())
	})

	rejected := map[string]func(tok *workloadstest.PodToken) *workloadstest.Cluster{
		// A token signed by a key the cluster does not publish.
		"ForgedSignature": func(*workloadstest.PodToken) *workloadstest.Cluster { return other },
		// A token meant for the Kubernetes API, not for coderd.
		"WrongAudience": func(tok *workloadstest.PodToken) *workloadstest.Cluster {
			tok.Audience = "https://kubernetes.default.svc"
			return trusted
		},
		"WrongIssuer": func(tok *workloadstest.PodToken) *workloadstest.Cluster {
			tok.Issuer = "https://elsewhere.example.com"
			return trusted
		},
		"Expired": func(tok *workloadstest.PodToken) *workloadstest.Cluster {
			tok.Expiry = now.Add(-time.Minute)
			return trusted
		},
		"OtherNamespace": func(tok *workloadstest.PodToken) *workloadstest.Cluster {
			tok.Namespace = "other"
			return trusted
		},
		// A service account token that is not bound to any pod.
		"NotPodBound": func(tok *workloadstest.PodToken) *workloadstest.Cluster {
			tok.NotPodBound = true
			return trusted
		},
	}
	for name, mutate := range rejected {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tok := workloadstest.ValidPodToken(now, podName, podUID)
			signer := mutate(&tok)
			_, err := workloads.VerifyPodToken(signer.Sign(tok), trusted.Keys(), testIssuer, testAudience, testNamespace, now)
			require.Error(t, err)
		})
	}

	t.Run("Garbage", func(t *testing.T) {
		t.Parallel()
		_, err := workloads.VerifyPodToken("not.a.token", trusted.Keys(), testIssuer, testAudience, testNamespace, now)
		require.Error(t, err)
	})
}

func TestLooksLikePodToken(t *testing.T) {
	t.Parallel()
	require.True(t, workloads.LooksLikePodToken("a.b.c"))
	require.False(t, workloads.LooksLikePodToken(uuid.NewString()))
}

func TestRegistry(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	newOperator := func(c *workloadstest.Cluster, name string) workloads.Operator {
		return workloads.Operator{
			OrganizationID: orgID,
			Name:           name,
			Namespace:      testNamespace,
			Issuer:         testIssuer,
			Audience:       testAudience,
			Keys:           c.Keys(),
		}
	}

	t.Run("VerifyPodTokenUsesLiveOperatorKeys", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t)
		registry := workloads.NewRegistry(clock)
		trusted := workloadstest.NewCluster(t)
		token := trusted.Sign(workloadstest.ValidPodToken(clock.Now(), podName, podUID))

		_, _, err := registry.VerifyPodToken(token)
		require.Error(t, err, "no operator has vouched for this cluster yet")

		operator := newOperator(trusted, "default")
		registry.SyncOperator(operator, nil)
		claims, got, err := registry.VerifyPodToken(token)
		require.NoError(t, err)
		require.Equal(t, operator.Key(), got.Key())
		require.Equal(t, "workload-a", claims.PodName)

		// Keys from another cluster with the same issuer do not verify it.
		impostor := workloads.NewRegistry(clock)
		impostor.SyncOperator(newOperator(workloadstest.NewCluster(t), "default"), nil)
		_, _, err = impostor.VerifyPodToken(token)
		require.Error(t, err)

		// An operator that stops syncing stops vouching.
		clock.Advance(workloads.OperatorTTL + time.Second)
		_, _, err = registry.VerifyPodToken(trusted.Sign(workloadstest.ValidPodToken(clock.Now(), podName, podUID)))
		require.Error(t, err)
	})

	t.Run("ShapesAndPods", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t)
		registry := workloads.NewRegistry(clock)
		operator := newOperator(workloadstest.NewCluster(t), "default")

		workspaceID := uuid.New()
		shape := workloads.Shape{AgentID: uuid.New(), AgentName: "workload", WorkspaceID: workspaceID, Deployment: "shape"}
		registry.SyncOperator(operator, []workloads.Shape{shape})

		got, gotOperator, ok := registry.Shape(shape.AgentID)
		require.True(t, ok)
		require.Equal(t, operator.Key(), gotOperator.Key())
		require.Equal(t, operator.Key(), got.OperatorKey)
		require.Len(t, registry.ShapesForWorkspace(workspaceID), 1)

		agentA, agentB := uuid.New(), uuid.New()
		registry.SetPods(operator.Key(), map[string]uuid.UUID{"uid-a": agentA, "uid-b": agentB})
		id, key, ok := registry.AgentForPod("uid-a")
		require.True(t, ok)
		require.Equal(t, agentA, id)
		require.Equal(t, operator.Key(), key)

		// A sync replaces the operator's pods and shapes: what it no longer
		// reports is no longer trusted.
		registry.SetPods(operator.Key(), map[string]uuid.UUID{"uid-b": agentB})
		_, _, ok = registry.AgentForPod("uid-a")
		require.False(t, ok)
		registry.SyncOperator(operator, nil)
		_, _, ok = registry.Shape(shape.AgentID)
		require.False(t, ok)
	})

	t.Run("Requests", func(t *testing.T) {
		t.Parallel()
		registry := workloads.NewRegistry(quartz.NewMock(t))
		workspaceID := uuid.New()
		first := workloads.Request{ID: uuid.New(), WorkspaceID: workspaceID, CreatedAt: time.Unix(1, 0)}
		second := workloads.Request{ID: uuid.New(), WorkspaceID: workspaceID, CreatedAt: time.Unix(2, 0)}
		registry.AddRequest(second)
		registry.AddRequest(first)
		registry.AddRequest(workloads.Request{ID: uuid.New(), WorkspaceID: uuid.New()})

		got := registry.Requests(func(r workloads.Request) bool { return r.WorkspaceID == workspaceID })
		require.Len(t, got, 2)
		require.Equal(t, first.ID, got[0].ID)

		taken, ok := registry.TakeRequest(first.ID)
		require.True(t, ok)
		require.Equal(t, first.ID, taken.ID)
		_, ok = registry.Request(first.ID)
		require.False(t, ok)
	})
}
