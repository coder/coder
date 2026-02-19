package autostart

import (
	"context"

	"github.com/coder/coder/v2/codersdk"
)

// WorkspaceDispatcher manages the distribution of workspace build updates from
// a single source channel to multiple per-workspace channels.
type WorkspaceDispatcher struct {
	// Channels maps workspace names to their respective update channels.
	Channels map[string]chan codersdk.WorkspaceBuildUpdate
}

// NewWorkspaceDispatcher creates a new dispatcher for the given workspace names.
// Each workspace gets a buffered channel that can hold all expected updates during
// the autostart test lifecycle:
// - initial build (~3 updates: pending, running, succeeded)
// - stop build (~3 updates: pending, running, succeeded)
// - autostart build (~3 updates: pending, running, succeeded)
// Total: ~9 updates. We use a buffer of 16 to provide headroom for timing variations.
func NewWorkspaceDispatcher(workspaceNames []string) *WorkspaceDispatcher {
	channels := make(map[string]chan codersdk.WorkspaceBuildUpdate, len(workspaceNames))
	for _, name := range workspaceNames {
		channels[name] = make(chan codersdk.WorkspaceBuildUpdate, 16)
	}
	return &WorkspaceDispatcher{
		Channels: channels,
	}
}

// Start begins listening for workspace build updates and dispatching them to
// the appropriate workspace channels. It runs in a goroutine and returns
// immediately. When the source channel closes, all workspace channels are
// closed automatically.
func (d *WorkspaceDispatcher) Start(ctx context.Context, source <-chan codersdk.WorkspaceBuildUpdate) {
	go func() {
		for update := range source {
			if ch, ok := d.Channels[update.WorkspaceName]; ok {
				select {
				case ch <- update:
				case <-ctx.Done():
					return
				}
			}
		}
		// Close all workspace channels when the source closes.
		for _, ch := range d.Channels {
			close(ch)
		}
	}()
}
