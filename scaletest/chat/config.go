package chat

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// Config describes a single chat runner within a scaletest invocation.
type Config struct {
	// RunID identifies a single CLI invocation across all chat runners.
	RunID string `json:"run_id"`

	// OrganizationID is the organization that owns the target workspace.
	OrganizationID uuid.UUID `json:"organization_id,omitempty"`

	// WorkspaceID is the pre-existing workspace to use for this chat run.
	WorkspaceID uuid.UUID `json:"workspace_id,omitempty"`

	// Prompt is the text content sent on every turn.
	Prompt string `json:"prompt"`

	// ModelConfigID optionally selects a specific model config.
	// When nil, the server uses its deployment default.
	ModelConfigID *uuid.UUID `json:"model_config_id,omitempty"`

	// Turns is the total number of user→assistant exchanges per chat.
	// Must be at least 1.
	Turns int `json:"turns"`

	// TurnStartDelay is the shared delay between every runner completing
	// its initial turn and the release of the follow-up turn storm. Set
	// to 0 to fire all turns back-to-back without an inter-phase pause.
	TurnStartDelay time.Duration `json:"turn_start_delay"`

	// ReadyWaitGroup is used to coordinate thundering-herd fanout from the CLI
	// layer. Each runner calls Done() once it is ready to start.
	ReadyWaitGroup *sync.WaitGroup `json:"-"`

	// StartChan blocks runners before creating chats. The CLI layer closes it
	// once all runners have reached the start barrier.
	StartChan chan struct{} `json:"-"`

	// TurnStartReadyWaitGroup coordinates the gap between the initial turn
	// finishing and the follow-up turn storm. Each runner signals once its
	// first turn has reached a terminal status, or it knows it will never
	// reach that point. Signaling happens via a sync.Once, so safety-net
	// defers and the natural-path signal are both safe to fire.
	TurnStartReadyWaitGroup *sync.WaitGroup `json:"-"`

	// StartTurnsChan blocks the turn storm until the CLI layer releases it.
	StartTurnsChan chan struct{} `json:"-"`

	Metrics *Metrics `json:"-"`

	// MetricLabelValues are the Prometheus label values shared across this run.
	MetricLabelValues []string
}

func (c Config) Validate() error {
	if c.RunID == "" {
		return xerrors.Errorf("validate run_id: must not be empty")
	}
	if c.OrganizationID == uuid.Nil {
		return xerrors.Errorf("validate organization_id: must not be empty")
	}
	if c.WorkspaceID == uuid.Nil {
		return xerrors.Errorf("validate workspace_id: must not be empty")
	}
	if c.Prompt == "" {
		return xerrors.Errorf("validate prompt: must not be empty")
	}
	if c.Turns < 1 {
		return xerrors.Errorf("validate turns: must be at least 1")
	}
	if c.TurnStartDelay < 0 {
		return xerrors.Errorf("validate turn_start_delay: must not be negative")
	}
	if c.ReadyWaitGroup == nil {
		return xerrors.Errorf("validate ready_wait_group: must not be nil")
	}
	if c.StartChan == nil {
		return xerrors.Errorf("validate start_chan: must not be nil")
	}
	if c.TurnStartDelay > 0 {
		if c.TurnStartReadyWaitGroup == nil {
			return xerrors.Errorf("validate turn_start_ready_wait_group: must not be nil when turn start delay is enabled")
		}
		if c.StartTurnsChan == nil {
			return xerrors.Errorf("validate start_turns_chan: must not be nil when turn start delay is enabled")
		}
	}
	if c.Metrics == nil {
		return xerrors.Errorf("validate metrics: must not be nil")
	}

	expectedLabels := len(MetricLabelNames())
	if len(c.MetricLabelValues) != expectedLabels {
		return xerrors.Errorf("validate metric_label_values: got %d values, want %d", len(c.MetricLabelValues), expectedLabels)
	}

	return nil
}
