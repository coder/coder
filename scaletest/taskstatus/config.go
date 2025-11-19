package taskstatus

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type Config struct {
	// TemplateID is the template ID to use for creating the external workspace.
	TemplateID uuid.UUID `json:"template_id"`

	// WorkspaceName is the name for the external workspace to create.
	WorkspaceName string `json:"workspace_name"`

	// AppSlug is the slug of the app designated as the AI Agent.
	AppSlug string `json:"app_slug"`

	// When the runner has connected to the watch-ws endpoint, it will call Done once on this wait group. Used to
	// coordinate multiple runners from the higher layer.
	ConnectedWaitGroup *sync.WaitGroup `json:"-"`

	// We read on this channel before starting to report task statuses. Used to coordinate multiple runners from the
	// higher layer.
	StartReporting chan struct{} `json:"-"`

	// Time between reporting task statuses.
	ReportStatusPeriod time.Duration `json:"report_status_period"`

	// Total time to report task statuses, starting from when we successfully read from the StartReporting channel.
	ReportStatusDuration time.Duration `json:"report_status_duration"`

	Metrics           *Metrics `json:"-"`
	MetricLabelValues []string `json:"metric_label_values"`
}

func (c *Config) Validate() error {
	if c.TemplateID == uuid.Nil {
		return xerrors.Errorf("validate template_id: must not be nil")
	}

	if c.WorkspaceName == "" {
		return xerrors.Errorf("validate workspace_name: must not be empty")
	}

	if c.AppSlug == "" {
		return xerrors.Errorf("validate app_slug: must not be empty")
	}

	if c.ConnectedWaitGroup == nil {
		return xerrors.Errorf("validate connected_wait_group: must not be nil")
	}

	if c.StartReporting == nil {
		return xerrors.Errorf("validate start_reporting: must not be nil")
	}

	if c.ReportStatusPeriod <= 0 {
		return xerrors.Errorf("validate report_status_period: must be greater than zero")
	}

	if c.ReportStatusDuration <= 0 {
		return xerrors.Errorf("validate report_status_duration: must be greater than zero")
	}

	if c.Metrics == nil {
		return xerrors.Errorf("validate metrics: must not be nil")
	}

	return nil
}
