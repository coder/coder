package chatlist

import (
	"golang.org/x/xerrors"
)

// Config describes a single chat list runner.
type Config struct {
	// ListRequests is the number of sequential list requests the runner
	// issues. Must be at least 1.
	ListRequests int `json:"list_requests"`

	// PageSize is the limit sent with each list request. Must be at least 1.
	PageSize int `json:"page_size"`

	Metrics *Metrics `json:"-"`
}

func (c Config) Validate() error {
	if c.ListRequests < 1 {
		return xerrors.Errorf("validate list_requests: must be at least 1")
	}
	if c.PageSize < 1 {
		return xerrors.Errorf("validate page_size: must be at least 1")
	}
	if c.Metrics == nil {
		return xerrors.Errorf("validate metrics: must not be nil")
	}
	return nil
}
