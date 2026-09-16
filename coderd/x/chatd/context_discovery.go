package chatd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/encoding/protojson"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	// maxInstructionAncestorDepth bounds the walk from a touched directory
	// up toward the working directory.
	maxInstructionAncestorDepth = 16
	// instructionDiscoveryTimeout bounds the agent round trip so a slow
	// workspace cannot stall the step.
	instructionDiscoveryTimeout = 3 * time.Second
	// instructionProbeTTL is how long a directory known to hold no
	// instruction file is not asked about again.
	instructionProbeTTL = 10 * time.Minute
	// maxInstructionProbeEntries bounds the negative cache across all
	// agents; the cache is reset when it fills.
	maxInstructionProbeEntries = 4096
)

// instructionFileNames mirrors the agent resolver's recognized names so a
// tool that writes one of them re-probes its directory.
var instructionFileNames = []string{"AGENTS.md", "CLAUDE.md", ".cursorrules"}

// instructionDiscoverer pins the instruction files found in the directories
// a step's executed tools touched, so the next model call sees them.
type instructionDiscoverer func(ctx context.Context, calls []fantasy.ToolCallContent, results []fantasy.Content)

// instructionProbeCache remembers directories the agent reported as holding
// no instruction file, per agent, so a step that keeps touching the same
// tree does not re-ask on every tool call.
type instructionProbeCache struct {
	mu      sync.Mutex
	entries map[instructionProbeKey]time.Time
}

type instructionProbeKey struct {
	agentID uuid.UUID
	dir     string
}

func (c *instructionProbeCache) negative(now time.Time, agentID uuid.UUID, dir string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	expiry, ok := c.entries[instructionProbeKey{agentID: agentID, dir: dir}]
	return ok && now.Before(expiry)
}

func (c *instructionProbeCache) markNegative(now time.Time, agentID uuid.UUID, dirs []string) {
	if len(dirs) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[instructionProbeKey]time.Time)
	}
	if len(c.entries)+len(dirs) > maxInstructionProbeEntries {
		for key, expiry := range c.entries {
			if !now.Before(expiry) {
				delete(c.entries, key)
			}
		}
		if len(c.entries)+len(dirs) > maxInstructionProbeEntries {
			c.entries = make(map[instructionProbeKey]time.Time)
		}
	}
	for _, dir := range dirs {
		c.entries[instructionProbeKey{agentID: agentID, dir: dir}] = now.Add(instructionProbeTTL)
	}
}

func (c *instructionProbeCache) forget(agentID uuid.UUID, dirs []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, dir := range dirs {
		delete(c.entries, instructionProbeKey{agentID: agentID, dir: dir})
	}
}

// touchedPaths returns the absolute paths the executed tool calls addressed:
// the read_file, write_file, and edit_files paths and the explicit execute
// workdir. Relative paths are dropped because the agent rejects them; the
// default execute directory is already a scan root.
func touchedPaths(calls []fantasy.ToolCallContent, results []fantasy.Content) (files, dirs []string) {
	executed := make(map[string]struct{}, len(results))
	for _, block := range results {
		if result, ok := fantasy.AsContentType[fantasy.ToolResultContent](block); ok {
			executed[result.ToolCallID] = struct{}{}
		} else if result, ok := fantasy.AsContentType[*fantasy.ToolResultContent](block); ok && result != nil {
			executed[result.ToolCallID] = struct{}{}
		}
	}
	addFile := func(p string) {
		if path.IsAbs(p) {
			files = append(files, path.Clean(p))
		}
	}
	for _, call := range calls {
		if _, ok := executed[call.ToolCallID]; !ok {
			continue
		}
		switch call.ToolName {
		case chattool.ReadFileToolName, chattool.WriteFileToolName:
			var args struct {
				Path string `json:"path"`
			}
			if json.Unmarshal([]byte(call.Input), &args) == nil {
				addFile(args.Path)
			}
		case chattool.EditFilesToolName:
			var args struct {
				Files []struct {
					Path string `json:"path"`
				} `json:"files"`
			}
			if json.Unmarshal([]byte(call.Input), &args) == nil {
				for _, file := range args.Files {
					addFile(file.Path)
				}
			}
		case chattool.ExecuteToolName:
			var args struct {
				WorkDir *string `json:"workdir"`
			}
			if json.Unmarshal([]byte(call.Input), &args) == nil && args.WorkDir != nil && path.IsAbs(*args.WorkDir) {
				dirs = append(dirs, path.Clean(*args.WorkDir))
			}
		}
	}
	return files, dirs
}

// candidateInstructionDirs lists the directories whose instruction files
// would apply to the touched paths: each touched directory and its
// ancestors, stopping below the working directory (its own files and those
// of its child projects arrive through the snapshot) and below the root.
// Directories above the working directory are not candidates either; only
// nested files refine the pinned ones. The result is deduplicated, ordered
// shallowest first, and capped at the agent request limit.
func candidateInstructionDirs(files, dirs []string, workingDir string) []string {
	workingDir = path.Clean(workingDir)
	seen := make(map[string]struct{})
	var out []string
	visit := func(dir string) {
		for depth := 0; depth < maxInstructionAncestorDepth; depth++ {
			if dir == "/" || dir == "." || dir == workingDir || (workingDir != "/" && strings.HasPrefix(workingDir+"/", dir+"/")) {
				return
			}
			if _, ok := seen[dir]; ok {
				return
			}
			seen[dir] = struct{}{}
			out = append(out, dir)
			dir = path.Dir(dir)
		}
	}
	for _, file := range files {
		visit(path.Dir(file))
	}
	for _, dir := range dirs {
		visit(dir)
	}
	slices.SortFunc(out, func(a, b string) int {
		if da, db := strings.Count(a, "/"), strings.Count(b, "/"); da != db {
			return da - db
		}
		return strings.Compare(a, b)
	})
	if len(out) > workspacesdk.MaxContextInstructionDirectories {
		out = out[:workspacesdk.MaxContextInstructionDirectories]
	}
	return out
}

func agentWorkingDirectory(agent database.WorkspaceAgent) string {
	if agent.ExpandedDirectory != "" {
		return agent.ExpandedDirectory
	}
	return agent.Directory
}

// newInstructionDiscoverer binds discovery to the turn's workspace context
// so the agent connection the tools already dialed is reused.
func (p *Server) newInstructionDiscoverer(workspaceCtx *turnWorkspaceContext, chat database.Chat, agent database.WorkspaceAgent) instructionDiscoverer {
	if agent.ID == uuid.Nil || agentWorkingDirectory(agent) == "" {
		return nil
	}
	return func(ctx context.Context, calls []fantasy.ToolCallContent, results []fantasy.Content) {
		p.discoverInstructionContext(ctx, workspaceCtx, chat, agent, calls, results)
	}
}

// discoverInstructionContext asks the agent for instruction files in the
// directories the step's tools touched and pins any it finds as discovered
// rows for this chat only. Every failure is logged and swallowed: discovery
// must never fail the step.
func (p *Server) discoverInstructionContext(
	ctx context.Context,
	workspaceCtx *turnWorkspaceContext,
	chat database.Chat,
	agent database.WorkspaceAgent,
	calls []fantasy.ToolCallContent,
	results []fantasy.Content,
) {
	files, dirs := touchedPaths(calls, results)
	candidates := candidateInstructionDirs(files, dirs, agentWorkingDirectory(agent))
	if len(candidates) == 0 {
		return
	}
	logger := p.logger.With(slog.F("chat_id", chat.ID), slog.F("agent_id", agent.ID))

	// Writing an instruction file changes what its directory holds, so a
	// cached negative for that directory is stale.
	var written []string
	for _, file := range files {
		if slices.Contains(instructionFileNames, path.Base(file)) {
			written = append(written, path.Dir(file))
		}
	}
	p.instructionProbes.forget(agent.ID, written)

	//nolint:gocritic // Chatd pins discovered rows onto a chat it does not own.
	dbCtx := dbauthz.AsChatd(ctx)
	rows, err := p.db.ListChatContextResourcesByChatID(dbCtx, chat.ID)
	if err != nil {
		logger.Debug(ctx, "list pinned context for instruction discovery", slog.Error(err))
		return
	}
	pinnedDirs := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.BodyKind == database.WorkspaceAgentContextBodyKindInstructionFile {
			pinnedDirs[path.Dir(row.Source)] = struct{}{}
		}
	}
	now := p.clock.Now()
	probe := make([]string, 0, len(candidates))
	for _, dir := range candidates {
		if _, ok := pinnedDirs[dir]; ok {
			continue
		}
		if p.instructionProbes.negative(now, agent.ID, dir) {
			continue
		}
		probe = append(probe, dir)
	}
	if len(probe) == 0 {
		return
	}

	conn, err := workspaceCtx.getWorkspaceConn(ctx)
	if err != nil {
		logger.Debug(ctx, "connect to agent for instruction discovery", slog.Error(err))
		return
	}
	resolveCtx, cancel := context.WithTimeout(ctx, instructionDiscoveryTimeout)
	defer cancel()
	resp, err := conn.ResolveContextInstructions(resolveCtx, workspacesdk.ResolveContextInstructionsRequest{Directories: probe})
	if err != nil {
		// Older agents answer 404; either way the step proceeds without
		// nested files.
		logger.Debug(ctx, "resolve instruction files through agent", slog.Error(err))
		return
	}

	found := make(map[string]struct{}, len(resp.Files))
	pinned := 0
	for _, file := range resp.Files {
		found[file.Directory] = struct{}{}
		ok, err := p.pinDiscoveredInstructionFile(dbCtx, chat.ID, file)
		if err != nil {
			logger.Warn(ctx, "pin discovered instruction file", slog.F("source", file.Source), slog.Error(err))
			continue
		}
		if ok {
			pinned++
		}
	}
	negatives := make([]string, 0, len(probe))
	for _, dir := range probe {
		if _, ok := found[dir]; !ok {
			negatives = append(negatives, dir)
		}
	}
	p.instructionProbes.markNegative(now, agent.ID, negatives)
	if pinned == 0 {
		return
	}
	logger.Debug(ctx, "pinned discovered instruction files", slog.F("count", pinned))
	updated, err := p.db.GetChatByID(dbCtx, chat.ID)
	if err != nil {
		logger.Warn(ctx, "read chat after instruction discovery", slog.Error(err))
		return
	}
	p.publishChatPubsubEvents([]database.Chat{updated}, codersdk.ChatWatchEventKindContextDirty)
}

// pinDiscoveredInstructionFile stores one resolved file as a discovered row.
// Only readable and oversize results are worth pinning: both tell the model
// the file exists, and an oversize row keeps the directory from being
// re-probed. Other statuses are transient read failures.
func (p *Server) pinDiscoveredInstructionFile(ctx context.Context, chatID uuid.UUID, file workspacesdk.ContextInstructionFile) (bool, error) {
	status := database.WorkspaceAgentContextResourceStatus(file.Status)
	if status != database.WorkspaceAgentContextResourceStatusOk && status != database.WorkspaceAgentContextResourceStatusOversize {
		return false, nil
	}
	if file.SizeBytes > math.MaxInt64 {
		return false, xerrors.Errorf("size %d exceeds int64 range", file.SizeBytes)
	}
	contentHash, err := hex.DecodeString(file.ContentHash)
	if err != nil {
		return false, xerrors.Errorf("decode content hash: %w", err)
	}
	body, err := protojson.Marshal(&agentproto.InstructionFileBody{Content: []byte(file.Content)})
	if err != nil {
		return false, xerrors.Errorf("encode instruction body: %w", err)
	}
	if err := p.db.UpsertChatContextDiscoveredResource(ctx, database.UpsertChatContextDiscoveredResourceParams{
		ChatID:      chatID,
		Source:      file.Source,
		BodyKind:    database.WorkspaceAgentContextBodyKindInstructionFile,
		Body:        body,
		ContentHash: contentHash,
		SizeBytes:   int64(file.SizeBytes),
		Status:      status,
		Error:       file.Error,
	}); err != nil {
		return false, xerrors.Errorf("upsert discovered resource: %w", err)
	}
	return true, nil
}

// rediscoverInstructionContext re-reads the discovered instruction files a
// refresh just dropped, so Refresh context re-pins nested files at their
// current contents instead of forgetting them until the next tool touch.
// Best-effort like discovery itself.
func (p *Server) rediscoverInstructionContext(ctx context.Context, chat database.Chat, dirs []string) {
	if len(dirs) == 0 || !chat.AgentID.Valid || p.agentConnFn == nil {
		return
	}
	if len(dirs) > workspacesdk.MaxContextInstructionDirectories {
		dirs = dirs[:workspacesdk.MaxContextInstructionDirectories]
	}
	logger := p.logger.With(slog.F("chat_id", chat.ID), slog.F("agent_id", chat.AgentID.UUID))
	resolveCtx, cancel := context.WithTimeout(ctx, instructionDiscoveryTimeout)
	defer cancel()
	conn, release, err := p.agentConnFn(resolveCtx, chat.AgentID.UUID)
	if err != nil {
		logger.Debug(ctx, "connect to agent for instruction rediscovery", slog.Error(err))
		return
	}
	defer release()
	resp, err := conn.ResolveContextInstructions(resolveCtx, workspacesdk.ResolveContextInstructionsRequest{Directories: dirs})
	if err != nil {
		logger.Debug(ctx, "re-resolve discovered instruction files", slog.Error(err))
		return
	}
	for _, file := range resp.Files {
		if _, err := p.pinDiscoveredInstructionFile(ctx, chat.ID, file); err != nil {
			logger.Warn(ctx, "re-pin discovered instruction file", slog.F("source", file.Source), slog.Error(err))
		}
	}
}

// discoveredInstructionDirs lists the directories of a chat's discovered
// rows, deduplicated, so a refresh can re-resolve them.
func discoveredInstructionDirs(rows []database.ChatContextResource) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, row := range rows {
		if !row.Discovered {
			continue
		}
		dir := path.Dir(row.Source)
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		out = append(out, dir)
	}
	return out
}
