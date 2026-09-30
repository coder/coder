package chatd

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/agentbox"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// turnBoxTracker holds the agent box of the turn a runner is executing.
// Turns are keyed by the prompt row ID (stopNudgeKey), which increases
// monotonically, so a canceled task of a finished turn cannot recreate
// the turn's box after the runner closed it.
type turnBoxTracker struct {
	mu        sync.Mutex
	key       int64
	closedKey int64
	box       *agentbox.Box
}

// acquire returns the box for key, creating one when the tracker holds
// none or holds a box for another key. created reports that this call
// made the box. key must be positive and newer than the last closed turn.
func (t *turnBoxTracker) acquire(engine *agentbox.Engine, key int64) (box *agentbox.Box, created bool, err error) {
	if key <= 0 {
		return nil, false, xerrors.New("agent box requires a user prompt")
	}
	var stale *agentbox.Box
	t.mu.Lock()
	switch {
	case key <= t.closedKey:
		t.mu.Unlock()
		return nil, false, xerrors.New("agent box for this turn is closed")
	case t.box != nil && t.key == key:
		box = t.box
	default:
		stale = t.box
		box, err = engine.NewBox()
		if err != nil {
			t.mu.Unlock()
			return nil, false, err
		}
		t.box, t.key = box, key
		created = true
	}
	t.mu.Unlock()
	if stale != nil {
		_ = stale.Close()
	}
	return box, created, nil
}

// take detaches the current box for the caller to close and marks its
// turn closed. It returns nil when no box is held.
func (t *turnBoxTracker) take() *agentbox.Box {
	t.mu.Lock()
	defer t.mu.Unlock()
	box := t.box
	if box != nil {
		t.closedKey = max(t.closedKey, t.key)
	}
	t.box = nil
	return box
}

// closeTurnBox releases the turn's box in the background. Directory
// removal must not delay the runner's state loop.
func (r *runner) closeTurnBox() {
	if box := r.boxes.take(); box != nil {
		go func() { _ = box.Close() }()
	}
}

// closeTurnBoxSync releases the turn's box before returning. Server.Close
// closes the worker before the engine, so the runner exit path must not
// leave a close running against a closing engine.
func (r *runner) closeTurnBoxSync() {
	if box := r.boxes.take(); box != nil {
		_ = box.Close()
	}
}

// lastTurnBoxID returns the box_id of the most recent box tool result in
// the current turn, or "" when the turn has none.
func lastTurnBoxID(messages []database.ChatMessage) string {
	last := ""
	for _, msg := range messages[currentTurnStartIndex(messages):] {
		if msg.Deleted || msg.Compressed || msg.Role != database.ChatMessageRoleTool {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		if err != nil {
			continue
		}
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeToolResult || part.IsError ||
				!slices.Contains(chattool.BoxToolNames, part.ToolName) {
				continue
			}
			var result struct {
				BoxID string `json:"box_id"`
			}
			if json.Unmarshal(part.Result, &result) == nil && result.BoxID != "" {
				last = result.BoxID
			}
		}
	}
	return last
}

// newTurnBoxGetter returns the GetBox callback for one step. The box is
// acquired on first use; reset is reported by the first call only, and
// only when the turn's history shows a different box was used before.
func newTurnBoxGetter(
	engine *agentbox.Engine,
	tracker *turnBoxTracker,
	messages []database.ChatMessage,
) chattool.GetBoxFunc {
	key := stopNudgeKey(messages)
	var mu sync.Mutex
	resetReported := false
	return func(context.Context) (*agentbox.Box, bool, error) {
		box, _, err := tracker.acquire(engine, key)
		if err != nil {
			return nil, false, err
		}
		mu.Lock()
		defer mu.Unlock()
		if resetReported {
			return box, false, nil
		}
		resetReported = true
		previous := lastTurnBoxID(messages)
		return box, previous != "" && previous != box.ID(), nil
	}
}
