package agentcontext

import "github.com/fsnotify/fsnotify"

// ManagerStarted exposes the unexported started() channel for
// use by external _test packages. Production code does not need
// this signal; the agent calls Run synchronously after wiring
// the Manager. Tests use it to coordinate without polling.
func ManagerStarted(m *Manager) <-chan struct{} { return m.started() }

// HashedResources exposes driftResources to tests.
func HashedResources(resources []Resource) []Resource { return driftResources(resources) }

// WatchDirs exposes the directory set Watcher.Sync would watch.
func WatchDirs(roots []ScanRoot, opts ResolveOptions) map[string]struct{} {
	out := make(map[string]struct{})
	for dir := range (*Watcher)(nil).collectDirs(roots, opts) {
		out[dir] = struct{}{}
	}
	return out
}

// WatchEventRelevant reports whether a watcher synced to roots and
// opts would re-resolve on ev.
func WatchEventRelevant(roots []ScanRoot, opts ResolveOptions, ev fsnotify.Event) bool {
	w := &Watcher{watched: (*Watcher)(nil).collectDirs(roots, opts)}
	return w.eventRelevant(ev)
}
