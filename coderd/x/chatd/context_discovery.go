package chatd

import (
	"bytes"
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
	// instructionDiscoveryTimeout bounds the agent round trip so a slow
	// workspace cannot stall the step.
	instructionDiscoveryTimeout = 3 * time.Second
	// instructionProbeTTL is how long a directory the agent reported empty
	// is skipped, unless a tool touches an instruction file or runs a
	// command there.
	instructionProbeTTL = 10 * time.Minute
	// maxInstructionProbeEntries bounds the probe cache across all agents;
	// the cache is reset when it fills.
	maxInstructionProbeEntries = 4096
	// maxInstructionProbesPerStep bounds the directories one step asks the
	// agent about after pinned and cached-negative ones are filtered.
	maxInstructionProbesPerStep = 4 * workspacesdk.MaxContextInstructionDirectories
	// maxDiscoveredInstructionBytes caps the readable discovered content a
	// chat holds across all steps, since every pinned body is rendered into
	// every later prompt; files past the cap are pinned as excluded.
	maxDiscoveredInstructionBytes = 1 << 20
	// maxDiscoveredInstructionFiles caps the discovered rows a chat holds,
	// whatever their status; the byte cap alone leaves bodyless rows unbounded.
	maxDiscoveredInstructionFiles = 256
)

// instructionFileNames mirrors the agent resolver's recognized names so a
// tool that touches one of them re-probes its directory.
var instructionFileNames = []string{"AGENTS.md", "CLAUDE.md", ".cursorrules"}

type instructionDiscoverer func(ctx context.Context, calls []fantasy.ToolCallContent, results []fantasy.Content)

// instructionProbeCache remembers, per agent, the directories the agent
// reported empty, for every chat on that agent.
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

// markNegative records dirs as empty until the TTL passes. When the cache
// would exceed maxInstructionProbeEntries, expired entries go first and then
// the whole cache, which only costs the next touch a probe it may not need.
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

// agentPath normalizes a tool or agent path so the POSIX path package can
// reason about it: a drive path's backslashes become forward slashes, which
// Windows accepts; on POSIX a backslash is an ordinary name character.
func agentPath(p string) string {
	if isDrivePath(p) {
		p = strings.ReplaceAll(p, "\\", "/")
	}
	return path.Clean(p)
}

// isDrivePath reports whether p starts with a Windows drive letter such as C:.
func isDrivePath(p string) bool {
	return len(p) >= 2 && p[1] == ':'
}

// pathKey is the comparison form of a normalized path: drive paths compare
// case-folded, since Windows file systems are case-insensitive by default.
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

// isAbsAgentPath reports whether the agent's file system takes p as
// absolute: a drive root such as C:/ on Windows, a leading slash elsewhere.
// The agent rejects a whole probe batch over one path it deems relative.
func isAbsAgentPath(p, operatingSystem string) bool {
	if operatingSystem == "windows" {
		return len(p) >= 3 && p[1] == ':' && p[2] == '/'
	}
	return path.IsAbs(p)
}

// isUNCPath reports whether raw is a Windows network path such as
// \\server\share, which agentPath would fold into a POSIX root that the
// Windows agent rejects as relative.
func isUNCPath(raw string) bool {
	return len(raw) >= 2 && (raw[0] == '\\' || raw[0] == '/') && (raw[1] == '\\' || raw[1] == '/')
}

// instructionRowDir is the directory an instruction row's file sits in,
// which for a discovered row is the directory that was probed: the agent
// reports a symlinked file under the link's own path.
func instructionRowDir(row database.ChatContextResource) string {
	return path.Dir(agentPath(row.Source))
}

func isRootAgentPath(dir string) bool {
	return dir == "/" || dir == "." || (len(dir) == 2 && dir[1] == ':')
}

// touchedPaths returns the paths the executed tool calls addressed, as the
// tools received them: file tool paths and explicit execute workdirs. The
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
		if p = strings.TrimSpace(p); p != "" {
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
			if json.Unmarshal([]byte(call.Input), &args) == nil && args.WorkDir != nil && *args.WorkDir != "" {
				dirs = append(dirs, *args.WorkDir)
			}
		}
	}
	return files, dirs
}

// agentTouchedPaths normalizes touched paths for the agent's file system
// and drops the ones the agent would reject as relative, including Windows
// network paths, which would fail the whole probe batch.
func agentTouchedPaths(files, dirs []string, operatingSystem string) (outFiles, outDirs []string) {
	keep := func(out []string, p string) []string {
		if operatingSystem == "windows" && isUNCPath(p) {
			return out
		}
		if p = agentPath(p); isAbsAgentPath(p, operatingSystem) {
			out = append(out, p)
		}
		return out
	}
	for _, file := range files {
		outFiles = keep(outFiles, file)
	}
	for _, dir := range dirs {
		outDirs = keep(outDirs, dir)
	}
	return outFiles, outDirs
}

// candidateInstructionDirs lists each touched directory and its ancestors
// below the working directory, whose own files arrive through the snapshot,
// or below the root when the agent reports none, deduplicated and
// shallowest first so the request cap keeps the widest scopes.
func candidateInstructionDirs(files, dirs []string, workingDir string) []string {
	workingKey := ""
	if workingDir != "" {
		workingKey = pathKey(agentPath(workingDir))
	}
	seen := make(map[string]struct{})
	var out []string
	visit := func(dir string) {
		for {
			key := pathKey(dir)
			if isRootAgentPath(dir) || (workingKey != "" && (key == workingKey || (workingKey != "/" && strings.HasPrefix(workingKey+"/", key+"/")))) {
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
	sortShallowestFirst(out)
	return out
}

// sortShallowestFirst orders directories by depth, then name, so caps and
// budgets applied in order favor the files that govern the most paths.
func sortShallowestFirst(dirs []string) {
	slices.SortFunc(dirs, func(a, b string) int {
		if da, db := strings.Count(a, "/"), strings.Count(b, "/"); da != db {
			return da - db
		}
		return strings.Compare(a, b)
	})
}

// invalidatedProbeDirs lists the directories whose cached empty answer
// the step's touches invalidate: those of touched instruction files and
// explicit execute workdirs.
func invalidatedProbeDirs(files, dirs []string) []string {
	out := slices.Clone(dirs)
	for _, file := range files {
		if isInstructionFilePath(file) {
			out = append(out, path.Dir(file))
		}
	}
	return out
}

// pinnedInstructionDirs lists, by pathKey, the directories whose files the
// chat already holds; discovery leaves them to Refresh.
func pinnedInstructionDirs(rows []database.ChatContextResource) map[string]struct{} {
	pinned := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.BodyKind == database.WorkspaceAgentContextBodyKindInstructionFile {
			pinned[pathKey(instructionRowDir(row))] = struct{}{}
		}
	}
	return pinned
}

// selectInstructionProbes keeps the candidates neither pinned nor cached
// negative. pinnedDirs is keyed by pathKey.
func selectInstructionProbes(candidates []string, pinnedDirs map[string]struct{}, negative func(dir string) bool) []string {
	probe := make([]string, 0, len(candidates))
	for _, dir := range candidates {
		if _, ok := pinnedDirs[pathKey(dir)]; ok || negative(dir) {
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

// newInstructionDiscoverer binds discovery to the turn's workspace context.
// The connection is taken before the agent: taking it may switch the turn
// to the workspace's current agent, which is the one the probes then reach.
func (p *Server) newInstructionDiscoverer(workspaceCtx *turnWorkspaceContext, chat database.Chat) instructionDiscoverer {
	return func(ctx context.Context, calls []fantasy.ToolCallContent, results []fantasy.Content) {
		files, dirs := touchedPaths(calls, results)
		if len(files) == 0 && len(dirs) == 0 {
			return
		}
		conn, err := workspaceCtx.getWorkspaceConn(ctx)
		if err != nil {
			p.logger.Debug(ctx, "connect to agent for instruction discovery", slog.F("chat_id", chat.ID), slog.Error(err))
			return
		}
		agent, err := workspaceCtx.getWorkspaceAgent(ctx)
		if err != nil {
			p.logger.Debug(ctx, "read agent for instruction discovery", slog.F("chat_id", chat.ID), slog.Error(err))
			return
		}
		p.discoverInstructionContext(ctx, conn, agent, chat, files, dirs)
	}
}

// discoverInstructionContext asks agent, over conn, for instruction files
// in the directories the touched files and dirs imply and pins the ones the
// chat does not hold yet. Pinned files are never rewritten here; Refresh
// re-reads them. Every failure is logged and swallowed: it must never fail
// the step.
func (p *Server) discoverInstructionContext(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	agent database.WorkspaceAgent,
	chat database.Chat,
	files, dirs []string,
) {
	files, dirs = agentTouchedPaths(files, dirs, agent.OperatingSystem)
	candidates := candidateInstructionDirs(files, dirs, agentWorkingDirectory(agent))
	if len(candidates) == 0 {
		return
	}
	logger := p.logger.With(slog.F("chat_id", chat.ID), slog.F("agent_id", agent.ID))
	p.instructionProbes.forget(agent.ID, invalidatedProbeDirs(files, dirs))

	//nolint:gocritic // Chatd reads the chat's pinned rows as the daemon subject.
	dbCtx := dbauthz.AsChatd(ctx)
	rows, err := p.db.ListChatContextResourcesByChatID(dbCtx, chat.ID)
	if err != nil {
		logger.Debug(ctx, "list pinned context for instruction discovery", slog.Error(err))
		return
	}
	if contextUsageOf(rows).full() {
		return
	}
	now := p.clock.Now()
	probe := selectInstructionProbes(candidates, pinnedInstructionDirs(rows), func(dir string) bool {
		return p.instructionProbes.negative(now, agent.ID, dir)
	})
	if len(probe) == 0 {
		return
	}
	if len(probe) > maxInstructionProbesPerStep {
		probe = probe[:maxInstructionProbesPerStep]
	}

	resolved, probed := resolveInstructionDirs(ctx, logger, conn, probe)
	if len(probed) == 0 {
		return
	}
	foundDirs := make(map[string]struct{}, len(resolved))
	for _, file := range resolved {
		foundDirs[pathKey(agentPath(file.Directory))] = struct{}{}
	}
	empty := slices.DeleteFunc(slices.Clone(probed), func(dir string) bool {
		_, found := foundDirs[pathKey(dir)]
		return found
	})
	p.instructionProbes.markNegative(now, agent.ID, empty)

	pinned, err := p.applyDiscoveredInstructionFiles(ctx, chat.ID, agent.ID, resolved, probed, nil)
	if err != nil {
		logger.Warn(ctx, "pin discovered instruction files", slog.Error(err))
		return
	}
	if pinned == 0 {
		return
	}
	logger.Debug(ctx, "pinned discovered instruction files", slog.F("pinned", pinned))
	updated, err := p.db.GetChatByID(dbCtx, chat.ID)
	if err != nil {
		logger.Warn(ctx, "read chat after instruction discovery", slog.Error(err))
		return
	}
	p.publishChatPubsubEvents([]database.Chat{updated}, codersdk.ChatWatchEventKindContextDirty)
}

// contextUsage is what a chat's pinned rows hold against the discovered
// caps and the whole-chat caps every addition path shares.
type contextUsage struct {
	resources, discoveredFiles    int
	contentBytes, discoveredBytes int64
}

func contextUsageOf(rows []database.ChatContextResource) contextUsage {
	var usage contextUsage
	for _, row := range rows {
		usage.resources++
		readable := row.Status == database.WorkspaceAgentContextResourceStatusOk
		if readable && row.BodyKind != database.WorkspaceAgentContextBodyKindMcpConfig && row.BodyKind != database.WorkspaceAgentContextBodyKindMcpServer {
			usage.contentBytes += row.SizeBytes
		}
		if row.Discovered {
			usage.discoveredFiles++
			if readable {
				usage.discoveredBytes += row.SizeBytes
			}
		}
	}
	return usage
}

// full reports whether a row cap leaves no room for another discovered file.
func (u contextUsage) full() bool {
	return u.discoveredFiles >= maxDiscoveredInstructionFiles || u.resources >= maxChatContextResources
}

// fits reports whether size more readable bytes stay within both byte caps.
func (u contextUsage) fits(size int64) bool {
	return size <= maxDiscoveredInstructionBytes-u.discoveredBytes && size <= maxChatContextContentBytes-u.contentBytes
}

// pinInstructionFiles pins, as discovered rows, the resolved files whose
// source the chat's inventory (rows) does not hold, in the order resolved,
// which is shallowest first. Content past a byte cap is pinned as excluded;
// files past a row cap are not pinned. It returns how many it pinned.
func pinInstructionFiles(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
	rows []database.ChatContextResource,
	resolved []workspacesdk.ContextInstructionFile,
) (int, error) {
	held := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		held[pathKey(agentPath(row.Source))] = struct{}{}
	}
	usage := contextUsageOf(rows)
	pinned := 0
	for _, file := range resolved {
		if usage.full() {
			break
		}
		key := pathKey(agentPath(file.Source))
		if _, ok := held[key]; ok || !database.WorkspaceAgentContextResourceStatus(file.Status).Valid() {
			continue
		}
		size := int64(math.MaxInt64)
		if file.SizeBytes <= math.MaxInt64 {
			size = int64(file.SizeBytes)
		}
		if file.Status == string(database.WorkspaceAgentContextResourceStatusOk) {
			if usage.fits(size) {
				usage.contentBytes += size
				usage.discoveredBytes += size
			} else {
				file.Status = string(database.WorkspaceAgentContextResourceStatusExcluded)
				file.Error = "chat instruction content cap reached"
				file.Content = ""
			}
		}
		if err := insertDiscoveredInstructionFile(ctx, store, chatID, file, size); err != nil {
			return pinned, xerrors.Errorf("pin discovered instruction file %q: %w", file.Source, err)
		}
		held[key] = struct{}{}
		usage.resources++
		usage.discoveredFiles++
		pinned++
	}
	return pinned, nil
}

// insertDiscoveredInstructionFile stores one resolved file as a discovered
// row. A non-OK status is pinned without a body so the inventory still lists
// the file.
func insertDiscoveredInstructionFile(ctx context.Context, store database.Store, chatID uuid.UUID, file workspacesdk.ContextInstructionFile, size int64) error {
	contentHash, err := hex.DecodeString(file.ContentHash)
	if err != nil {
		return xerrors.Errorf("decode content hash: %w", err)
	}
	body, err := protojson.Marshal(&agentproto.InstructionFileBody{Content: []byte(file.Content)})
	if err != nil {
		return xerrors.Errorf("encode instruction body: %w", err)
	}
	if err := store.InsertChatContextDiscoveredResource(ctx, database.InsertChatContextDiscoveredResourceParams{
		ChatID:      chatID,
		Source:      file.Source,
		BodyKind:    database.WorkspaceAgentContextBodyKindInstructionFile,
		Body:        body,
		ContentHash: contentHash,
		SizeBytes:   size,
		Status:      database.WorkspaceAgentContextResourceStatus(file.Status),
		Error:       file.Error,
	}); err != nil {
		return xerrors.Errorf("insert discovered resource: %w", err)
	}
	return nil
}

// applyDiscoveredInstructionFiles pins a probe's answer against the chat's
// current inventory, and drops it once the chat is bound to another agent.
// A refresh passes the discovered rows it captured, so rows still unchanged
// in a probed directory take the probe's answer and newer state wins.
func (p *Server) applyDiscoveredInstructionFiles(
	ctx context.Context,
	chatID, agentID uuid.UUID,
	resolved []workspacesdk.ContextInstructionFile,
	probed []string,
	captured []database.ChatContextResource,
) (int, error) {
	//nolint:gocritic // Chatd pins discovered rows onto a chat it does not own.
	ctx = dbauthz.AsChatd(ctx)
	var pinned int
	err := database.ReadModifyUpdate(p.db, func(tx database.Store) error {
		pinned = 0
		bound, err := tx.LockChatContextForWrite(ctx, chatID)
		if err != nil {
			return xerrors.Errorf("lock chat context: %w", err)
		}
		if !bound.Valid || bound.UUID != agentID {
			return nil
		}
		rows, err := tx.ListChatContextResourcesByChatID(ctx, chatID)
		if err != nil {
			return xerrors.Errorf("list chat context resources: %w", err)
		}
		rows, kept, err := releaseCapturedRows(ctx, tx, chatID, rows, captured, probed)
		if err != nil {
			return err
		}
		files := slices.DeleteFunc(slices.Clone(resolved), func(file workspacesdk.ContextInstructionFile) bool {
			_, ok := kept[pathKey(agentPath(file.Source))]
			return ok
		})
		pinned, err = pinInstructionFiles(ctx, tx, chatID, rows, files)
		return err
	})
	return pinned, err
}

// releaseCapturedRows deletes the captured rows in probed directories that
// are still as captured, so the probe's answer replaces them. The captured
// sources that changed or disappeared meanwhile are returned by pathKey: a
// newer writer owns them, so the answer must not recreate or overwrite them.
func releaseCapturedRows(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
	rows, captured []database.ChatContextResource,
	probed []string,
) ([]database.ChatContextResource, map[string]struct{}, error) {
	probedDirs := make(map[string]struct{}, len(probed))
	for _, dir := range probed {
		probedDirs[pathKey(dir)] = struct{}{}
	}
	current := make(map[string]database.ChatContextResource, len(rows))
	for _, row := range rows {
		current[pathKey(agentPath(row.Source))] = row
	}
	released := make(map[string]struct{}, len(captured))
	kept := make(map[string]struct{})
	for _, was := range captured {
		if _, ok := probedDirs[pathKey(instructionRowDir(was))]; !ok {
			continue
		}
		key := pathKey(agentPath(was.Source))
		row, ok := current[key]
		if !ok || !row.Discovered || !sameDiscoveredRow(was, row) {
			kept[key] = struct{}{}
			continue
		}
		if err := store.DeleteChatContextDiscoveredResource(ctx, database.DeleteChatContextDiscoveredResourceParams{ChatID: chatID, Source: row.Source}); err != nil {
			return nil, nil, xerrors.Errorf("delete re-read instruction file %q: %w", row.Source, err)
		}
		released[key] = struct{}{}
	}
	rows = slices.DeleteFunc(rows, func(row database.ChatContextResource) bool {
		_, ok := released[pathKey(agentPath(row.Source))]
		return ok
	})
	return rows, kept, nil
}

// rediscoverInstructionContext re-reads the discovered rows a refresh kept
// so nested files come back at their current contents and vanished ones are
// dropped. Best effort: a directory a failed batch left unread keeps its rows.
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
	if _, err := p.applyDiscoveredInstructionFiles(ctx, chat.ID, chat.AgentID.UUID, resolved, probed, captured); err != nil {
		logger.Warn(ctx, "apply instruction rediscovery", slog.Error(err))
	}
}

// sameDiscoveredRow compares every field a pin writes.
func sameDiscoveredRow(a, b database.ChatContextResource) bool {
	return bytes.Equal(a.ContentHash, b.ContentHash) &&
		a.Status == b.Status &&
		a.Error == b.Error &&
		a.SizeBytes == b.SizeBytes &&
		bytes.Equal(a.Body, b.Body)
}

// resolveInstructionDirs asks the agent about dirs in request-sized batches
// and returns the files reported and the directories actually asked about:
// a failed batch (or an agent without the endpoint) ends the round early.
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
// rows, deduplicated and shallowest first, the order the budget favors.
func discoveredInstructionDirs(rows []database.ChatContextResource) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, row := range rows {
		dir := instructionRowDir(row)
		if _, ok := seen[pathKey(dir)]; ok {
			continue
		}
		seen[pathKey(dir)] = struct{}{}
		out = append(out, dir)
	}
	sortShallowestFirst(out)
	return out
}
