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
	// maxInstructionProbesPerStep bounds the directories one step asks the
	// agent about once pinned and cached-negative ones are filtered, so a
	// step that touches many trees costs at most a few round trips.
	maxInstructionProbesPerStep = 4 * workspacesdk.MaxContextInstructionDirectories
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
	expiry, ok := c.entries[instructionProbeKey{agentID: agentID, dir: pathKey(dir)}]
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
		c.entries[instructionProbeKey{agentID: agentID, dir: pathKey(dir)}] = now.Add(instructionProbeTTL)
	}
}

func (c *instructionProbeCache) forget(agentID uuid.UUID, dirs []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, dir := range dirs {
		delete(c.entries, instructionProbeKey{agentID: agentID, dir: pathKey(dir)})
	}
}

// agentPath normalizes a path a tool addressed or the agent reported to
// forward slashes, so the POSIX path package can reason about Linux and
// Windows agent paths alike. Windows accepts forward slashes on the way back.
func agentPath(p string) string {
	return path.Clean(strings.ReplaceAll(p, "\\", "/"))
}

// pathKey is the comparison form of a normalized path. Windows file systems
// are case-insensitive by default, so drive-rooted paths compare folded;
// POSIX paths compare as they are.
func pathKey(p string) string {
	if len(p) >= 2 && p[1] == ':' {
		return strings.ToLower(p)
	}
	return p
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
// nested files refine the pinned ones. The result is deduplicated and
// ordered shallowest first.
func candidateInstructionDirs(files, dirs []string, workingDir string) []string {
	workingKey := pathKey(agentPath(workingDir))
	seen := make(map[string]struct{})
	var out []string
	visit := func(dir string) {
		for depth := 0; depth < maxInstructionAncestorDepth; depth++ {
			key := pathKey(dir)
			if isRootAgentPath(dir) || key == workingKey || (workingKey != "/" && strings.HasPrefix(workingKey+"/", key+"/")) {
				return
			}
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
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
	return out
}

// staleInstructionDirs lists, by pathKey, the directories whose instruction
// files may have changed during the step: those an instruction file was
// written to, and explicit execute workdirs, where a command may have
// created or removed one.
func staleInstructionDirs(files, dirs []string) map[string]struct{} {
	stale := make(map[string]struct{}, len(dirs))
	for _, file := range files {
		if slices.Contains(instructionFileNames, path.Base(file)) {
			stale[pathKey(path.Dir(file))] = struct{}{}
		}
	}
	for _, dir := range dirs {
		stale[pathKey(dir)] = struct{}{}
	}
	return stale
}

// selectInstructionProbes picks the candidates worth asking the agent
// about. A stale directory is always probed; otherwise a directory that
// already contributed a pinned instruction file, or that is a fresh
// negative, is skipped. pinnedDirs and stale are keyed by pathKey.
func selectInstructionProbes(candidates []string, pinnedDirs, stale map[string]struct{}, negative func(dir string) bool) []string {
	probe := make([]string, 0, len(candidates))
	for _, dir := range candidates {
		key := pathKey(dir)
		if _, touched := stale[key]; touched {
			probe = append(probe, dir)
			continue
		}
		if _, ok := pinnedDirs[key]; ok || negative(dir) {
			continue
		}
		probe = append(probe, dir)
	}
	return probe
}

// removedDiscoveredSources lists the discovered rows in stale, probed
// directories whose source the probe did not return: files the step removed
// or renamed. stale and returned are keyed by pathKey.
func removedDiscoveredSources(rows []database.ChatContextResource, stale map[string]struct{}, probed []string, returned map[string]struct{}) []string {
	probedKeys := make(map[string]struct{}, len(probed))
	for _, dir := range probed {
		probedKeys[pathKey(dir)] = struct{}{}
	}
	var out []string
	for _, row := range rows {
		if !row.Discovered {
			continue
		}
		source := agentPath(row.Source)
		dirKey := pathKey(path.Dir(source))
		if _, ok := stale[dirKey]; !ok {
			continue
		}
		if _, ok := probedKeys[dirKey]; !ok {
			continue
		}
		if _, ok := returned[pathKey(source)]; ok {
			continue
		}
		out = append(out, row.Source)
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
			pinnedDirs[pathKey(path.Dir(agentPath(row.Source)))] = struct{}{}
		}
	}
	now := p.clock.Now()
	probe := selectInstructionProbes(candidates, pinnedDirs, stale, func(dir string) bool {
		return p.instructionProbes.negative(now, agent.ID, dir)
	})
	if len(probe) == 0 {
		return
	}
	if len(probe) > maxInstructionProbesPerStep {
		probe = probe[:maxInstructionProbesPerStep]
	}

	conn, err := workspaceCtx.getWorkspaceConn(ctx)
	if err != nil {
		logger.Debug(ctx, "connect to agent for instruction discovery", slog.Error(err))
		return
	}
	resolved, probed := resolveInstructionDirs(ctx, logger, conn, probe)
	if len(probed) == 0 {
		return
	}

	returned := make(map[string]struct{}, len(resolved))
	foundDirs := make(map[string]struct{}, len(resolved))
	pinned := 0
	for _, file := range resolved {
		returned[pathKey(agentPath(file.Source))] = struct{}{}
		foundDirs[pathKey(agentPath(file.Directory))] = struct{}{}
		ok, err := p.pinDiscoveredInstructionFile(dbCtx, chat.ID, file)
		if err != nil {
			logger.Warn(ctx, "pin discovered instruction file", slog.F("source", file.Source), slog.Error(err))
			continue
		}
		if ok {
			pinned++
		}
	}
	removed := 0
	for _, source := range removedDiscoveredSources(rows, stale, probed, returned) {
		if err := p.db.DeleteChatContextDiscoveredResource(dbCtx, database.DeleteChatContextDiscoveredResourceParams{ChatID: chat.ID, Source: source}); err != nil {
			logger.Warn(ctx, "delete removed instruction file", slog.F("source", source), slog.Error(err))
			continue
		}
		removed++
	}
	negatives := make([]string, 0, len(probed))
	for _, dir := range probed {
		if _, ok := foundDirs[pathKey(dir)]; !ok {
			negatives = append(negatives, dir)
		}
	}
	p.instructionProbes.markNegative(now, agent.ID, negatives)
	if pinned == 0 && removed == 0 {
		return
	}
	logger.Debug(ctx, "reconciled discovered instruction files", slog.F("pinned", pinned), slog.F("removed", removed))
	updated, err := p.db.GetChatByID(dbCtx, chat.ID)
	if err != nil {
		logger.Warn(ctx, "read chat after instruction discovery", slog.Error(err))
		return
	}
	p.publishChatPubsubEvents([]database.Chat{updated}, codersdk.ChatWatchEventKindContextDirty)
}

// pinDiscoveredInstructionFile stores one resolved file as a discovered row.
// Readable results are pinned with their body; oversize and excluded ones
// (past the per-file or the per-response cap) without it, so the model and
// the inventory still learn the file exists and the directory is not
// re-probed. Other statuses are transient read failures.
func (p *Server) pinDiscoveredInstructionFile(ctx context.Context, chatID uuid.UUID, file workspacesdk.ContextInstructionFile) (bool, error) {
	status := database.WorkspaceAgentContextResourceStatus(file.Status)
	switch status {
	case database.WorkspaceAgentContextResourceStatusOk,
		database.WorkspaceAgentContextResourceStatusOversize,
		database.WorkspaceAgentContextResourceStatusExcluded:
	default:
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
	resolved, _ := resolveInstructionDirs(ctx, logger, conn, dirs)
	for _, file := range resolved {
		if _, err := p.pinDiscoveredInstructionFile(ctx, chat.ID, file); err != nil {
			logger.Warn(ctx, "re-pin discovered instruction file", slog.F("source", file.Source), slog.Error(err))
		}
	}
}

// resolveInstructionDirs asks the agent about dirs in request-sized batches,
// each under the discovery timeout. It returns every file the agent
// reported and the directories it was actually asked about: a failed batch
// ends the round without discarding earlier answers, and an older agent
// that lacks the endpoint fails the first batch, so the step proceeds
// without nested files.
func resolveInstructionDirs(ctx context.Context, logger slog.Logger, conn workspacesdk.AgentConn, dirs []string) (files []workspacesdk.ContextInstructionFile, probed []string) {
	for batch := range slices.Chunk(dirs, workspacesdk.MaxContextInstructionDirectories) {
		resolveCtx, cancel := context.WithTimeout(ctx, instructionDiscoveryTimeout)
		resp, err := conn.ResolveContextInstructions(resolveCtx, workspacesdk.ResolveContextInstructionsRequest{Directories: batch})
		cancel()
		if err != nil {
			logger.Debug(ctx, "resolve instruction files through agent", slog.Error(err))
			return files, probed
		}
		files = append(files, resp.Files...)
		probed = append(probed, batch...)
	}
	return files, probed
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
