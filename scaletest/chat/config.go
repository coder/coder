package chat

import (
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type Config struct {
	// WorkspaceID is the pre-existing workspace to create chats against.
	WorkspaceID uuid.UUID `json:"workspace_id"`

	// Prompt is the text content for the initial chat message.
	Prompt string `json:"prompt"`

	// ModelConfigID optionally selects a specific model config.
	// When nil the server uses its deployment default.
	ModelConfigID *uuid.UUID `json:"model_config_id,omitempty"`

	// ReadyWaitGroup is used to coordinate thundering-herd fanout from the CLI
	// layer. Each runner calls Done() once it is ready to start.
	ReadyWaitGroup *sync.WaitGroup `json:"-"`

	// StartChan blocks runners before creating chats. The CLI layer closes it
	// once all runners are ready.
	StartChan chan struct{} `json:"-"`

	Metrics           *Metrics `json:"-"`
	MetricLabelValues []string `json:"metric_label_values"`
}

func (c *Config) Validate() error {
	if c.WorkspaceID == uuid.Nil {
		return xerrors.Errorf("validate workspace_id: must not be nil")
	}

	if c.Prompt == "" {
		return xerrors.Errorf("validate prompt: must not be empty")
	}

	if c.ReadyWaitGroup == nil {
		return xerrors.Errorf("validate ready_wait_group: must not be nil")
	}

	if c.StartChan == nil {
		return xerrors.Errorf("validate start_chan: must not be nil")
	}

	if c.Metrics == nil {
		return xerrors.Errorf("validate metrics: must not be nil")
	}

	return nil
}
