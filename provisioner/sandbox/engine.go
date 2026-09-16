package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

// State is durable native provisioner state. AgentToken is a credential.
type State struct {
	Version     int                       `json:"version"`
	WorkspaceID string                    `json:"workspace_id"`
	BuildID     string                    `json:"build_id"`
	BuildNumber int32                     `json:"build_number"`
	RuntimeID   string                    `json:"runtime_id,omitempty"`
	AgentToken  string                    `json:"agent_token,omitempty"`
	Manifest    Manifest                  `json:"manifest"`
	Phase       string                    `json:"phase"`
	Transition  proto.WorkspaceTransition `json:"transition"`
}

// Engine serializes workspace operations across provisioner workers using
// a shared, persistent host directory. Closing a session never stops compute.
type Engine struct {
	runtime        Runtime
	directory      string
	cleanupTimeout time.Duration
}

// NewEngine prepares the durable operation journal outside session storage.
func NewEngine(runtime Runtime, directory string) (*Engine, error) {
	if runtime == nil {
		return nil, xerrors.New("sandbox runtime is required")
	}
	if !filepath.IsAbs(directory) {
		return nil, xerrors.New("sandbox state directory must be absolute")
	}
	if err := os.MkdirAll(filepath.Join(directory, "operations"), 0o700); err != nil {
		return nil, xerrors.Errorf("create sandbox journal: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "locks"), 0o700); err != nil {
		return nil, xerrors.Errorf("create sandbox locks: %w", err)
	}
	return &Engine{runtime: runtime, directory: directory, cleanupTimeout: 10 * time.Second}, nil
}

func validID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u != uuid.Nil && u.String() == id
}

// DecodeState validates the identity before any state can address host data.
func DecodeState(data []byte, workspaceID string) (State, error) {
	var s State
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, xerrors.Errorf("decode sandbox state: %w", err)
	}
	if s.Version != 1 || s.BuildNumber < 1 || !validID(s.WorkspaceID) || !validID(s.BuildID) || (workspaceID != "" && s.WorkspaceID != workspaceID) {
		return s, xerrors.New("sandbox state version or workspace identity is invalid")
	}
	if s.RuntimeID != "" && s.RuntimeID != "coder-sandbox-"+s.BuildID {
		return s, xerrors.New("sandbox runtime identity is invalid")
	}
	return s, nil
}

// Apply performs a single authorized lifecycle operation. A replay of the same
// build cannot create a second sandbox or revive a retired generation.
func (e *Engine) Apply(ctx context.Context, m Manifest, metadata *proto.Metadata) (State, error) {
	return e.execute(ctx, m, metadata, nil)
}

func (e *Engine) execute(ctx context.Context, m Manifest, metadata *proto.Metadata, runtimeTimings *[]*proto.Timing) (State, error) {
	var empty State
	if metadata == nil || !validID(metadata.WorkspaceId) || !validID(metadata.WorkspaceBuildId) {
		return empty, xerrors.New("sandbox apply requires canonical workspace and build IDs")
	}
	if metadata.WorkspaceBuildNumber < 1 {
		return empty, xerrors.New("sandbox apply requires a positive workspace build number")
	}
	if metadata.WorkspaceTransition != proto.WorkspaceTransition_START && metadata.WorkspaceTransition != proto.WorkspaceTransition_STOP && metadata.WorkspaceTransition != proto.WorkspaceTransition_DESTROY {
		return empty, xerrors.New("unsupported sandbox transition")
	}
	lock := flock.New(filepath.Join(e.directory, "locks", metadata.WorkspaceId+".lock"))
	locked, err := lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil {
		return empty, xerrors.Errorf("lock sandbox workspace: %w", err)
	}
	if !locked {
		return empty, xerrors.Errorf("lock sandbox workspace: %w", ctx.Err())
	}
	defer lock.Close()

	file := e.operationPath(metadata.WorkspaceId, metadata.WorkspaceBuildId)
	s := State{Version: 1, WorkspaceID: metadata.WorkspaceId, BuildID: metadata.WorkspaceBuildId, BuildNumber: metadata.WorkspaceBuildNumber, Manifest: m, Phase: "intent", Transition: metadata.WorkspaceTransition}
	data, err := os.ReadFile(file)
	hasJournal := err == nil
	if err == nil {
		s, err = DecodeState(data, metadata.WorkspaceId)
		if err != nil {
			return empty, err
		}
		if s.BuildID != metadata.WorkspaceBuildId || s.BuildNumber != metadata.WorkspaceBuildNumber || s.Manifest != m || s.Transition != metadata.WorkspaceTransition {
			return s, xerrors.New("sandbox operation identity or configuration changed")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return empty, xerrors.Errorf("read sandbox operation: %w", err)
	}
	allocated, err := e.checkGeneration(ctx, s)
	if err != nil {
		return empty, err
	}
	if allocated && !hasJournal {
		// Labels cannot recover the original credential. Keep the allocation
		// until a new authorized build can replace it.
		return empty, xerrors.New("sandbox allocation has no matching operation journal; create a new build")
	}
	if hasJournal {
		if s.Phase == "superseded" {
			return s, xerrors.New("sandbox build has been superseded")
		}
		if s.Phase == "stopped" {
			if s.Transition == proto.WorkspaceTransition_START {
				return s, xerrors.New("sandbox start operation has been retired")
			}
			return s, nil
		}
	} else {
		if s.Transition == proto.WorkspaceTransition_START {
			s.RuntimeID = "coder-sandbox-" + s.BuildID
			s.AgentToken = uuid.NewString()
		}
		if err := e.save(s); err != nil {
			return s, err
		}
	}
	if s.Phase == "cleanup" {
		if err := e.cleanup(&s); err != nil {
			return s, err
		}
		return s, xerrors.New("sandbox start was canceled or failed; create a new build")
	}

	if s.Transition != proto.WorkspaceTransition_START {
		if err := e.removePrior(ctx, s); err != nil {
			return s, err
		}
		s.Phase = "stopped"
		return s, e.save(s)
	}
	if s.Phase == "running" {
		running, err := e.runtime.Inspect(ctx, s.RuntimeID)
		if err != nil {
			return s, err
		}
		if !running {
			return s, xerrors.New("sandbox runtime is no longer running; create a new build")
		}
		return s, nil
	}
	if err := e.removePrior(ctx, s); err != nil {
		return s, err
	}
	if err := ctx.Err(); err != nil {
		return s, err
	}
	creationStarted := time.Now()
	err = e.runtime.Start(ctx, RuntimeSpec{
		ID: s.RuntimeID, WorkspaceID: s.WorkspaceID, BuildID: s.BuildID, BuildNumber: s.BuildNumber,
		Image: m.Image, CPU: m.CPU, MemoryBytes: m.MemoryMiB << 20, Workdir: m.Workdir,
		AgentToken: s.AgentToken, CoderURL: metadata.CoderUrl,
	})
	if runtimeTimings != nil {
		entries := timing(creationStarted, "apply", "create", err)
		entries[0].Source = "containerd"
		entries[0].Resource = s.RuntimeID
		*runtimeTimings = append(*runtimeTimings, entries...)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		if errors.Is(err, ErrRuntimeOwnership) {
			return s, err
		}
		s.Phase = "cleanup"
		journalErr := e.save(s)
		cleanupErr := e.cleanup(&s)
		return s, errors.Join(xerrors.Errorf("start sandbox: %w", err), journalErr, cleanupErr)
	}
	s.Phase = "running"
	if err := e.save(s); err != nil {
		// The intent and token are already durable. Retain the allocation so
		// a retry can inspect it instead of creating a duplicate.
		return s, err
	}
	return s, nil
}

// checkGeneration also fences a first delivery of an old request that never
// wrote a journal. The build number comes from coderd's persisted build.
func (e *Engine) checkGeneration(ctx context.Context, current State) (bool, error) {
	dir := filepath.Dir(e.operationPath(current.WorkspaceID, current.BuildID))
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, xerrors.Errorf("read sandbox generations: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return false, err
		}
		prior, err := DecodeState(data, current.WorkspaceID)
		if err != nil {
			return false, err
		}
		if prior.BuildNumber > current.BuildNumber || (prior.BuildNumber == current.BuildNumber && prior.BuildID != current.BuildID) {
			return false, xerrors.Errorf("sandbox build has been superseded by generation %d", prior.BuildNumber)
		}
	}
	allocations, err := e.runtime.List(ctx, current.WorkspaceID)
	if err != nil {
		return false, xerrors.Errorf("recover sandbox inventory: %w", err)
	}
	allocated := false
	for _, allocation := range allocations {
		if allocation.WorkspaceID != current.WorkspaceID || !validID(allocation.BuildID) || allocation.BuildNumber < 1 || allocation.ID != "coder-sandbox-"+allocation.BuildID {
			return false, xerrors.New("runtime returned an allocation with invalid ownership")
		}
		if allocation.BuildNumber > current.BuildNumber || (allocation.BuildNumber == current.BuildNumber && allocation.BuildID != current.BuildID) {
			return false, xerrors.Errorf("sandbox build has been superseded by runtime generation %d", allocation.BuildNumber)
		}
		if allocation.BuildID == current.BuildID {
			allocated = true
		}
		if allocation.BuildID == current.BuildID && (allocation.BuildNumber != current.BuildNumber || allocation.Image != current.Manifest.Image) {
			// Labels cannot recover the original agent credential. Retain the
			// allocation until a new authorized build can replace it.
			return false, xerrors.New("sandbox allocation has no matching operation journal; create a new build")
		}
	}
	return allocated, nil
}

func (e *Engine) operationPath(workspaceID, buildID string) string {
	return filepath.Join(e.directory, "operations", workspaceID, buildID+".json")
}

func (e *Engine) save(s State) error {
	data, err := json.Marshal(s)
	if err != nil {
		return xerrors.Errorf("encode sandbox operation: %w", err)
	}
	file := e.operationPath(s.WorkspaceID, s.BuildID)
	if err := writeAtomic(file, data); err != nil {
		return xerrors.Errorf("persist sandbox operation: %w", err)
	}
	return nil
}

func writeAtomic(file string, data []byte) error {
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".operation-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), file); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (e *Engine) cleanup(s *State) error {
	ctx, cancel := context.WithTimeout(context.Background(), e.cleanupTimeout)
	defer cancel()
	if err := e.runtime.Delete(ctx, s.RuntimeID); err != nil {
		return xerrors.Errorf("clean sandbox allocation (record retained for retry): %w", err)
	}
	s.Phase = "stopped"
	s.AgentToken = ""
	return e.save(*s)
}

func (e *Engine) removePrior(ctx context.Context, current State) error {
	allocations, err := e.runtime.List(ctx, current.WorkspaceID)
	if err != nil {
		return xerrors.Errorf("inventory workspace sandboxes: %w", err)
	}
	for _, allocation := range allocations {
		if allocation.WorkspaceID != current.WorkspaceID || !validID(allocation.BuildID) || allocation.ID != "coder-sandbox-"+allocation.BuildID {
			return xerrors.New("runtime returned an allocation with invalid ownership")
		}
		if allocation.BuildID == current.BuildID {
			continue
		}
		if err := e.runtime.Delete(ctx, allocation.ID); err != nil {
			return xerrors.Errorf("remove prior sandbox: %w", err)
		}
	}
	// Retire even intents that never reached runtime creation. Otherwise a
	// delayed retry could resurrect an older build after a successful stop.
	entries, err := os.ReadDir(filepath.Dir(e.operationPath(current.WorkspaceID, current.BuildID)))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || entry.Name() == current.BuildID+".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(e.directory, "operations", current.WorkspaceID, entry.Name()))
		if err != nil {
			return err
		}
		old, err := DecodeState(data, current.WorkspaceID)
		if err != nil {
			return err
		}
		// A crash before network ownership is committed can leave only
		// staging files, which inventory cannot attribute to a workspace.
		// The durable intent still authorizes cleanup of this exact ID.
		if old.RuntimeID != "" && old.Phase != "stopped" && old.Phase != "superseded" {
			if err := e.runtime.Delete(ctx, old.RuntimeID); err != nil {
				return xerrors.Errorf("clean prior sandbox intent: %w", err)
			}
		}
		old.Phase = "superseded"
		old.AgentToken = ""
		if err := e.save(old); err != nil {
			return err
		}
	}
	return nil
}
