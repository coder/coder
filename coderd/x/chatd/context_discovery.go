package chatd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"maps"
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

// agentPath normalizes a path a tool addressed or the agent reported to
// forward slashes, so the POSIX path package can reason about Linux and
// Windows agent paths alike. Windows accepts forward slashes on the way back.
func agentPath(p string) string {
	return path.Clean(strings.ReplaceAll(p, "\\", "/"))
}

// isAbsAgentPath accepts a POSIX root or a Windows drive root such as C:/.
func isAbsAgentPath(p string) bool {
	return path.IsAbs(p) || (len(p) >= 3 && p[1] == ':' && p[2] == '/')
}

// isRootAgentPath reports whether dir has no parent worth probing.
func isRootAgentPath(dir string) bool {
	return dir == "/" || dir == "." || (len(dir) == 2 && dir[1] == ':')
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
		if p = agentPath(p); isAbsAgentPath(p) {
			files = append(files, p)
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
			if json.Unmarshal([]byte(call.Input), &args) == nil && args.WorkDir != nil {
				if dir := agentPath(*args.WorkDir); isAbsAgentPath(dir) {
					dirs = append(dirs, dir)
				}
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
	workingDir = agentPath(workingDir)
	seen := make(map[string]struct{})
	var out []string
	visit := func(dir string) {
		for depth := 0; depth < maxInstructionAncestorDepth; depth++ {
			if isRootAgentPath(dir) || dir == workingDir || (workingDir != "/" && strings.HasPrefix(workingDir+"/", dir+"/")) {
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

// staleInstructionDirs lists the directories whose instruction files may
// have changed during the step: those an instruction file was written to,
// and explicit execute workdirs, where a command may have created one.
func staleInstructionDirs(files, dirs []string) map[string]struct{} {
	stale := make(map[string]struct{}, len(dirs))
	for _, file := range files {
		if slices.Contains(instructionFileNames, path.Base(file)) {
			stale[path.Dir(file)] = struct{}{}
		}
	}
	for _, dir := range dirs {
		stale[dir] = struct{}{}
	}
	return stale
}

// selectInstructionProbes picks the candidates worth asking the agent
// about. A stale directory is always probed; otherwise a directory that
// already contributed a pinned instruction file, or that is a fresh
// negative, is skipped.
func selectInstructionProbes(candidates []string, pinnedDirs, stale map[string]struct{}, negative func(dir string) bool) []string {
	probe := make([]string, 0, len(candidates))
	for _, dir := range candidates {
		if _, touched := stale[dir]; touched {
			probe = append(probe, dir)
			continue
		}
		if _, ok := pinnedDirs[dir]; ok || negative(dir) {
			continue
		}
		probe = append(probe, dir)
	}
	return probe
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

	stale := staleInstructionDirs(files, dirs)
	p.instructionProbes.forget(agent.ID, slices.Collect(maps.Keys(stale)))

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
			pinnedDirs[path.Dir(agentPath(row.Source))] = struct{}{}
		}
	}
	now := p.clock.Now()
	probe := selectInstructionProbes(candidates, pinnedDirs, stale, func(dir string) bool {
		return p.instructionProbes.negative(now, agent.ID, dir)
	})
	if len(probe) == 0 {
		return
	}

	conn, err := workspaceCtx.getWorkspaceConn(ctx)
	if err != nil {
		logger.Debug(ctx, "connect to agent for instruction discovery", slog.Error(err))
		return
	}
	resp, err := p.resolveInstructionBatch(ctx, conn, probe)
	if err != nil {
		// Older agents answer 404; either way the step proceeds without
		// nested files.
		logger.Debug(ctx, "resolve instruction files through agent", slog.Error(err))
		return
	}

	found := make(map[string]struct{}, len(resp.Files))
	pinned := 0
	for _, file := range resp.Files {
		found[agentPath(file.Directory)] = struct{}{}
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
	logger := p.logger.With(slog.F("chat_id", chat.ID), slog.F("agent_id", chat.AgentID.UUID))
	conn, release, err := p.agentConnFn(ctx, chat.AgentID.UUID)
	if err != nil {
		logger.Debug(ctx, "connect to agent for instruction rediscovery", slog.Error(err))
		return
	}
	defer release()
	// A long chat can hold discovered files from more directories than one
	// request may name, so the list is sent in request-sized batches.
	for batch := range slices.Chunk(dirs, workspacesdk.MaxContextInstructionDirectories) {
		resp, err := p.resolveInstructionBatch(ctx, conn, batch)
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
}

// resolveInstructionBatch asks the agent for one request's worth of
// directories under the discovery timeout.
func (*Server) resolveInstructionBatch(ctx context.Context, conn workspacesdk.AgentConn, dirs []string) (workspacesdk.ResolveContextInstructionsResponse, error) {
	resolveCtx, cancel := context.WithTimeout(ctx, instructionDiscoveryTimeout)
	defer cancel()
	return conn.ResolveContextInstructions(resolveCtx, workspacesdk.ResolveContextInstructionsRequest{Directories: dirs})
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
		dir := path.Dir(agentPath(row.Source))
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		out = append(out, dir)
	}
	return out
}
