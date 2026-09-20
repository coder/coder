package chatd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	// instructionProbeTTL is how long a directory the agent reported empty
	// is skipped, unless a later step makes it stale.
	instructionProbeTTL = 10 * time.Minute
	// maxInstructionProbeEntries bounds each probe cache across all agents;
	// a cache is reset when it fills.
	maxInstructionProbeEntries = 4096
	// maxDiscoveredInstructionBytes caps the readable discovered content a
	// chat holds across all steps, since every pinned body is rendered into
	// every later prompt; files past the cap are pinned as excluded.
	maxDiscoveredInstructionBytes = 1 << 20
	// maxDiscoveredInstructionFiles caps the discovered rows a chat holds,
	// whatever their status; the byte cap alone leaves bodyless rows unbounded.
	maxDiscoveredInstructionFiles = 256
	// pendingProbeTTL is how long a directory a command ran in is re-probed
	// on later touches of its tree: a background or timed-out command may
	// still be writing after its result returned.
	pendingProbeTTL = 10 * time.Minute
)

// instructionFileNames mirrors the agent resolver's recognized names so a
// tool that writes one of them re-probes its directory.
var instructionFileNames = []string{"AGENTS.md", "CLAUDE.md", ".cursorrules"}

// instructionProbeCache remembers, per agent, directories reported empty
// (shared by every chat), directories one chat's row cap kept out (that
// chat only), and directories a command ran in, which are re-probed for a while.
type instructionProbeCache struct {
	mu      sync.Mutex
	entries map[instructionProbeKey]time.Time
	pending map[instructionProbeKey]time.Time
}

// instructionProbeKey has chatID uuid.Nil for entries every chat shares.
type instructionProbeKey struct {
	agentID uuid.UUID
	chatID  uuid.UUID
	dir     string
}

func (c *instructionProbeCache) negative(now time.Time, agentID, chatID uuid.UUID, dir string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, scope := range []uuid.UUID{uuid.Nil, chatID} {
		if expiry, ok := c.entries[instructionProbeKey{agentID: agentID, chatID: scope, dir: pathKey(dir)}]; ok && now.Before(expiry) {
			return true
		}
	}
	return false
}

// markPending records directories a command ran in.
func (c *instructionProbeCache) markPending(now time.Time, agentID uuid.UUID, dirs []string) {
	keys := make([]instructionProbeKey, 0, len(dirs))
	for _, dir := range dirs {
		keys = append(keys, instructionProbeKey{agentID: agentID, dir: pathKey(dir)})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = insertBounded(c.pending, now, pendingProbeTTL, keys)
}

// isPending reports whether a command ran recently in dir or an ancestor,
// since the command may be writing anywhere in its tree.
func (c *instructionProbeCache) isPending(now time.Time, agentID uuid.UUID, dir string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		if expiry, ok := c.pending[instructionProbeKey{agentID: agentID, dir: pathKey(dir)}]; ok && now.Before(expiry) {
			return true
		}
		if isRootAgentPath(dir) {
			return false
		}
		dir = path.Dir(dir)
	}
}

func (c *instructionProbeCache) markNegative(now time.Time, agentID, chatID uuid.UUID, dirs []string) {
	keys := make([]instructionProbeKey, 0, len(dirs))
	for _, dir := range dirs {
		keys = append(keys, instructionProbeKey{agentID: agentID, chatID: chatID, dir: pathKey(dir)})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = insertBounded(c.entries, now, instructionProbeTTL, keys)
}

// insertBounded records keys with an expiry of now plus ttl. When the map
// would exceed maxInstructionProbeEntries, expired entries go first and then
// the whole map, which only costs the next touch a probe it may not need.
func insertBounded(m map[instructionProbeKey]time.Time, now time.Time, ttl time.Duration, keys []instructionProbeKey) map[instructionProbeKey]time.Time {
	if len(keys) == 0 {
		return m
	}
	if m == nil {
		m = make(map[instructionProbeKey]time.Time)
	}
	if len(m)+len(keys) > maxInstructionProbeEntries {
		for key, expiry := range m {
			if !now.Before(expiry) {
				delete(m, key)
			}
		}
		if len(m)+len(keys) > maxInstructionProbeEntries {
			m = make(map[instructionProbeKey]time.Time)
		}
	}
	for _, key := range keys {
		m[key] = now.Add(ttl)
	}
	return m
}

func (c *instructionProbeCache) forget(agentID, chatID uuid.UUID, dirs []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, dir := range dirs {
		for _, scope := range []uuid.UUID{uuid.Nil, chatID} {
			delete(c.entries, instructionProbeKey{agentID: agentID, chatID: scope, dir: pathKey(dir)})
		}
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
// deduplicated and shallowest first so the request cap keeps the widest scopes.
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

// staleInstructionDirs lists, by pathKey, the directories an instruction
// file was written to and the explicit execute workdirs, whose files may
// have changed during the step.
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

// pinnedInstructionDirs lists, by pathKey, the directories whose files the
// chat already holds, and separately those whose excluded file would fit
// the bytes now free, so a later touch re-reads them.
func pinnedInstructionDirs(rows []database.ChatContextResource) (pinned, freed map[string]struct{}) {
	var used int64
	for _, row := range rows {
		if row.Discovered && row.BodyKind == database.WorkspaceAgentContextBodyKindInstructionFile && row.Status == database.WorkspaceAgentContextResourceStatusOk {
			used += row.SizeBytes
		}
	}
	free := maxDiscoveredInstructionBytes - used
	freed = make(map[string]struct{})
	pinned = make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.BodyKind != database.WorkspaceAgentContextBodyKindInstructionFile {
			continue
		}
		key := pathKey(instructionRowDir(row))
		if row.Discovered && row.Status == database.WorkspaceAgentContextResourceStatusExcluded && row.SizeBytes <= free {
			freed[key] = struct{}{}
		}
		pinned[key] = struct{}{}
	}
	for key := range freed {
		delete(pinned, key)
	}
	return pinned, freed
}

// selectInstructionProbes keeps the stale candidates and those neither
// pinned nor cached negative. pinnedDirs and stale are keyed by pathKey.
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
// directories whose source the probe did not return. stale and returned
// are keyed by pathKey.
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

type discoveryReconciliation struct {
	pinned, removed int
	// capped lists the directories whose new files the row cap kept out.
	capped []string
}

// discoveryBudget is the share of the chat's discovered caps that rows
// outside a reconciliation's reach already consume.
type discoveryBudget struct {
	bytes int64
	files int
}

// reconcileDiscoveredInstructionFiles removes the discovered rows in stale,
// probed directories the agent no longer returned, then pins the resolved
// files against the chat's inventory (rows): a file pinned under another
// spelling of a Windows path keeps that spelling, since the row key is
// case-sensitive, and a file the snapshot owns is left to it. Content past
// the byte cap is pinned as excluded and a new source past the row cap is not
// pinned; reserved is the share of both caps held by rows outside the inventory.
func reconcileDiscoveredInstructionFiles(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
	rows []database.ChatContextResource,
	resolved []workspacesdk.ContextInstructionFile,
	stale map[string]struct{},
	probed []string,
	reserved discoveryBudget,
) (discoveryReconciliation, error) {
	var result discoveryReconciliation
	spellings := make(map[string]string, len(rows))
	snapshotOwned := make(map[string]struct{}, len(rows))
	heldBytes := make(map[string]int64, len(rows))
	used := reserved.bytes
	count := reserved.files
	for _, row := range rows {
		if row.BodyKind != database.WorkspaceAgentContextBodyKindInstructionFile {
			continue
		}
		key := pathKey(agentPath(row.Source))
		spellings[key] = row.Source
		if !row.Discovered {
			snapshotOwned[key] = struct{}{}
			continue
		}
		count++
		if row.Status == database.WorkspaceAgentContextResourceStatusOk {
			heldBytes[key] = row.SizeBytes
			used += row.SizeBytes
		}
	}
	returned := make(map[string]struct{}, len(resolved))
	for _, file := range resolved {
		returned[pathKey(agentPath(file.Source))] = struct{}{}
	}
	// Vanished files go first so a renamed file takes over the budget and
	// the row its old name held.
	for _, source := range removedDiscoveredSources(rows, stale, probed, returned) {
		if err := store.DeleteChatContextDiscoveredResource(ctx, database.DeleteChatContextDiscoveredResourceParams{ChatID: chatID, Source: source}); err != nil {
			return result, xerrors.Errorf("delete removed instruction file %q: %w", source, err)
		}
		key := pathKey(agentPath(source))
		used -= heldBytes[key]
		delete(heldBytes, key)
		delete(spellings, key)
		count--
		result.removed++
	}
	for _, file := range resolved {
		key := pathKey(agentPath(file.Source))
		if _, owned := snapshotOwned[key]; owned {
			continue
		}
		if !database.WorkspaceAgentContextResourceStatus(file.Status).Valid() {
			// A status this server does not know leaves the row as it was.
			continue
		}
		source, known := spellings[key]
		if known {
			file.Source = source
		} else if count >= maxDiscoveredInstructionFiles {
			if dir := agentPath(file.Directory); !slices.Contains(result.capped, dir) {
				result.capped = append(result.capped, dir)
			}
			continue
		}
		// A re-read replaces the bytes an earlier read held, whatever the
		// file's status is now.
		used -= heldBytes[key]
		delete(heldBytes, key)
		size := int64(math.MaxInt64)
		if file.SizeBytes <= math.MaxInt64 {
			size = int64(file.SizeBytes)
		}
		if file.Status == string(database.WorkspaceAgentContextResourceStatusOk) {
			if size > maxDiscoveredInstructionBytes-used {
				file.Status = string(database.WorkspaceAgentContextResourceStatusExcluded)
				file.Error = fmt.Sprintf("discovered instruction content cap of %d bytes reached", maxDiscoveredInstructionBytes)
				file.Content = ""
			} else {
				used += size
				heldBytes[key] = size
			}
		}
		if err := pinDiscoveredInstructionFile(ctx, store, chatID, file, size); err != nil {
			return result, xerrors.Errorf("pin discovered instruction file %q: %w", file.Source, err)
		}
		result.pinned++
		if !known {
			spellings[key] = file.Source
			count++
		}
	}
	return result, nil
}

// pinDiscoveredInstructionFile stores one resolved file as a discovered row.
// A non-OK status is pinned without a body so the inventory still lists the
// file and a file that stopped being readable drops its earlier body.
func pinDiscoveredInstructionFile(ctx context.Context, store database.Store, chatID uuid.UUID, file workspacesdk.ContextInstructionFile, size int64) error {
	contentHash, err := hex.DecodeString(file.ContentHash)
	if err != nil {
		return xerrors.Errorf("decode content hash: %w", err)
	}
	body, err := protojson.Marshal(&agentproto.InstructionFileBody{Content: []byte(file.Content)})
	if err != nil {
		return xerrors.Errorf("encode instruction body: %w", err)
	}
	if err := store.UpsertChatContextDiscoveredResource(ctx, database.UpsertChatContextDiscoveredResourceParams{
		ChatID:      chatID,
		Source:      file.Source,
		BodyKind:    database.WorkspaceAgentContextBodyKindInstructionFile,
		Body:        body,
		ContentHash: contentHash,
		SizeBytes:   size,
		Status:      database.WorkspaceAgentContextResourceStatus(file.Status),
		Error:       file.Error,
	}); err != nil {
		return xerrors.Errorf("upsert discovered resource: %w", err)
	}
	return nil
}
