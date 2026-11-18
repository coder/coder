package agentcontext

import "github.com/fsnotify/fsnotify"

// ManagerStarted exposes the unexported started() channel for
// use by external _test packages. Production code does not need
// this signal; the agent calls Run synchronously after wiring
// the Manager. Tests use it to coordinate without polling.
func ManagerStarted(m *Manager) <-chan struct{} { return m.started() }

const (
	MaxPluginVersionRunes     = maxPluginVersionRunes
	MaxPluginDescriptionRunes = maxPluginDescriptionRunes
)

// WatchDirs exposes the directory set Watcher.Sync would watch.
func WatchDirs(roots []ScanRoot, opts ResolveOptions) map[string]struct{} {
	out := make(map[string]struct{})
	for dir := range (*Watcher)(nil).collectDirs(roots, opts) {
		out[dir] = struct{}{}
	}
	return out
}

// WouldResolveOn exposes eventRelevant using the roles the last Sync
// published.
func (w *Watcher) WouldResolveOn(ev fsnotify.Event) bool {
	return w.eventRelevant(ev)
}

// WatchList returns the directories fsnotify currently watches.
func (w *Watcher) WatchList() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.watcher == nil {
		return nil
	}
	return w.watcher.WatchList()
}
