package prebuilds

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

type Config struct {
	// OrganizationID is the ID of the organization to create the prebuilds in.
	OrganizationID uuid.UUID `json:"organization_id"`
	// NumPresets is the number of presets the template should have.
	NumPresets int `json:"num_presets"`
	// NumPresetPrebuilds is the number of prebuilds per preset.
	// Total prebuilds = NumPresets * NumPresetPrebuilds
	NumPresetPrebuilds int `json:"num_preset_prebuilds"`

	// TemplateVersionJobTimeout is how long to wait for template version
	// provisioning jobs to complete.
	TemplateVersionJobTimeout time.Duration `json:"template_version_job_timeout"`

	// PrebuildWorkspaceTimeout is how long to wait for all prebuild
	// workspaces to be created and completed.
	PrebuildWorkspaceTimeout time.Duration `json:"prebuild_workspace_timeout"`

	Metrics *Metrics `json:"-"`

	// SetupBarrier is used to ensure all templates have been created
	// before unpausing prebuilds.
	SetupBarrier *sync.WaitGroup `json:"-"`

	// CreationBarrier is used to ensure all prebuild creation has completed
	// before pausing prebuilds for deletion.
	CreationBarrier *sync.WaitGroup `json:"-"`

	// DeletionBarrier is used to ensure all templates have been updated
	// with 0 prebuilds before resuming prebuilds.
	DeletionBarrier *sync.WaitGroup `json:"-"`

	Clock quartz.Clock `json:"-"`
}

func (c Config) Validate() error {
	if c.TemplateVersionJobTimeout <= 0 {
		return xerrors.New("template_version_job_timeout must be greater than 0")
	}

	if c.PrebuildWorkspaceTimeout <= 0 {
		return xerrors.New("prebuild_workspace_timeout must be greater than 0")
	}

	if c.SetupBarrier == nil {
		return xerrors.New("setup barrier must be set")
	}

	if c.CreationBarrier == nil {
		return xerrors.New("creation barrier must be set")
	}

	if c.DeletionBarrier == nil {
		return xerrors.New("deletion barrier must be set")
	}

	if c.Metrics == nil {
		return xerrors.New("metrics must be set")
	}

	if c.Clock == nil {
		return xerrors.New("clock must be set")
	}

	return nil
}
