// Package rewrite2026augustlog is scaffolding, and exists to be deleted.
//
// It marks, inside the code rather than beside it, the points where a
// lifecycle event happens and is not yet journaled. Each call stands where a
// journal entry and its posting will go, and records what was in scope when the
// event occurred.
//
// # How this ends
//
// A call is removed when the journal write that belongs there is written. When
// none remain, the package is deleted. Deleting it turns every remaining call
// into a compile error, so the rewrite cannot be left half finished without the
// build saying so. That is the point of routing these through one package
// rather than writing log lines in place: an empty log file cannot tell a hook
// that never fired from a hook that no longer exists.
//
// # Audience
//
// Whoever writes the journal code, reading the file afterwards to learn which
// hooks fire, in what order, and carrying what. The identifiers recorded are
// those of the code being replaced, which are not the entity identities this
// work uses; translating between them is the work these calls stand in for.
//
// # Not production code
//
// Every function is best effort and returns nothing. A failure to record is
// swallowed, because scaffolding observing a code path must never be able to
// break it.
package rewrite2026augustlog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// F carries whatever was in scope at a call site. It is a map rather than a
// typed struct per event because these call sites are temporary and their
// contents will change as each is understood.
type F map[string]any

// PathEnv names the environment variable that sets where the log is written.
// Unset, the log goes to rewrite-2026-august.jsonl under .coderv2 if that
// directory already exists, and nowhere otherwise. Nothing creates the
// directory: a test run that has no .coderv2 writes no file and needs no
// cleanup.
const PathEnv = "CODER_REWRITE_2026_AUGUST_LOG"

const defaultPath = ".coderv2/rewrite-2026-august.jsonl"

var (
	mu       sync.Mutex
	file     *os.File
	resolved bool
)

// AIAgentCreated marks an AI agent coming into existence.
func AIAgentCreated(ctx context.Context, f F) { record(ctx, "ai_agent.created", f) }

// AIAgentRevoked marks an AI agent identity being revoked. Two call sites reach
// this event by different routes, so a count of these is not a count of AI
// agents unless the two are known to be exclusive.
func AIAgentRevoked(ctx context.Context, f F) { record(ctx, "ai_agent.revoked", f) }

// SandboxCreated marks a sandbox being created.
func SandboxCreated(ctx context.Context, f F) { record(ctx, "sandbox.created", f) }

// SandboxDeleted marks a sandbox being deleted.
func SandboxDeleted(ctx context.Context, f F) { record(ctx, "sandbox.deleted", f) }

// SessionOpened marks an AI agent becoming able to act outside its sandbox,
// which is when the egress proxy has its rules and the confined process has
// started.
func SessionOpened(ctx context.Context, f F) { record(ctx, "session.opened", f) }

// SessionClosed marks a session reported as ended. The report is best effort in
// the code that sends it, so an absence here is not evidence a session is still
// open.
func SessionClosed(ctx context.Context, f F) { record(ctx, "session.closed", f) }

func record(_ context.Context, event string, f F) {
	mu.Lock()
	defer mu.Unlock()

	w := writer()
	if w == nil {
		return
	}

	entry := map[string]any{
		"at":     time.Now().UTC().Format(time.RFC3339Nano),
		"event":  event,
		"caller": caller(),
		"fields": f,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	// Errors are deliberately dropped. See the package comment.
	_, _ = w.Write(append(line, '\n'))
}

// writer opens the log on first use and gives up permanently if it cannot. It
// is called with mu held.
func writer() *os.File {
	if resolved {
		return file
	}
	resolved = true

	path := os.Getenv(PathEnv)
	if path == "" {
		if info, err := os.Stat(filepath.Dir(defaultPath)); err != nil || !info.IsDir() {
			return nil
		}
		path = defaultPath
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil
	}
	file = f
	return file
}

// caller names the call site, which is what makes a line in this file findable
// in the code it is standing in for.
func caller() string {
	// 0 is caller, 1 is record, 2 is the exported function, 3 is the call site.
	_, path, line, ok := runtime.Caller(3)
	if !ok {
		return "unknown"
	}
	return filepath.Base(filepath.Dir(path)) + "/" + filepath.Base(path) + ":" + itoa(line)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
