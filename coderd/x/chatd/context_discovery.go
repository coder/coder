package chatd

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	// maxDiscoveredInstructionBytes caps the readable discovered content a
	// chat holds across every step. The agent caps one response at the same
	// size, but a step can make several requests and a chat many steps, and
	// every pinned body is rendered into every later prompt; files past the
	// cap are pinned as excluded, without content.
	maxDiscoveredInstructionBytes = 1 << 20
	// pendingProbeTTL is how long a directory a command ran in is re-probed
	// on every later touch of its tree, so a file the command creates after
	// its result returned (a background process, a timed-out run) is still
	// picked up.
	pendingProbeTTL = 10 * time.Minute
)

// instructionFileNames mirrors the agent resolver's recognized names so a
// tool that writes one of them re-probes its directory.
var instructionFileNames = []string{"AGENTS.md", "CLAUDE.md", ".cursorrules"}

// instructionDiscoverer pins the instruction files found in the directories
// a step's executed tools touched, so the next model call sees them.
type instructionDiscoverer func(ctx context.Context, calls []fantasy.ToolCallContent, results []fantasy.Content)

// instructionProbeCache remembers, per agent, directories the agent
// reported as holding no instruction file, so a step that keeps touching
// the same tree does not re-ask on every tool call, and directories a
// command ran in, which later touches re-probe for a while because the
// command may still be writing.
type instructionProbeCache struct {
	mu      sync.Mutex
	entries map[instructionProbeKey]time.Time
	pending map[instructionProbeKey]time.Time
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

// markPending records directories a command ran in. The set is bounded
// like the negative cache; when it fills, expired entries go first and then
// the whole set, which only costs the next touch a probe it may not need.
func (c *instructionProbeCache) markPending(now time.Time, agentID uuid.UUID, dirs []string) {
	if len(dirs) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending == nil {
		c.pending = make(map[instructionProbeKey]time.Time)
	}
	if len(c.pending)+len(dirs) > maxInstructionProbeEntries {
		for key, expiry := range c.pending {
			if !now.Before(expiry) {
				delete(c.pending, key)
			}
		}
		if len(c.pending)+len(dirs) > maxInstructionProbeEntries {
			c.pending = make(map[instructionProbeKey]time.Time)
		}
	}
	for _, dir := range dirs {
		c.pending[instructionProbeKey{agentID: agentID, dir: pathKey(dir)}] = now.Add(pendingProbeTTL)
	}
}

// isPending reports whether a command ran in dir recently enough that a
// touch of its tree should read it again.
func (c *instructionProbeCache) isPending(now time.Time, agentID uuid.UUID, dir string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	expiry, ok := c.pending[instructionProbeKey{agentID: agentID, dir: pathKey(dir)}]
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

// isDrivePath reports whether p is a Windows drive-rooted path such as C:/.
func isDrivePath(p string) bool {
	return len(p) >= 2 && p[1] == ':'
}

// pathKey is the comparison form of a normalized path. Windows file systems
// are case-insensitive by default, so drive-rooted paths compare folded;
// POSIX paths compare as they are.
func pathKey(p string) string {
	if isDrivePath(p) {
		return strings.ToLower(p)
	}
	return p
}

// isInstructionFilePath reports whether file has a recognized instruction
// file name, spelled exactly on POSIX and in any case on a Windows drive.
func isInstructionFilePath(file string) bool {
	base := path.Base(file)
	for _, name := range instructionFileNames {
		if base == name || (isDrivePath(file) && strings.EqualFold(base, name)) {
			return true
		}
	}
	return false
}

// isAbsAgentPath accepts a POSIX root or a Windows drive root such as C:/.
func isAbsAgentPath(p string) bool {
	return path.IsAbs(p) || (len(p) >= 3 && p[1] == ':' && p[2] == '/')
}

// isUNCPath reports whether raw is a Windows network path such as
// \\server\share. Discovery leaves those alone: agentPath would fold the
// leading separators into a POSIX root, which the Windows agent rejects as
// relative and fails the whole probe batch with.
func isUNCPath(raw string) bool {
	return len(raw) >= 2 && (raw[0] == '\\' || raw[0] == '/') && (raw[1] == '\\' || raw[1] == '/')
}

// instructionRowDir is the directory an instruction row's file sits in,
// which for a discovered row is the directory that was probed: the agent
// reports a symlinked file under the link's own path.
func instructionRowDir(row database.ChatContextResource) string {
	return path.Dir(agentPath(row.Source))
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
		if isUNCPath(p) {
			return
		}
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
			if json.Unmarshal([]byte(call.Input), &args) == nil && args.WorkDir != nil && !isUNCPath(*args.WorkDir) {
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
// ordered shallowest first, so the per-step request cap keeps the
// directories whose files govern the most.
func candidateInstructionDirs(files, dirs []string, workingDir string) []string {
	workingKey := pathKey(agentPath(workingDir))
	seen := make(map[string]struct{})
	var out []string
	visit := func(dir string) {
		for {
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
		if isInstructionFilePath(file) {
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
		dirKey := pathKey(instructionRowDir(row))
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

	now := p.clock.Now()
	stale := staleInstructionDirs(files, dirs)
	// A directory a command ran in recently stays stale for later touches:
	// the command may still be writing when its result returns.
	p.instructionProbes.markPending(now, agent.ID, dirs)
	for _, dir := range candidates {
		if p.instructionProbes.isPending(now, agent.ID, dir) {
			stale[pathKey(dir)] = struct{}{}
		}
	}
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
			pinnedDirs[pathKey(instructionRowDir(row))] = struct{}{}
		}
	}
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
	pinned, removed, err := reconcileDiscoveredInstructionFiles(dbCtx, p.db, chat.ID, rows, resolved, stale, probed)
	if err != nil {
		logger.Warn(ctx, "reconcile discovered instruction files", slog.Error(err))
		return
	}

	foundDirs := make(map[string]struct{}, len(resolved))
	for _, file := range resolved {
		foundDirs[pathKey(agentPath(file.Directory))] = struct{}{}
	}
	negatives := make([]string, 0, len(probed))
	for _, dir := range probed {
		key := pathKey(dir)
		if _, ok := foundDirs[key]; ok {
			continue
		}
		// A stale directory is not remembered as empty: the command that
		// made it stale may still be running in the background and create
		// the file after this probe, and a later touch must ask again.
		if _, ok := stale[key]; ok {
			continue
		}
		negatives = append(negatives, dir)
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

// reconcileDiscoveredInstructionFiles pins the files the agent resolved and
// removes the discovered rows in stale, probed directories it no longer
// returned. rows is the chat's inventory before the probe: a file the chat
// already pins under another spelling of a Windows path reuses that
// spelling, since the agent echoes the request's casing and the row key is
// case-sensitive. Readable content past the chat's discovered budget is
// pinned as excluded; resolved files arrive shallowest directory first, so
// the budget goes to the files that govern the most. It reports how many
// rows were pinned and removed, and stops at the first store error so a
// transaction caller sees it.
func reconcileDiscoveredInstructionFiles(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
	rows []database.ChatContextResource,
	resolved []workspacesdk.ContextInstructionFile,
	stale map[string]struct{},
	probed []string,
) (pinned, removed int, err error) {
	spellings := make(map[string]string, len(rows))
	heldBytes := make(map[string]int64, len(rows))
	var used int64
	for _, row := range rows {
		if row.BodyKind != database.WorkspaceAgentContextBodyKindInstructionFile {
			continue
		}
		key := pathKey(agentPath(row.Source))
		spellings[key] = row.Source
		if row.Discovered && row.Status == database.WorkspaceAgentContextResourceStatusOk {
			heldBytes[key] = row.SizeBytes
			used += row.SizeBytes
		}
	}
	returned := make(map[string]struct{}, len(resolved))
	for _, file := range resolved {
		key := pathKey(agentPath(file.Source))
		returned[key] = struct{}{}
		if source, ok := spellings[key]; ok {
			file.Source = source
		}
		if file.Status == string(database.WorkspaceAgentContextResourceStatusOk) {
			// A re-read of a pinned file replaces its bytes rather than
			// adding to them.
			used -= heldBytes[key]
			delete(heldBytes, key)
			size := int64(math.MaxInt64)
			if file.SizeBytes <= math.MaxInt64 {
				size = int64(file.SizeBytes)
			}
			if size > maxDiscoveredInstructionBytes-used {
				file.Status = string(database.WorkspaceAgentContextResourceStatusExcluded)
				file.Error = fmt.Sprintf("discovered instruction content cap of %d bytes reached", maxDiscoveredInstructionBytes)
				file.Content = ""
			} else {
				used += size
				heldBytes[key] = size
			}
		}
		ok, err := pinDiscoveredInstructionFile(ctx, store, chatID, file)
		if err != nil {
			return pinned, removed, xerrors.Errorf("pin discovered instruction file %q: %w", file.Source, err)
		}
		if ok {
			pinned++
		}
	}
	for _, source := range removedDiscoveredSources(rows, stale, probed, returned) {
		if err := store.DeleteChatContextDiscoveredResource(ctx, database.DeleteChatContextDiscoveredResourceParams{ChatID: chatID, Source: source}); err != nil {
			return pinned, removed, xerrors.Errorf("delete removed instruction file %q: %w", source, err)
		}
		removed++
	}
	return pinned, removed, nil
}

// pinDiscoveredInstructionFile stores one resolved file as a discovered row.
// A readable result is pinned with its body; any other status (oversize,
// excluded, unreadable, invalid) without one, so the inventory and the
// prompt's omitted-file note still learn the file exists, the directory is
// not re-probed, and a file that stopped being readable replaces the body
// pinned from an earlier read rather than keeping it.
func pinDiscoveredInstructionFile(ctx context.Context, store database.Store, chatID uuid.UUID, file workspacesdk.ContextInstructionFile) (bool, error) {
	status := database.WorkspaceAgentContextResourceStatus(file.Status)
	if !status.Valid() {
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
	if err := store.UpsertChatContextDiscoveredResource(ctx, database.UpsertChatContextDiscoveredResourceParams{
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

// rediscoverInstructionContext re-reads the discovered rows a refresh kept,
// so Refresh context brings nested files to their current contents and
// drops the ones that vanished. Best-effort like discovery itself: a
// directory a failed batch left unread keeps its rows as they were.
//
// The probe runs after the refresh transaction, so a step may pin newer
// bytes for one of these files before its result is applied. The result is
// therefore applied in a repeatable-read transaction and only to rows still
// as the refresh captured them: a row a step rewrote earlier fails the hash
// check, and one it rewrites during the transaction fails the transaction
// with a serialization error, whose retry sees the new hash.
func (p *Server) rediscoverInstructionContext(ctx context.Context, chat database.Chat, captured []database.ChatContextResource) {
	dirs := discoveredInstructionDirs(captured)
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
	resolved, probed := resolveInstructionDirs(ctx, logger, conn, dirs)
	if len(probed) == 0 {
		return
	}
	stale := make(map[string]struct{}, len(probed))
	for _, dir := range probed {
		stale[pathKey(dir)] = struct{}{}
	}
	err = database.ReadModifyUpdate(p.db, func(tx database.Store) error {
		current, err := tx.ListChatContextResourcesByChatID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("list chat context resources for rediscovery: %w", err)
		}
		rows, files := unchangedDiscoveredRows(captured, current, resolved)
		_, _, err = reconcileDiscoveredInstructionFiles(ctx, tx, chat.ID, rows, files, stale, probed)
		return err
	})
	if err != nil {
		logger.Warn(ctx, "apply instruction rediscovery", slog.Error(err))
	}
}

// unchangedDiscoveredRows narrows a rediscovery to what a concurrent step has
// not touched since the refresh captured the discovered rows. It returns the
// current inventory without the discovered rows whose content changed or
// that vanished, so only untouched rows can be rewritten or removed, and the
// resolved files minus those whose source a step pinned meanwhile, whether
// by rewriting a captured row or by discovering a file the refresh did not
// know.
func unchangedDiscoveredRows(
	captured, current []database.ChatContextResource,
	resolved []workspacesdk.ContextInstructionFile,
) ([]database.ChatContextResource, []workspacesdk.ContextInstructionFile) {
	capturedHash := make(map[string][]byte, len(captured))
	for _, row := range captured {
		if row.Discovered {
			capturedHash[pathKey(agentPath(row.Source))] = row.ContentHash
		}
	}
	rows := make([]database.ChatContextResource, 0, len(current))
	unchanged := make(map[string]struct{}, len(current))
	touched := make(map[string]struct{}, len(current))
	for _, row := range current {
		key := pathKey(agentPath(row.Source))
		if !row.Discovered {
			rows = append(rows, row)
			continue
		}
		if hash, ok := capturedHash[key]; ok && bytes.Equal(hash, row.ContentHash) {
			rows = append(rows, row)
			unchanged[key] = struct{}{}
			continue
		}
		touched[key] = struct{}{}
	}
	files := make([]workspacesdk.ContextInstructionFile, 0, len(resolved))
	for _, file := range resolved {
		key := pathKey(agentPath(file.Source))
		if _, ok := touched[key]; ok {
			continue
		}
		if _, ok := capturedHash[key]; ok {
			if _, ok := unchanged[key]; !ok {
				// Captured but gone from the inventory: a step removed it.
				continue
			}
		}
		files = append(files, file)
	}
	return rows, files
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

// discoveredInstructionRows filters a chat's inventory to its discovered
// rows.
func discoveredInstructionRows(rows []database.ChatContextResource) []database.ChatContextResource {
	var out []database.ChatContextResource
	for _, row := range rows {
		if row.Discovered {
			out = append(out, row)
		}
	}
	return out
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
		dir := instructionRowDir(row)
		if _, ok := seen[pathKey(dir)]; ok {
			continue
		}
		seen[pathKey(dir)] = struct{}{}
		out = append(out, dir)
	}
	return out
}
