package workloadoperator

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

// Options configure an Operator.
type Options struct {
	Client         *codersdk.Client
	Kube           *KubeClient
	Logger         slog.Logger
	OrganizationID uuid.UUID
	// Name identifies the operator to coderd and labels its pods.
	Name      string
	Namespace string
	Pod       PodOptions
	// Interval is the time between syncs with coderd.
	Interval time.Duration
}

// Operator keeps the workload pods of one namespace in line with the
// workloads coderd wants.
type Operator struct {
	opts Options

	mu sync.Mutex
	// created records the UIDs of the pods this process created, and when.
	// Only these pods are offered to coderd as the home of a new workload
	// identity: a label alone proves nothing about who made a pod.
	created map[string]time.Time
}

// New returns an operator. Call Run to start it.
func New(opts Options) (*Operator, error) {
	if opts.Client == nil || opts.Kube == nil {
		return nil, xerrors.New("a Coder client and a Kubernetes client are required")
	}
	if opts.Name == "" || opts.Namespace == "" || opts.Pod.Audience == "" || opts.Pod.ServiceAccount == "" {
		return nil, xerrors.New("name, namespace, audience and workload service account are required")
	}
	if opts.Interval <= 0 {
		opts.Interval = 2 * time.Second
	}
	opts.Pod.OperatorName = opts.Name
	return &Operator{opts: opts, created: make(map[string]time.Time)}, nil
}

// Run syncs until the context is canceled. A failed sync is logged and
// retried; it never deletes pods.
func (o *Operator) Run(ctx context.Context) error {
	o.opts.Logger.Info(ctx, "workload operator started",
		slog.F("name", o.opts.Name), slog.F("namespace", o.opts.Namespace))
	ticker := time.NewTicker(o.opts.Interval)
	defer ticker.Stop()
	for {
		if err := o.Sync(ctx); err != nil && ctx.Err() == nil {
			o.opts.Logger.Warn(ctx, "workload sync failed", slog.Error(err))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// ObservedPod is a pod in the namespace that carries the operator's label.
type ObservedPod struct {
	Name           string
	UID            string
	WorkloadID     uuid.UUID
	ServiceAccount string
	Phase          string
	// Terminating pods no longer count as running a workload, but their
	// names are still taken.
	Terminating bool
}

// Plan is what the operator must change so that every wanted workload has
// exactly one pod, and that pod is one the operator created.
type Plan struct {
	// Delete lists pods by name.
	Delete []string
	Create []codersdk.WorkloadOperatorWorkload
}

// PlanPods compares the workloads coderd wants with the labeled pods in
// the namespace. created holds the UIDs of the pods this operator made.
//
// A pod survives only if coderd has bound its workload to it, or if the
// workload is not bound yet and the operator made the pod. Any other pod
// that claims a workload is deleted: someone else created it.
func PlanPods(desired []codersdk.WorkloadOperatorWorkload, pods []ObservedPod, created map[string]struct{}) Plan {
	var plan Plan
	names := make(map[string]struct{}, len(pods))
	byWorkload := make(map[uuid.UUID][]ObservedPod, len(pods))
	for _, pod := range pods {
		names[pod.Name] = struct{}{}
		if !pod.Terminating {
			byWorkload[pod.WorkloadID] = append(byWorkload[pod.WorkloadID], pod)
		}
	}

	for _, workload := range desired {
		kept := false
		for _, pod := range byWorkload[workload.ID] {
			_, mine := created[pod.UID]
			switch {
			case workload.PodUID != "" && pod.UID == workload.PodUID:
				kept = true
			case workload.PodUID == "" && mine && !kept:
				kept = true
			default:
				plan.Delete = append(plan.Delete, pod.Name)
			}
		}
		delete(byWorkload, workload.ID)
		if kept || workload.PodUID != "" {
			// A bound workload whose pod is gone is not recreated. coderd
			// drops it on the next sync, and with it the identity.
			continue
		}
		if _, taken := names[PodName(workload)]; taken {
			// The name frees up once the pod holding it is gone.
			continue
		}
		plan.Create = append(plan.Create, workload)
	}

	// What remains claims workloads coderd does not want.
	for _, orphans := range byWorkload {
		for _, pod := range orphans {
			plan.Delete = append(plan.Delete, pod.Name)
		}
	}
	return plan
}

func observePod(pod object) ObservedPod {
	metadata, _ := pod["metadata"].(object)
	labels, _ := metadata["labels"].(object)
	spec, _ := pod["spec"].(object)
	status, _ := pod["status"].(object)
	observed := ObservedPod{}
	observed.Name, _ = metadata["name"].(string)
	observed.UID, _ = metadata["uid"].(string)
	observed.ServiceAccount, _ = spec["serviceAccountName"].(string)
	observed.Phase, _ = status["phase"].(string)
	_, observed.Terminating = metadata["deletionTimestamp"]
	rawID, _ := labels[LabelWorkloadID].(string)
	// A pod without a valid workload ID keeps the zero ID, which coderd
	// never wants, so the plan deletes it.
	observed.WorkloadID, _ = uuid.Parse(rawID)
	return observed
}

// createdSet returns the UIDs of the pods this process created. It forgets
// UIDs that have been absent from the namespace for a while.
func (o *Operator) createdSet(pods []ObservedPod) map[string]struct{} {
	o.mu.Lock()
	defer o.mu.Unlock()

	present := make(map[string]struct{}, len(pods))
	for _, pod := range pods {
		present[pod.UID] = struct{}{}
	}
	set := make(map[string]struct{}, len(o.created))
	for uid, at := range o.created {
		if _, ok := present[uid]; !ok && time.Since(at) > time.Minute {
			delete(o.created, uid)
			continue
		}
		set[uid] = struct{}{}
	}
	return set
}

// Sync reports the namespace to coderd once and reconciles its pods with
// the answer.
func (o *Operator) Sync(ctx context.Context) error {
	namespace := o.opts.Namespace

	issuer, jwks, err := o.opts.Kube.TokenKeys(ctx)
	if err != nil {
		return err
	}

	deployments, err := o.opts.Kube.Deployments(ctx, namespace, LabelShape+"=true")
	if err != nil {
		return xerrors.Errorf("list shapes: %w", err)
	}
	req := codersdk.WorkloadOperatorSyncRequest{
		Namespace: namespace,
		Issuer:    issuer,
		Audience:  o.opts.Pod.Audience,
		JWKS:      jwks,
		Shapes:    []codersdk.WorkloadOperatorShape{},
		Pods:      []codersdk.WorkloadOperatorPod{},
	}
	byName := make(map[string]object, len(deployments))
	for _, deployment := range deployments {
		name, _ := deployment["metadata"].(object)["name"].(string)
		token, err := ShapeAgentToken(deployment)
		if err != nil {
			o.opts.Logger.Warn(ctx, "skipping workload shape", slog.F("deployment", name), slog.Error(err))
			continue
		}
		byName[name] = deployment
		req.Shapes = append(req.Shapes, codersdk.WorkloadOperatorShape{Deployment: name, AgentToken: token})
	}

	rawPods, err := o.opts.Kube.Pods(ctx, namespace, LabelOperator+"="+o.opts.Name)
	if err != nil {
		return xerrors.Errorf("list workload pods: %w", err)
	}
	pods := make([]ObservedPod, 0, len(rawPods))
	for _, raw := range rawPods {
		pods = append(pods, observePod(raw))
	}
	created := o.createdSet(pods)
	for _, pod := range pods {
		if pod.Terminating || pod.WorkloadID == uuid.Nil {
			continue
		}
		_, mine := created[pod.UID]
		req.Pods = append(req.Pods, codersdk.WorkloadOperatorPod{
			WorkloadID:     pod.WorkloadID,
			Name:           pod.Name,
			UID:            pod.UID,
			ServiceAccount: pod.ServiceAccount,
			Phase:          pod.Phase,
			Created:        mine,
		})
	}

	resp, err := o.opts.Client.SyncWorkloadOperator(ctx, o.opts.OrganizationID, o.opts.Name, req)
	if err != nil {
		return xerrors.Errorf("sync with coderd: %w", err)
	}
	for _, shape := range resp.Shapes {
		if shape.Error != "" {
			o.opts.Logger.Warn(ctx, "coderd rejected shape",
				slog.F("deployment", shape.Deployment), slog.F("reason", shape.Error))
		}
	}

	plan := PlanPods(resp.Workloads, pods, created)
	for _, name := range plan.Delete {
		if err := o.opts.Kube.DeletePod(ctx, namespace, name); err != nil {
			o.opts.Logger.Error(ctx, "delete workload pod", slog.F("pod", name), slog.Error(err))
			continue
		}
		o.opts.Logger.Info(ctx, "deleted workload pod", slog.F("pod", name))
	}
	for _, workload := range plan.Create {
		deployment, ok := byName[workload.Deployment]
		if !ok {
			o.opts.Logger.Warn(ctx, "workload refers to an unknown shape",
				slog.F("workload", workload.Name), slog.F("deployment", workload.Deployment))
			continue
		}
		pod, err := BuildWorkloadPod(deployment, workload, o.opts.Pod)
		if err != nil {
			o.opts.Logger.Error(ctx, "build workload pod", slog.F("workload", workload.Name), slog.Error(err))
			continue
		}
		stored, err := o.opts.Kube.CreatePod(ctx, namespace, pod)
		if err != nil {
			var kubeErr *KubeError
			if xerrors.As(err, &kubeErr) && kubeErr.Status == http.StatusConflict {
				continue
			}
			o.opts.Logger.Error(ctx, "create workload pod", slog.F("workload", workload.Name), slog.Error(err))
			continue
		}
		uid, _ := stored["metadata"].(object)["uid"].(string)
		o.mu.Lock()
		o.created[uid] = time.Now()
		o.mu.Unlock()
		o.opts.Logger.Info(ctx, "created workload pod",
			slog.F("workload", workload.Name), slog.F("workload_id", workload.ID), slog.F("pod_uid", uid))
	}
	return nil
}
