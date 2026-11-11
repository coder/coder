package workloads

import (
	"slices"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

// OperatorTTL is how long an operator stays usable after its last sync.
const OperatorTTL = 30 * time.Second

// Operator is a workload operator that has synced with coderd.
type Operator struct {
	OrganizationID uuid.UUID
	Name           string
	Namespace      string
	// Issuer, Audience and Keys verify pod tokens from the operator's
	// cluster.
	Issuer   string
	Audience string
	Keys     jose.JSONWebKeySet
	LastSeen time.Time
}

// Key identifies an operator within the registry.
func (o Operator) Key() string {
	return OperatorKey(o.OrganizationID, o.Name)
}

// OperatorKey returns the registry key of an operator.
func OperatorKey(organizationID uuid.UUID, name string) string {
	return organizationID.String() + "/" + name
}

// Shape is a template-declared agent whose pod template an operator can
// clone.
type Shape struct {
	AgentID     uuid.UUID
	AgentName   string
	WorkspaceID uuid.UUID
	Deployment  string
	OperatorKey string
}

// Request is a workload a user asked for that has no pod yet.
type Request struct {
	ID                  uuid.UUID
	Name                string
	WorkspaceID         uuid.UUID
	ShapeAgentID        uuid.UUID
	Command             string
	RequestedByID       uuid.UUID
	RequestedByUsername string
	CreatedAt           time.Time
}

// PodMetadata is stored on a runtime agent row. It records the facts about
// its pod and its request that the agent row has no columns for.
type PodMetadata struct {
	Namespace           string    `json:"namespace"`
	PodName             string    `json:"pod_name"`
	PodUID              string    `json:"pod_uid"`
	ServiceAccount      string    `json:"service_account"`
	Operator            string    `json:"operator"`
	Shape               string    `json:"shape"`
	Command             string    `json:"command,omitempty"`
	RequestedByID       uuid.UUID `json:"requested_by_id"`
	RequestedByUsername string    `json:"requested_by_username"`
	// RequestedAt is when `coder run` was called. The agent row is created
	// later, once the pod exists.
	RequestedAt time.Time `json:"requested_at"`
}

// Registry holds operators, their shapes, and pending workload requests.
type Registry struct {
	clock quartz.Clock

	mu        sync.Mutex
	operators map[string]Operator
	shapes    map[uuid.UUID]Shape
	pending   map[uuid.UUID]Request
	// pods maps a pod UID to the runtime agent bound to it. It is rebuilt
	// from agent rows on every operator sync.
	pods map[string]podBinding
}

type podBinding struct {
	agentID     uuid.UUID
	operatorKey string
}

// NewRegistry returns an empty registry.
func NewRegistry(clock quartz.Clock) *Registry {
	return &Registry{
		clock:     clock,
		operators: make(map[string]Operator),
		shapes:    make(map[uuid.UUID]Shape),
		pending:   make(map[uuid.UUID]Request),
		pods:      make(map[string]podBinding),
	}
}

// SetPods replaces the pod bindings of an operator. The map is keyed by pod
// UID.
func (r *Registry) SetPods(operatorKey string, pods map[string]uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for uid, binding := range r.pods {
		if binding.operatorKey == operatorKey {
			delete(r.pods, uid)
		}
	}
	for uid, agentID := range pods {
		r.pods[uid] = podBinding{agentID: agentID, operatorKey: operatorKey}
	}
}

// AgentForPod returns the runtime agent bound to a pod and the key of the
// operator that reported the pod.
func (r *Registry) AgentForPod(podUID string) (uuid.UUID, string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	binding, ok := r.pods[podUID]
	return binding.agentID, binding.operatorKey, ok
}

// SyncOperator records an operator and replaces the shapes it serves.
func (r *Registry) SyncOperator(operator Operator, shapes []Shape) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := operator.Key()
	operator.LastSeen = r.clock.Now()
	r.operators[key] = operator
	for id, shape := range r.shapes {
		if shape.OperatorKey == key {
			delete(r.shapes, id)
		}
	}
	for _, shape := range shapes {
		shape.OperatorKey = key
		r.shapes[shape.AgentID] = shape
	}
}

func (r *Registry) liveLocked(key string) (Operator, bool) {
	operator, ok := r.operators[key]
	if !ok || r.clock.Now().Sub(operator.LastSeen) > OperatorTTL {
		return Operator{}, false
	}
	return operator, true
}

// Shape returns the shape cloned from an agent and its operator, if a live
// operator serves it.
func (r *Registry) Shape(agentID uuid.UUID) (Shape, Operator, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	shape, ok := r.shapes[agentID]
	if !ok {
		return Shape{}, Operator{}, false
	}
	operator, ok := r.liveLocked(shape.OperatorKey)
	if !ok {
		return Shape{}, Operator{}, false
	}
	return shape, operator, true
}

// ShapesForWorkspace returns the workspace's shapes that a live operator
// serves, ordered by agent name.
func (r *Registry) ShapesForWorkspace(workspaceID uuid.UUID) []Shape {
	r.mu.Lock()
	defer r.mu.Unlock()

	var shapes []Shape
	for _, shape := range r.shapes {
		if shape.WorkspaceID != workspaceID {
			continue
		}
		if _, ok := r.liveLocked(shape.OperatorKey); ok {
			shapes = append(shapes, shape)
		}
	}
	slices.SortFunc(shapes, func(a, b Shape) int {
		return compareStrings(a.AgentName, b.AgentName)
	})
	return shapes
}

// ShapesForOperator returns the shapes an operator serves.
func (r *Registry) ShapesForOperator(operatorKey string) []Shape {
	r.mu.Lock()
	defer r.mu.Unlock()

	var shapes []Shape
	for _, shape := range r.shapes {
		if shape.OperatorKey == operatorKey {
			shapes = append(shapes, shape)
		}
	}
	slices.SortFunc(shapes, func(a, b Shape) int {
		return compareStrings(a.Deployment, b.Deployment)
	})
	return shapes
}

// AddRequest records a workload that needs a pod.
func (r *Registry) AddRequest(req Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending[req.ID] = req
}

// TakeRequest removes and returns a pending request.
func (r *Registry) TakeRequest(id uuid.UUID) (Request, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.pending[id]
	delete(r.pending, id)
	return req, ok
}

// Request returns a pending request.
func (r *Registry) Request(id uuid.UUID) (Request, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.pending[id]
	return req, ok
}

// Requests returns the pending requests that match keep, oldest first.
func (r *Registry) Requests(keep func(Request) bool) []Request {
	r.mu.Lock()
	defer r.mu.Unlock()

	var reqs []Request
	for _, req := range r.pending {
		if keep(req) {
			reqs = append(reqs, req)
		}
	}
	slices.SortFunc(reqs, func(a, b Request) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return compareStrings(a.ID.String(), b.ID.String())
	})
	return reqs
}

// VerifyPodToken verifies a pod token against the keys of every live
// operator that serves the token's issuer and namespace. It returns the pod
// and the operator whose cluster vouches for it.
func (r *Registry) VerifyPodToken(raw string) (PodClaims, Operator, error) {
	unverified, err := PeekPodToken(raw)
	if err != nil {
		return PodClaims{}, Operator{}, err
	}

	r.mu.Lock()
	var candidates []Operator
	for key, operator := range r.operators {
		if operator.Issuer != unverified.Issuer || operator.Namespace != unverified.Namespace {
			continue
		}
		if live, ok := r.liveLocked(key); ok {
			candidates = append(candidates, live)
		}
	}
	now := r.clock.Now()
	r.mu.Unlock()

	if len(candidates) == 0 {
		return PodClaims{}, Operator{}, xerrors.New("no workload operator serves this pod token's cluster and namespace")
	}
	for _, operator := range candidates {
		claims, verifyErr := VerifyPodToken(raw, operator.Keys, operator.Issuer, operator.Audience, operator.Namespace, now)
		if verifyErr == nil {
			return claims, operator, nil
		}
		err = verifyErr
	}
	return PodClaims{}, Operator{}, err
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
