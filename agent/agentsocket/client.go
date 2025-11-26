package agentsocket

// Option represents a configuration option for NewClient.
type Option func(*options)

type options struct {
	path string
}

// WithPath sets the socket path. If not provided or empty, the client will
// auto-discover the default socket path.
func WithPath(path string) Option {
	return func(opts *options) {
		opts.path = path
	}
}

// SyncStatusResponse contains the status information for a unit.
type SyncStatusResponse struct {
	Status       string           `json:"status"`
	IsReady      bool             `json:"is_ready"`
	Dependencies []DependencyInfo `json:"dependencies"`
}

// DependencyInfo contains information about a unit dependency.
type DependencyInfo struct {
	DependsOn      string `json:"depends_on"`
	RequiredStatus string `json:"required_status"`
	CurrentStatus  string `json:"current_status"`
	IsSatisfied    bool   `json:"is_satisfied"`
}
