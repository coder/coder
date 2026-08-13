package agentsocket

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/agent/agentsocket/proto"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/unit"
)

var _ proto.DRPCAgentSocketServer = (*DRPCAgentSocketService)(nil)

var (
	ErrUnitManagerNotAvailable    = xerrors.New("unit manager not available")
	ErrAgentAPINotConnected       = xerrors.New("agent not connected to coderd")
	ErrContextManagerNotAvailable = xerrors.New("context manager not available")
	ErrContextSourceNotFound      = xerrors.New("context source not found")
)

// ContextManager is the subset of *agentcontext.Manager the socket
// service needs to serve workspace-context source CRUD. It is an
// interface so tests can supply a fake.
type ContextManager interface {
	Sources() []agentcontext.Source
	HasSource(path string) (canonical string, ok bool)
	AddSource(s agentcontext.Source) (agentcontext.Source, error)
	RemoveSource(path string) error
	Snapshot() agentcontext.Snapshot
	Resync(ctx context.Context) (agentcontext.Snapshot, error)
}

// WorkspaceIdentity reports what the workspace_agent knows about itself, so
// that the service can check a caller's claims against it.
//
// Both are read when a call arrives rather than captured once. The manifest is
// replaced on reconnection and the credential can be refreshed, so a snapshot
// of either goes stale and would reject legitimate callers.
type WorkspaceIdentity interface {
	// WorkspaceID reports the workspace this workspace_agent serves. ok is
	// false until the first manifest has arrived.
	WorkspaceID() (id uuid.UUID, ok bool)
	// Credential reports the credential this workspace_agent currently
	// authenticates to the control plane with.
	Credential() string
}

// DRPCAgentSocketService implements the DRPC agent socket service.
type DRPCAgentSocketService struct {
	unitManager       *unit.Manager
	contextManager    ContextManager
	workspaceIdentity WorkspaceIdentity
	logger            slog.Logger

	mu       sync.Mutex
	agentAPI agentproto.DRPCAgentClient211
}

// SetAgentAPI sets the agent API client used to forward requests
// to coderd. This is called when the agent connects to coderd.
func (s *DRPCAgentSocketService) SetAgentAPI(api agentproto.DRPCAgentClient211) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentAPI = api
}

// ClearAgentAPI clears the agent API client. This is called when
// the agent disconnects from coderd.
func (s *DRPCAgentSocketService) ClearAgentAPI() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentAPI = nil
}

// Ping responds to a ping request to check if the service is alive.
func (*DRPCAgentSocketService) Ping(_ context.Context, _ *proto.PingRequest) (*proto.PingResponse, error) {
	return &proto.PingResponse{}, nil
}

// CreateAIAgent registers an AI agent created inside this workspace, by
// forwarding to coderd over the workspace_agent's own authenticated
// connection.
//
// The request this handler receives and the one it sends differ, and the
// difference is the point. A caller on the socket must state the workspace and
// present a credential, because the socket is local IPC with no authentication
// of its own. coderd needs neither: it resolved the workspace, its owner, and
// this workspace_agent while authenticating the connection being forwarded
// over. Passing them on would be redundant, and passing a caller's credential
// on would be worse than redundant.
//
// So this handler verifies, and then drops, both. Nothing crosses. What returns
// is the identifier coderd minted and the credential it issued, both of which
// this handler passes back untouched.
func (s *DRPCAgentSocketService) CreateAIAgent(ctx context.Context, req *proto.CreateAIAgentRequest) (*proto.CreateAIAgentResponse, error) {
	workspaceID, err := uuid.FromBytes(req.GetWorkspaceId())
	if err != nil {
		return nil, xerrors.Errorf("workspace_id is not a uuid: %w", err)
	}

	if err := s.verifyCaller(workspaceID, req.GetWorkspaceCredential()); err != nil {
		return nil, err
	}

	s.mu.Lock()
	api := s.agentAPI
	s.mu.Unlock()

	if api == nil {
		return nil, ErrAgentAPINotConnected
	}

	// The credential is deliberately absent from the log line. It is a
	// credential, and this is a log.
	s.logger.Info(ctx, "forwarding CreateAIAgent to coderd",
		slog.F("workspace_id", workspaceID))

	resp, err := api.CreateAIAgent(ctx, &agentproto.CreateAIAgentRequest{})
	if err != nil {
		return nil, xerrors.Errorf("forward CreateAIAgent to coderd: %w", err)
	}

	return &proto.CreateAIAgentResponse{
		Id:         resp.GetId(),
		Credential: resp.GetCredential(),
	}, nil
}

// verifyCaller checks a caller's claims against what the workspace_agent knows
// about itself.
//
// What this establishes is narrow, and worth stating so that it is not relied
// on for more. The workspace_agent exports its credential to every process it
// starts, so any of them can present it. A caller that passes this check has
// shown that it is inside this workspace. It has not shown which process it
// is, and nothing here distinguishes one from another.
//
// It fails closed. A workspace_agent that does not yet know its own identity
// cannot verify anyone, and answering as though it could would be worse than
// refusing.
func (s *DRPCAgentSocketService) verifyCaller(workspaceID uuid.UUID, credential []byte) error {
	if s.workspaceIdentity == nil {
		return xerrors.New("this workspace_agent cannot verify callers")
	}

	ownID, ok := s.workspaceIdentity.WorkspaceID()
	if !ok {
		return xerrors.New("this workspace_agent does not yet know its workspace")
	}
	if workspaceID != ownID {
		return xerrors.Errorf("workspace_id %s is not this workspace", workspaceID)
	}

	// The credential is an opaque blob. It is not parsed, inspected, or assumed
	// to have a shape, only compared. The comparison is constant time because
	// the operands are secrets, even though one of them is ambient here.
	ownCredential := s.workspaceIdentity.Credential()
	if ownCredential == "" {
		return xerrors.New("this workspace_agent has no credential to compare against")
	}
	if subtle.ConstantTimeCompare([]byte(ownCredential), credential) != 1 {
		return xerrors.New("workspace_credential does not match this workspace")
	}

	return nil
}

// SyncStart starts a unit in the dependency graph.
func (s *DRPCAgentSocketService) SyncStart(_ context.Context, req *proto.SyncStartRequest) (*proto.SyncStartResponse, error) {
	if s.unitManager == nil {
		return nil, xerrors.Errorf("SyncStart: %w", ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)

	if err := s.unitManager.Register(unitID); err != nil {
		if !errors.Is(err, unit.ErrUnitAlreadyRegistered) {
			return nil, xerrors.Errorf("SyncStart: %w", err)
		}
	}

	isReady, err := s.unitManager.IsReady(unitID)
	if err != nil {
		return nil, xerrors.Errorf("cannot check readiness: %w", err)
	}
	if !isReady {
		return nil, xerrors.Errorf("cannot start unit %q: unit not ready", req.Unit)
	}

	err = s.unitManager.UpdateStatus(unitID, unit.StatusStarted)
	if err != nil {
		return nil, xerrors.Errorf("cannot start unit %q: %w", req.Unit, err)
	}

	return &proto.SyncStartResponse{}, nil
}

// SyncWant declares a dependency between units.
func (s *DRPCAgentSocketService) SyncWant(_ context.Context, req *proto.SyncWantRequest) (*proto.SyncWantResponse, error) {
	if s.unitManager == nil {
		return nil, xerrors.Errorf("cannot add dependency: %w", ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)
	dependsOnID := unit.ID(req.DependsOn)

	if err := s.unitManager.Register(unitID); err != nil && !errors.Is(err, unit.ErrUnitAlreadyRegistered) {
		return nil, xerrors.Errorf("cannot add dependency: %w", err)
	}

	if err := s.unitManager.AddDependency(unitID, dependsOnID, unit.StatusComplete); err != nil {
		return nil, xerrors.Errorf("cannot add dependency: %w", err)
	}

	return &proto.SyncWantResponse{}, nil
}

// SyncComplete marks a unit as complete in the dependency graph.
func (s *DRPCAgentSocketService) SyncComplete(_ context.Context, req *proto.SyncCompleteRequest) (*proto.SyncCompleteResponse, error) {
	if s.unitManager == nil {
		return nil, xerrors.Errorf("cannot complete unit: %w", ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)

	if err := s.unitManager.UpdateStatus(unitID, unit.StatusComplete); err != nil {
		return nil, xerrors.Errorf("cannot complete unit %q: %w", req.Unit, err)
	}

	return &proto.SyncCompleteResponse{}, nil
}

// SyncReady checks whether a unit is ready to be started. That is, all dependencies are satisfied.
func (s *DRPCAgentSocketService) SyncReady(_ context.Context, req *proto.SyncReadyRequest) (*proto.SyncReadyResponse, error) {
	if s.unitManager == nil {
		return nil, xerrors.Errorf("cannot check readiness: %w", ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)
	isReady, err := s.unitManager.IsReady(unitID)
	if err != nil {
		return nil, xerrors.Errorf("cannot check readiness: %w", err)
	}

	return &proto.SyncReadyResponse{
		Ready: isReady,
	}, nil
}

// SyncStatus gets the status of a unit and lists its dependencies.
func (s *DRPCAgentSocketService) SyncStatus(_ context.Context, req *proto.SyncStatusRequest) (*proto.SyncStatusResponse, error) {
	if s.unitManager == nil {
		return nil, xerrors.Errorf("cannot get status for unit %q: %w", req.Unit, ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)

	isReady, err := s.unitManager.IsReady(unitID)
	if err != nil {
		return nil, xerrors.Errorf("cannot check readiness: %w", err)
	}

	dependencies, err := s.unitManager.GetAllDependencies(unitID)
	switch {
	case errors.Is(err, unit.ErrUnitNotFound):
		dependencies = []unit.Dependency{}
	case err != nil:
		return nil, xerrors.Errorf("cannot get dependencies: %w", err)
	}

	var depInfos []*proto.DependencyInfo
	for _, dep := range dependencies {
		depInfos = append(depInfos, &proto.DependencyInfo{
			Unit:           string(dep.Unit),
			DependsOn:      string(dep.DependsOn),
			RequiredStatus: string(dep.RequiredStatus),
			CurrentStatus:  string(dep.CurrentStatus),
			IsSatisfied:    dep.IsSatisfied,
		})
	}

	u, err := s.unitManager.Unit(unitID)
	if err != nil {
		return nil, xerrors.Errorf("cannot get status for unit %q: %w", req.Unit, err)
	}
	return &proto.SyncStatusResponse{
		Status:       string(u.Status()),
		IsReady:      isReady,
		Dependencies: depInfos,
	}, nil
}

// SyncList returns all registered units and their current statuses.
func (s *DRPCAgentSocketService) SyncList(_ context.Context, _ *proto.SyncListRequest) (*proto.SyncListResponse, error) {
	if s.unitManager == nil {
		return nil, xerrors.Errorf("cannot list units: %w", ErrUnitManagerNotAvailable)
	}

	units := s.unitManager.ListUnits()
	var unitInfos []*proto.UnitInfo
	for _, u := range units {
		isReady, err := s.unitManager.IsReady(u.ID())
		if err != nil {
			return nil, xerrors.Errorf("cannot check readiness for unit %q: %w", u.ID(), err)
		}
		unitInfos = append(unitInfos, &proto.UnitInfo{
			Unit:    string(u.ID()),
			Status:  string(u.Status()),
			IsReady: isReady,
		})
	}

	return &proto.SyncListResponse{Units: unitInfos}, nil
}

// UpdateAppStatus forwards an app status update to coderd via the
// agent API. Returns an error if the agent is not connected.
func (s *DRPCAgentSocketService) UpdateAppStatus(ctx context.Context, req *agentproto.UpdateAppStatusRequest) (*agentproto.UpdateAppStatusResponse, error) {
	s.mu.Lock()
	api := s.agentAPI
	s.mu.Unlock()

	if api == nil {
		return nil, ErrAgentAPINotConnected
	}
	return api.UpdateAppStatus(ctx, req)
}

// ContextSources lists the workspace-context sources registered on the agent.
func (s *DRPCAgentSocketService) ContextSources(_ context.Context, _ *proto.ContextSourcesRequest) (*proto.ContextSourcesResponse, error) {
	if s.contextManager == nil {
		return nil, ErrContextManagerNotAvailable
	}
	sources := s.contextManager.Sources()
	out := &proto.ContextSourcesResponse{Sources: make([]*proto.ContextSource, 0, len(sources))}
	for _, src := range sources {
		out.Sources = append(out.Sources, &proto.ContextSource{Path: src.Path})
	}
	return out, nil
}

// GetContextSource returns a single registered source, canonicalizing the
// requested path before matching.
func (s *DRPCAgentSocketService) GetContextSource(_ context.Context, req *proto.GetContextSourceRequest) (*proto.GetContextSourceResponse, error) {
	if s.contextManager == nil {
		return nil, ErrContextManagerNotAvailable
	}
	canonical, ok := s.contextManager.HasSource(req.Path)
	if !ok {
		return nil, xerrors.Errorf("%q: %w", req.Path, ErrContextSourceNotFound)
	}
	return &proto.GetContextSourceResponse{Source: &proto.ContextSource{Path: canonical}}, nil
}

// AddContextSource registers a new scan root and triggers a re-resolve.
func (s *DRPCAgentSocketService) AddContextSource(_ context.Context, req *proto.AddContextSourceRequest) (*proto.AddContextSourceResponse, error) {
	if s.contextManager == nil {
		return nil, ErrContextManagerNotAvailable
	}
	src, err := s.contextManager.AddSource(agentcontext.Source{Path: req.Path})
	if err != nil {
		return nil, xerrors.Errorf("add context source: %w", err)
	}
	return &proto.AddContextSourceResponse{Source: &proto.ContextSource{Path: src.Path}}, nil
}

// RemoveContextSource removes a previously-registered scan root.
func (s *DRPCAgentSocketService) RemoveContextSource(_ context.Context, req *proto.RemoveContextSourceRequest) (*proto.RemoveContextSourceResponse, error) {
	if s.contextManager == nil {
		return nil, ErrContextManagerNotAvailable
	}
	if err := s.contextManager.RemoveSource(req.Path); err != nil {
		if errors.Is(err, agentcontext.ErrSourceNotFound) {
			return nil, xerrors.Errorf("%q: %w", req.Path, ErrContextSourceNotFound)
		}
		return nil, xerrors.Errorf("remove context source: %w", err)
	}
	return &proto.RemoveContextSourceResponse{}, nil
}

// GetContextSnapshot returns the agent's current resolved snapshot without
// forcing a re-walk.
func (s *DRPCAgentSocketService) GetContextSnapshot(_ context.Context, _ *proto.ContextSnapshotRequest) (*proto.ContextSnapshotResponse, error) {
	if s.contextManager == nil {
		return nil, ErrContextManagerNotAvailable
	}
	return &proto.ContextSnapshotResponse{Snapshot: contextSnapshotToProto(s.contextManager.Snapshot())}, nil
}

// ResyncContext forces a re-walk and synchronous push, returning the
// resulting snapshot. Callers use it as a barrier before fanning out a
// refresh.
func (s *DRPCAgentSocketService) ResyncContext(ctx context.Context, _ *proto.ResyncContextRequest) (*proto.ResyncContextResponse, error) {
	if s.contextManager == nil {
		return nil, ErrContextManagerNotAvailable
	}
	snap, err := s.contextManager.Resync(ctx)
	if err != nil {
		return nil, xerrors.Errorf("resync context: %w", err)
	}
	return &proto.ResyncContextResponse{Snapshot: contextSnapshotToProto(snap)}, nil
}

// contextSnapshotToProto converts an agentcontext.Snapshot to its on-wire
// form. Payload bytes are intentionally omitted; they reach coderd via the
// drpc PushContextState path. Keep the per-resource field mapping in sync
// with snapshotResponse in agent/agentcontext/api.go.
func contextSnapshotToProto(s agentcontext.Snapshot) *proto.ContextSnapshot {
	out := &proto.ContextSnapshot{
		Version:       s.Version,
		AggregateHash: hex.EncodeToString(s.AggregateHash[:]),
		Resources:     make([]*proto.ContextResource, 0, len(s.Resources)),
		PayloadBytes:  s.PayloadBytes,
		SnapshotError: s.SnapshotError,
	}
	for _, r := range s.Resources {
		out.Resources = append(out.Resources, &proto.ContextResource{
			Id:          r.ID,
			Kind:        r.Kind.String(),
			Source:      r.Source,
			SourcePath:  r.SourcePath,
			ContentHash: hex.EncodeToString(r.ContentHash[:]),
			SizeBytes:   r.SizeBytes,
			Status:      r.Status.String(),
			Error:       r.Error,
			Name:        r.Name,
			Description: r.Description,
		})
	}
	return out
}
