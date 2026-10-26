package agentnative

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// JobSpec is the one initial literal task configured for this workspace.
type JobSpec struct {
	Harness string `json:"harness"`
	Cwd     string `json:"cwd"`
	Prompt  string `json:"prompt"`
}

// Job records an initial task's sole attempt. It intentionally stores only a
// fingerprint, not prompt text. Unknown outcomes are never retried.
type Job struct {
	ID          string `json:"jobId"`
	InputHash   string `json:"inputHash"`
	SessionID   string `json:"sessionId,omitempty"`
	Status      string `json:"status"`
	RequestID   string `json:"requestId"`
	AttemptedAt int64  `json:"attemptedAt"`
}

func normalizeJob(spec JobSpec) (JobSpec, error) {
	if spec.Harness == "" {
		spec.Harness = "claude"
	}
	if spec.Harness != "claude" && spec.Harness != "codex" && spec.Harness != "opencode" {
		return spec, xerrors.New("unknown native job harness")
	}
	if !filepath.IsAbs(spec.Cwd) || len(spec.Cwd) > 4096 || strings.ContainsRune(spec.Cwd, 0) || strings.TrimSpace(spec.Prompt) == "" || len(spec.Prompt) > maxPromptBytes {
		return spec, xerrors.New("native job requires an absolute cwd and nonempty prompt up to 100000 bytes")
	}
	return spec, nil
}

func validJob(j Job, s snapshot) bool {
	if !validUUID(j.ID) || !validUUID(j.RequestID) || len(j.InputHash) != 64 || j.AttemptedAt < 0 {
		return false
	}
	if _, err := hex.DecodeString(j.InputHash); err != nil {
		return false
	}
	if j.SessionID != "" && s.Sessions[j.SessionID] == nil {
		return false
	}
	if j.SessionID == "" && j.Status != "starting" && j.Status != "unknown" {
		return false
	}
	switch j.Status {
	case "starting", "created", "prompting", "completed", "failed", "canceled", "blocked", "unknown":
		return true
	default:
		return false
	}
}

// RunJob durably claims the initial task before even creating its session. Once
// claimed, matching invocations return its status without creating or submitting
// anything, including after a crash between creation and recording the identity.
func (m *Manager) RunJob(ctx context.Context, spec JobSpec) (Job, error) {
	return runManagerOperation(ctx, m, func(ctx context.Context) (Job, error) {
		spec, err := normalizeJob(spec)
		if err != nil {
			return Job{}, err
		}
		encoded, _ := json.Marshal(spec)
		hash := textHash(string(encoded))
		m.mu.Lock()
		if err := m.availableLocked(); err != nil {
			m.mu.Unlock()
			return Job{}, err
		}
		if old := m.state.Job; old != nil {
			job := *old
			m.mu.Unlock()
			if job.InputHash != hash {
				return job, xerrors.New("initial native job is already claimed by different input; nothing was replayed")
			}
			return job, nil
		}
		job := Job{ID: uuid.NewString(), InputHash: hash, Status: "starting", RequestID: uuid.NewString(), AttemptedAt: m.clock.Now().UnixMilli()}
		next := m.state.clone()
		next.Job = &job
		if err := m.commitLocked(next); err != nil {
			m.mu.Unlock()
			return Job{}, err
		}
		m.mu.Unlock()
		s, err := m.createSession(ctx, spec.Harness, spec.Cwd)
		if s.ID != "" {
			m.mu.Lock()
			next = m.state.clone()
			if next.Sessions[s.ID] != nil {
				next.Job.SessionID, next.Job.Status = s.ID, "created"
				err = errors.Join(err, m.commitLocked(next))
			}
			m.mu.Unlock()
		}
		if err != nil {
			return m.failJob("unknown", err)
		}
		if s.PendingInput || s.Fault != "" {
			return m.failJob("blocked", xerrors.New("initial native task requires human input; nothing was submitted"))
		}
		if err := m.changeMode(ctx, s.ID, "automation"); err != nil {
			return m.failJob("unknown", err)
		}
		m.mu.Lock()
		next = m.state.clone()
		next.Job.Status = "prompting"
		err = m.commitLocked(next)
		m.mu.Unlock()
		if err != nil {
			return m.failJob("unknown", err)
		}
		if err := m.submitPrompt(ctx, s.ID, Prompt{ID: job.RequestID, Text: spec.Prompt}); err != nil {
			return m.failJob("unknown", err)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		return *m.state.Job, nil
	})
}

func (m *Manager) failJob(status string, cause error) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.state.clone()
	if next.Job.Status != "completed" && next.Job.Status != "failed" && next.Job.Status != "canceled" {
		next.Job.Status = status
	}
	return *next.Job, errors.Join(cause, m.commitLocked(next))
}

func updateJob(s *snapshot, sessionID string) {
	j := s.Job
	if j == nil || j.SessionID != sessionID {
		return
	}
	r := s.Sessions[sessionID]
	request, ok := r.Requests[j.RequestID]
	if !ok {
		return
	}
	switch request.Status {
	case "attempted", "running":
		j.Status = "prompting"
	case "completed":
		j.Status = "completed"
	case "rejected", "failed":
		j.Status = "failed"
	case "canceled":
		j.Status = "canceled"
	case "unknown":
		j.Status = "unknown"
	}
}
