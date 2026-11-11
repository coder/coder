package workloadoperator_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/workloadoperator"
	"github.com/coder/coder/v2/codersdk"
)

func TestPlanPods(t *testing.T) {
	t.Parallel()

	workload := func(name, podUID string) codersdk.WorkloadOperatorWorkload {
		return codersdk.WorkloadOperatorWorkload{ID: uuid.New(), Name: name, Deployment: "shape", PodUID: podUID}
	}
	pod := func(w codersdk.WorkloadOperatorWorkload, name, uid string) workloadoperator.ObservedPod {
		return workloadoperator.ObservedPod{Name: name, UID: uid, WorkloadID: w.ID}
	}
	set := func(uids ...string) map[string]struct{} {
		out := make(map[string]struct{}, len(uids))
		for _, uid := range uids {
			out[uid] = struct{}{}
		}
		return out
	}

	t.Run("CreatesPodForNewWorkload", func(t *testing.T) {
		t.Parallel()
		w := workload("a", "")
		plan := workloadoperator.PlanPods([]codersdk.WorkloadOperatorWorkload{w}, nil, set())
		require.Empty(t, plan.Delete)
		require.Equal(t, []codersdk.WorkloadOperatorWorkload{w}, plan.Create)
	})

	t.Run("KeepsOwnPodUntilBound", func(t *testing.T) {
		t.Parallel()
		w := workload("a", "")
		plan := workloadoperator.PlanPods(
			[]codersdk.WorkloadOperatorWorkload{w},
			[]workloadoperator.ObservedPod{pod(w, "workload-a", "mine")},
			set("mine"),
		)
		require.Empty(t, plan.Delete)
		require.Empty(t, plan.Create)
	})

	t.Run("KeepsBoundPod", func(t *testing.T) {
		t.Parallel()
		// After an operator restart the created set is empty. A pod coderd
		// has already bound must still survive.
		w := workload("a", "bound")
		plan := workloadoperator.PlanPods(
			[]codersdk.WorkloadOperatorWorkload{w},
			[]workloadoperator.ObservedPod{pod(w, "workload-a", "bound")},
			set(),
		)
		require.Empty(t, plan.Delete)
		require.Empty(t, plan.Create)
	})

	t.Run("DeletesForeignPodClaimingPendingWorkload", func(t *testing.T) {
		t.Parallel()
		// Someone else created a pod with a pending workload's labels. It
		// must not become that workload, and must not block the real pod
		// for longer than its own deletion takes.
		w := workload("a", "")
		plan := workloadoperator.PlanPods(
			[]codersdk.WorkloadOperatorWorkload{w},
			[]workloadoperator.ObservedPod{pod(w, "impostor", "foreign")},
			set(),
		)
		require.Equal(t, []string{"impostor"}, plan.Delete)
		require.Equal(t, []codersdk.WorkloadOperatorWorkload{w}, plan.Create)
	})

	t.Run("DeletesForeignPodClaimingBoundWorkload", func(t *testing.T) {
		t.Parallel()
		w := workload("a", "bound")
		plan := workloadoperator.PlanPods(
			[]codersdk.WorkloadOperatorWorkload{w},
			[]workloadoperator.ObservedPod{pod(w, "workload-a", "bound"), pod(w, "impostor", "foreign")},
			set("bound"),
		)
		require.Equal(t, []string{"impostor"}, plan.Delete)
		require.Empty(t, plan.Create)
	})

	t.Run("DoesNotRecreateBoundWorkloadWhosePodIsGone", func(t *testing.T) {
		t.Parallel()
		// The identity was bound to a pod that no longer exists. A new pod
		// would be a different identity, so coderd must drop the workload.
		w := workload("a", "gone")
		plan := workloadoperator.PlanPods([]codersdk.WorkloadOperatorWorkload{w}, nil, set())
		require.Empty(t, plan.Delete)
		require.Empty(t, plan.Create)
	})

	t.Run("WaitsForTerminatingPodName", func(t *testing.T) {
		t.Parallel()
		w := workload("a", "")
		terminating := pod(w, "workload-a", "old")
		terminating.Terminating = true
		plan := workloadoperator.PlanPods(
			[]codersdk.WorkloadOperatorWorkload{w},
			[]workloadoperator.ObservedPod{terminating},
			set("old"),
		)
		require.Empty(t, plan.Delete)
		require.Empty(t, plan.Create)
	})

	t.Run("DeletesPodsOfUnwantedWorkloads", func(t *testing.T) {
		t.Parallel()
		removed := workload("removed", "")
		plan := workloadoperator.PlanPods(
			nil,
			[]workloadoperator.ObservedPod{
				pod(removed, "workload-removed", "mine"),
				{Name: "no-workload-id", UID: "x"},
			},
			set("mine"),
		)
		require.ElementsMatch(t, []string{"workload-removed", "no-workload-id"}, plan.Delete)
		require.Empty(t, plan.Create)
	})
}
