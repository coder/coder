package usage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"

	"golang.org/x/xerrors"
)

const (
	// AgentRuntimeBackfillSiteConfigKey stores the durable checkpoint for the
	// retained-history Agent Time catch-up.
	AgentRuntimeBackfillSiteConfigKey = "agent_runtime_all_history_catchup_v1"
	// AgentRuntimeBackfillVersion is the current checkpoint schema version.
	AgentRuntimeBackfillVersion = 1
	// AgentRuntimeBackfillPendingJSON is the initial durable checkpoint value.
	AgentRuntimeBackfillPendingJSON = `{"version":1,"status":"pending"}`
)

// AgentRuntimeBackfillStatus is the lifecycle state of the retained-history
// Agent Time catch-up.
type AgentRuntimeBackfillStatus string

const (
	AgentRuntimeBackfillStatusPending  AgentRuntimeBackfillStatus = "pending"
	AgentRuntimeBackfillStatusRunning  AgentRuntimeBackfillStatus = "running"
	AgentRuntimeBackfillStatusComplete AgentRuntimeBackfillStatus = "complete"
)

// AgentRuntimeBackfillState is the durable checkpoint for the retained-history
// Agent Time catch-up. Bucket bounds are UTC and end-exclusive.
type AgentRuntimeBackfillState struct {
	Version      int                        `json:"version"`
	Status       AgentRuntimeBackfillStatus `json:"status"`
	NextBucket   *time.Time                 `json:"next_bucket,omitempty"`
	EndExclusive *time.Time                 `json:"end_exclusive,omitempty"`
	CompletedAt  *time.Time                 `json:"completed_at,omitempty"`
}

// ParseAgentRuntimeBackfillState strictly parses and validates a checkpoint.
func ParseAgentRuntimeBackfillState(value string) (AgentRuntimeBackfillState, error) {
	var state AgentRuntimeBackfillState
	dec := json.NewDecoder(bytes.NewBufferString(value))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&state); err != nil {
		return AgentRuntimeBackfillState{}, xerrors.Errorf("decode agent runtime backfill state: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = xerrors.New("unexpected trailing JSON value")
		}
		return AgentRuntimeBackfillState{}, xerrors.Errorf("decode agent runtime backfill state: %w", err)
	}
	if err := state.validate(); err != nil {
		return AgentRuntimeBackfillState{}, err
	}
	return state, nil
}

// MarshalAgentRuntimeBackfillState validates and serializes a checkpoint.
func MarshalAgentRuntimeBackfillState(state AgentRuntimeBackfillState) (string, error) {
	if err := state.validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return "", xerrors.Errorf("marshal agent runtime backfill state: %w", err)
	}
	return string(data), nil
}

func (s AgentRuntimeBackfillState) validate() error {
	if s.Version != AgentRuntimeBackfillVersion {
		return xerrors.Errorf("unsupported agent runtime backfill state version %d", s.Version)
	}

	switch s.Status {
	case AgentRuntimeBackfillStatusPending:
		if s.NextBucket != nil || s.EndExclusive != nil || s.CompletedAt != nil {
			return xerrors.New("pending agent runtime backfill state must not contain progress")
		}
		return nil
	case AgentRuntimeBackfillStatusRunning, AgentRuntimeBackfillStatusComplete:
		if s.NextBucket == nil || s.EndExclusive == nil {
			return xerrors.Errorf("%s agent runtime backfill state requires bucket bounds", s.Status)
		}
	default:
		return xerrors.Errorf("invalid agent runtime backfill status %q", s.Status)
	}

	if !isUTCHour(*s.NextBucket) || !isUTCHour(*s.EndExclusive) {
		return xerrors.New("agent runtime backfill bucket bounds must be UTC hours")
	}
	if s.NextBucket.After(*s.EndExclusive) {
		return xerrors.New("agent runtime backfill next bucket is after end")
	}

	if s.Status == AgentRuntimeBackfillStatusRunning {
		if s.CompletedAt != nil {
			return xerrors.New("running agent runtime backfill state must not have completion time")
		}
		return nil
	}
	if s.CompletedAt == nil || s.CompletedAt.IsZero() {
		return xerrors.New("complete agent runtime backfill state requires completion time")
	}
	if !s.NextBucket.Equal(*s.EndExclusive) {
		return xerrors.New("complete agent runtime backfill state must be at its end")
	}
	return nil
}

func isUTCHour(t time.Time) bool {
	_, offset := t.Zone()
	return offset == 0 && t.Equal(t.UTC().Truncate(time.Hour))
}
