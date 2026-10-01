package chatd

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"sync"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/agentbox"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

// turnBoxTracker holds the agent box of the turn a runner is executing.
// Turns are keyed by the prompt row ID (stopNudgeKey), which increases
// monotonically, so a canceled task of an older turn can neither recreate
// that turn's box after it was closed nor replace a newer turn's box.
type turnBoxTracker struct {
	mu sync.Mutex
	// key is the newest turn that held a box.
	key       int64
	closedKey int64
	box       *agentbox.Box
}

// acquire returns the box for key, creating one when the tracker holds
// none. A box held for an older key is closed first so it does not count
// against the engine's live box limit. created reports that this call
// made the box. key must be positive, at least the newest key seen, and
// newer than the last closed turn.
func (t *turnBoxTracker) acquire(
	logger slog.Logger,
	engine *agentbox.Engine,
	key int64,
) (box *agentbox.Box, created bool, err error) {
	if key <= 0 {
		return nil, false, xerrors.New("agent box requires a user prompt")
	}
	for {
		t.mu.Lock()
		switch {
		case t.box != nil && t.key == key:
			box = t.box
			t.mu.Unlock()
			return box, false, nil
		case key <= t.closedKey || key < t.key:
			t.mu.Unlock()
			return nil, false, xerrors.New("agent box for this turn is closed")
		case t.box != nil:
			stale := t.detachLocked()
			t.mu.Unlock()
			closeBox(logger, stale)
			continue
		}
		box, err = engine.NewBox()
		if err != nil {
			t.mu.Unlock()
			return nil, false, err
		}
		t.box, t.key = box, key
		t.mu.Unlock()
		return box, true, nil
	}
}

// retireOlder detaches a box held for a turn older than key so the caller
// can close it. It returns nil when there is none.
func (t *turnBoxTracker) retireOlder(key int64) *agentbox.Box {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.box == nil || t.key >= key {
		return nil
	}
	return t.detachLocked()
}

// take detaches the current box for the caller to close and marks its
// turn closed. It returns nil when no box is held.
func (t *turnBoxTracker) take() *agentbox.Box {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.detachLocked()
}

func (t *turnBoxTracker) detachLocked() *agentbox.Box {
	box := t.box
	if box != nil {
		t.closedKey = max(t.closedKey, t.key)
	}
	t.box = nil
	return box
}

func closeBox(logger slog.Logger, box *agentbox.Box) {
	if err := box.Close(); err != nil {
		logger.Warn(context.Background(), "failed to close agent box", slog.F("box_id", box.ID()), slog.Error(err))
	}
}

// closeTurnBox releases the turn's box in the background. Directory
// removal must not delay the runner's state loop.
func (r *runner) closeTurnBox() {
	if box := r.boxes.take(); box != nil {
		box.CloseAsync()
	}
}

// closeTurnBoxSync releases the turn's box before returning, so no close
// is left running after the runner exits.
func (r *runner) closeTurnBoxSync() {
	if box := r.boxes.take(); box != nil {
		closeBox(r.opts.Logger, box)
	}
}

// agentBoxToolNamesForTurn lists the box tools usable in a turn with the
// given plan mode.
func agentBoxToolNamesForTurn(planMode database.NullChatPlanMode, isRootChat bool) []string {
	return slices.DeleteFunc(chattool.BoxToolNames(), func(name string) bool {
		return !builtinToolAllowedForTurn(name, planMode, isRootChat)
	})
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
				!slices.Contains(chattool.BoxToolNames(), part.ToolName) {
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

// newTurnBoxGetter returns the GetBox callback for one step. A box still
// held for an older turn is closed in the background. The box is acquired
// on first use; reset is reported by the first call only, and only when
// the turn's history shows a different box was used before.
func newTurnBoxGetter(
	logger slog.Logger,
	engine *agentbox.Engine,
	tracker *turnBoxTracker,
	messages []database.ChatMessage,
) chattool.GetBoxFunc {
	key := stopNudgeKey(messages)
	if stale := tracker.retireOlder(key); stale != nil {
		stale.CloseAsync()
	}
	var mu sync.Mutex
	resetReported := false
	return func(ctx context.Context) (*agentbox.Box, bool, error) {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		box, _, err := tracker.acquire(logger, engine, key)
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

// boxMCPServerNamer returns the display name of an MCP tool's server:
// the config slug for org and inline tools, or the workspace server name.
func boxMCPServerNamer(slugByConfigID map[uuid.UUID]string) func(fantasy.AgentTool) string {
	return func(tool fantasy.AgentTool) string {
		if identified, ok := tool.(mcpclient.MCPToolIdentifier); ok {
			if slug, ok := slugByConfigID[identified.MCPServerConfigID()]; ok {
				return slug
			}
		}
		return workspaceMCPServerName(tool)
	}
}

// boxMCPServers counts the MCP tools in tools that the turn allows and
// lists, sorted and deduplicated, the servers behind them. A tool with no
// resolvable server name is counted but not listed.
func boxMCPServers(
	tools []fantasy.AgentTool,
	namer func(fantasy.AgentTool) string,
	allowed func(fantasy.AgentTool) bool,
) (reachable int, servers []string) {
	seen := map[string]struct{}{}
	for _, tool := range tools {
		if _, ok := tool.(mcpclient.RawCaller); !ok || !allowed(tool) {
			continue
		}
		reachable++
		if name := namer(tool); name != "" {
			seen[name] = struct{}{}
		}
	}
	return reachable, slices.Sorted(maps.Keys(seen))
}
