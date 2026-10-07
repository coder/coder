package chattool

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

const (
	MaxMemories = 200
	// MemoryConsolidateThreshold is the count from which save results ask the
	// agent to consolidate, and MemoryConsolidateTarget is the count they ask
	// it to get under. The 80% trigger and 70% target mirror the nudge Claude
	// Code gives when its memory index approaches its read limit.
	MemoryConsolidateThreshold = MaxMemories * 8 / 10
	MemoryConsolidateTarget    = MaxMemories * 7 / 10

	MaxMemoryBodyBytes        = 8192
	MaxMemoryDescriptionChars = 150

	ReadMemoryToolName        = "read_memory"
	SaveMemoryToolName        = "save_memory"
	DeleteMemoryToolName      = "delete_memory"
	ConsolidateMemoryToolName = "consolidate_memory"
)

var (
	memoryNameRE      = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	ErrMemoryNotFound = xerrors.New("memory not found")
	ErrMemoryExists   = xerrors.New("memory already exists")
	// ErrMemoryLimit is returned when a write would leave a project with
	// more than MaxMemories memories.
	ErrMemoryLimit = xerrors.New("memory limit reached")
)

// writeProjectMemories runs a memory write in one transaction behind a
// per-project advisory lock, then rejects it if it left more than
// MaxMemories memories. The lock serializes writers so two of them cannot
// both observe room and overshoot the cap, and the transaction means a
// consolidation applies all of its deletes and saves or none of them. The
// count runs as the system because the caller has already been authorized
// to write memories in this project, and a create-only scope may lack the
// read the count would otherwise require.
func writeProjectMemories(ctx context.Context, db database.Store, projectID uuid.UUID, write func(tx database.Store) error) (int64, error) {
	var count int64
	err := db.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, database.GenLockID("chat-project-memory:"+projectID.String())); err != nil {
			return xerrors.Errorf("lock memories: %w", err)
		}
		if err := write(tx); err != nil {
			return err
		}
		//nolint:gocritic // See writeProjectMemories.
		n, err := tx.CountChatProjectMemoriesByProjectID(dbauthz.AsSystemRestricted(ctx), projectID)
		if err != nil {
			return xerrors.Errorf("count memories: %w", err)
		}
		if n > MaxMemories {
			return ErrMemoryLimit
		}
		count = n
		return nil
	}, nil)
	return count, err
}

// InsertProjectMemory creates a project memory under the cap and returns it
// with the project's memory count after the insert.
func InsertProjectMemory(ctx context.Context, db database.Store, params database.InsertChatProjectMemoryParams) (database.ChatProjectMemory, int64, error) {
	var memory database.ChatProjectMemory
	count, err := writeProjectMemories(ctx, db, params.ProjectID, func(tx database.Store) error {
		var err error
		memory, err = insertProjectMemoryTx(ctx, tx, params)
		return err
	})
	return memory, count, err
}

// ConsolidateProjectMemories deletes and inserts project memories in one
// transaction and returns the deleted and inserted rows with the project's
// memory count afterward. Deletes run first, so an insert may reuse a deleted
// name to replace that memory. Any missing delete, duplicate insert, or result
// over the cap rolls back every change.
func ConsolidateProjectMemories(ctx context.Context, db database.Store, projectID uuid.UUID, deleteNames []string, inserts []database.InsertChatProjectMemoryParams) (deleted, inserted []database.ChatProjectMemory, count int64, err error) {
	count, err = writeProjectMemories(ctx, db, projectID, func(tx database.Store) error {
		// Reset so a retried transaction does not report rows twice.
		deleted, inserted = nil, nil
		for _, name := range deleteNames {
			memory, err := deleteProjectMemory(ctx, tx, projectID, name)
			if err != nil {
				return err
			}
			deleted = append(deleted, memory)
		}
		for _, params := range inserts {
			memory, err := insertProjectMemoryTx(ctx, tx, params)
			if err != nil {
				return err
			}
			inserted = append(inserted, memory)
		}
		return nil
	})
	if err != nil {
		return nil, nil, 0, err
	}
	return deleted, inserted, count, nil
}

func insertProjectMemoryTx(ctx context.Context, db database.Store, params database.InsertChatProjectMemoryParams) (database.ChatProjectMemory, error) {
	memory, err := db.InsertChatProjectMemory(ctx, params)
	if database.IsUniqueViolation(err) {
		return database.ChatProjectMemory{}, xerrors.Errorf("%w: %q", ErrMemoryExists, params.Name)
	}
	return memory, err
}

func deleteProjectMemory(ctx context.Context, db database.Store, projectID uuid.UUID, name string) (database.ChatProjectMemory, error) {
	memory, err := db.DeleteChatProjectMemoryByName(ctx, database.DeleteChatProjectMemoryByNameParams{ProjectID: projectID, Name: name})
	if errors.Is(err, sql.ErrNoRows) {
		return database.ChatProjectMemory{}, xerrors.Errorf("%w: %q", ErrMemoryNotFound, name)
	}
	return memory, err
}

func memoryIntro(projectName string) string {
	return fmt.Sprintf("Memory is durable context shared by every chat in the project %q, including chats started by other users the project is shared with.", projectName)
}

// Memory is a durable memory with its provenance.
type Memory struct {
	Name              string
	Description       string
	Body              string
	CreatedAt         time.Time
	CreatedByUsername string
}

// MemoryInput is the content of a new durable memory.
type MemoryInput struct {
	Name        string
	Description string
	Body        string
}

// MemoryIndexEntry is a compact memory entry for prompt injection.
type MemoryIndexEntry struct {
	Name        string
	Description string
}

// MemoryStore stores durable memories in one scope. Memories are immutable:
// changing one means deleting it and saving its replacement, which
// Consolidate does atomically.
type MemoryStore interface {
	Get(ctx context.Context, name string) (Memory, error)
	List(ctx context.Context) ([]MemoryIndexEntry, error)
	// Insert saves a new memory and returns the memory count afterward.
	Insert(ctx context.Context, input MemoryInput) (int64, error)
	Delete(ctx context.Context, name string) error
	// Consolidate deletes and saves memories in one transaction and returns
	// the memory count afterward.
	Consolidate(ctx context.Context, deleteNames []string, saves []MemoryInput) (int64, error)
}

// MemoryAuditFunc records a committed memory change. oldMemory is zero for a
// create and newMemory is zero for a delete.
type MemoryAuditFunc func(ctx context.Context, action database.AuditAction, oldMemory, newMemory database.ChatProjectMemory)

type projectMemoryStore struct {
	db             database.Store
	projectID      uuid.UUID
	organizationID uuid.UUID
	ownerID        uuid.UUID
	audit          MemoryAuditFunc
}

// NewProjectMemoryStore returns a store scoped to a chat project. New
// memories are attributed to ownerID. audit, when set, is called for every
// memory the store creates or deletes, after the change commits.
func NewProjectMemoryStore(db database.Store, projectID, organizationID, ownerID uuid.UUID, audit MemoryAuditFunc) MemoryStore {
	return projectMemoryStore{db: db, projectID: projectID, organizationID: organizationID, ownerID: ownerID, audit: audit}
}

func (s projectMemoryStore) auditChange(ctx context.Context, action database.AuditAction, oldMemory, newMemory database.ChatProjectMemory) {
	if s.audit != nil {
		s.audit(ctx, action, oldMemory, newMemory)
	}
}

func (s projectMemoryStore) Get(ctx context.Context, name string) (Memory, error) {
	row, err := s.db.GetChatProjectMemoryByName(ctx, database.GetChatProjectMemoryByNameParams{ProjectID: s.projectID, Name: name})
	if errors.Is(err, sql.ErrNoRows) {
		return Memory{}, ErrMemoryNotFound
	}
	if err != nil {
		return Memory{}, err
	}
	return Memory{Name: row.ChatProjectMemory.Name, Description: row.ChatProjectMemory.Description, Body: row.ChatProjectMemory.Body, CreatedAt: row.ChatProjectMemory.CreatedAt, CreatedByUsername: row.CreatedByUsername}, nil
}

func (s projectMemoryStore) List(ctx context.Context) ([]MemoryIndexEntry, error) {
	rows, err := s.db.GetChatProjectMemoriesByProjectID(ctx, s.projectID)
	if err != nil {
		return nil, err
	}
	entries := make([]MemoryIndexEntry, len(rows))
	for i, row := range rows {
		entries[i] = MemoryIndexEntry{Name: row.ChatProjectMemory.Name, Description: row.ChatProjectMemory.Description}
	}
	return entries, nil
}

func (s projectMemoryStore) insertParams(input MemoryInput) database.InsertChatProjectMemoryParams {
	return database.InsertChatProjectMemoryParams{
		ID: uuid.NullUUID{}, ProjectID: s.projectID, OrganizationID: s.organizationID,
		Name: input.Name, Description: input.Description, Body: input.Body, CreatedBy: s.ownerID,
	}
}

func (s projectMemoryStore) Insert(ctx context.Context, input MemoryInput) (int64, error) {
	memory, count, err := InsertProjectMemory(ctx, s.db, s.insertParams(input))
	if err != nil {
		return 0, err
	}
	s.auditChange(ctx, database.AuditActionCreate, database.ChatProjectMemory{}, memory)
	return count, nil
}

func (s projectMemoryStore) Delete(ctx context.Context, name string) error {
	memory, err := deleteProjectMemory(ctx, s.db, s.projectID, name)
	if err != nil {
		return err
	}
	s.auditChange(ctx, database.AuditActionDelete, memory, database.ChatProjectMemory{})
	return nil
}

func (s projectMemoryStore) Consolidate(ctx context.Context, deleteNames []string, saves []MemoryInput) (int64, error) {
	inserts := make([]database.InsertChatProjectMemoryParams, len(saves))
	for i, input := range saves {
		inserts[i] = s.insertParams(input)
	}
	deleted, inserted, count, err := ConsolidateProjectMemories(ctx, s.db, s.projectID, deleteNames, inserts)
	if err != nil {
		return 0, err
	}
	for _, memory := range deleted {
		s.auditChange(ctx, database.AuditActionDelete, memory, database.ChatProjectMemory{})
	}
	for _, memory := range inserted {
		s.auditChange(ctx, database.AuditActionCreate, database.ChatProjectMemory{}, memory)
	}
	return count, nil
}

// NormalizeMemoryName lowercases and validates a stable memory identifier.
func NormalizeMemoryName(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !memoryNameRE.MatchString(name) {
		return "", xerrors.Errorf("name %q must match %q", name, memoryNameRE.String())
	}
	return name, nil
}

// NormalizeMemoryInput sanitizes and validates a new memory. Memory text is
// shown to every chat in the project, so invisible characters that could
// hide instructions are stripped, and the description is folded onto one
// line because each one is a single line of the memory index.
func NormalizeMemoryInput(name, description, body string) (MemoryInput, error) {
	name, err := NormalizeMemoryName(name)
	if err != nil {
		return MemoryInput{}, err
	}
	description = strings.Join(strings.Fields(codersdk.SanitizePromptText(description)), " ")
	body = codersdk.SanitizePromptText(body)
	if description == "" {
		return MemoryInput{}, xerrors.Errorf("memory %q: description is required", name)
	}
	if utf8.RuneCountInString(description) > MaxMemoryDescriptionChars {
		return MemoryInput{}, xerrors.Errorf("memory %q: description must be at most %d characters", name, MaxMemoryDescriptionChars)
	}
	if body == "" {
		return MemoryInput{}, xerrors.Errorf("memory %q: body is required", name)
	}
	if len(body) > MaxMemoryBodyBytes {
		return MemoryInput{}, xerrors.Errorf("memory %q: body must be at most %d bytes", name, MaxMemoryBodyBytes)
	}
	return MemoryInput{Name: name, Description: description, Body: body}, nil
}

// memoryGuidance tells the model what belongs in durable memory.
const memoryGuidance = "Save facts that will matter in future chats: who the people on this project are and how they like to work; " +
	"corrections you received and approaches that were explicitly confirmed; ongoing work, deadlines, and decisions that cannot be derived from the code or git history; " +
	"and where to find information outside the project, such as an issue tracker or dashboard.\n" +
	"Save a memory as soon as durable information surfaces, without waiting to be asked. " +
	"Do not save anything derivable from the codebase (architecture, file paths, debugging fixes), anything already stated in instructions, or temporary in-progress state. " +
	"Everyone with access to the project can read memories, so never save secrets, credentials, or personal details beyond how people work on the project. " +
	"Never save that something is unknown or undecided. Convert relative dates to absolute dates. " +
	"The memory index is in the conversation: a <project-memory-index> message lists every saved memory, and <project-memory-index-update> messages at the start of later turns list what changed. " +
	"Changes made during a turn, including your own, appear in the next turn's update. " +
	"When a question might be answered by a memory in the index, call read_memory before answering or asking the user. " +
	"Memories may be stale or wrong; verify before relying on one. Memories cannot be edited: replace one that no longer holds with consolidate_memory, or delete it."

// FormatMemoryGuidance renders the stable durable-memory prompt block. It
// carries no memory state: the index travels in conversation messages
// instead, so tool definitions and the system prompt stay identical as
// memories change and the provider's cached prefix survives every write.
func FormatMemoryGuidance(projectName string) string {
	return "<memory>\n" + memoryIntro(projectName) + "\n" + memoryGuidance + "\n</memory>"
}

const (
	memoryIndexTag       = "<project-memory-index>"
	memoryIndexUpdateTag = "<project-memory-index-update>"
	memoryIndexAddedHdr  = "Added or changed:"
	memoryIndexRemoveHdr = "Removed:"
)

// FormatMemoryIndexSnapshot renders the full memory index as a conversation
// message. Every entry is listed; descriptions are single lines, so the index
// at MaxMemories stays around 46KB.
func FormatMemoryIndexSnapshot(entries []MemoryIndexEntry) string {
	var b strings.Builder
	_, _ = b.WriteString(memoryIndexTag + "\n")
	if len(entries) == 0 {
		_, _ = b.WriteString("No memories saved yet.\n")
	}
	for _, entry := range entries {
		_, _ = fmt.Fprintf(&b, "- %s: %s\n", entry.Name, entry.Description)
	}
	_, _ = b.WriteString("</project-memory-index>")
	return b.String()
}

// FormatMemoryIndexUpdate renders the change to the memory index since the
// model last saw it.
func FormatMemoryIndexUpdate(changed []MemoryIndexEntry, removed []string) string {
	var b strings.Builder
	_, _ = b.WriteString(memoryIndexUpdateTag + "\n")
	if len(changed) > 0 {
		_, _ = b.WriteString(memoryIndexAddedHdr + "\n")
		for _, entry := range changed {
			_, _ = fmt.Fprintf(&b, "- %s: %s\n", entry.Name, entry.Description)
		}
	}
	if len(removed) > 0 {
		_, _ = b.WriteString(memoryIndexRemoveHdr + "\n")
		for _, name := range removed {
			_, _ = fmt.Fprintf(&b, "- %s\n", name)
		}
	}
	_, _ = b.WriteString("</project-memory-index-update>")
	return b.String()
}

// ReplayMemoryIndex rebuilds the index the model has seen from memory index
// messages in conversation order, as name to description. ok is false when
// no snapshot is present, for example on a chat's first turn or after
// compaction dropped the earlier snapshot. Text that is not an index message
// is ignored.
func ReplayMemoryIndex(texts []string) (seen map[string]string, ok bool) {
	for _, text := range texts {
		switch {
		case strings.HasPrefix(text, memoryIndexTag):
			seen = make(map[string]string)
			ok = true
			for _, line := range strings.Split(text, "\n")[1:] {
				if name, description, found := parseMemoryIndexLine(line); found {
					seen[name] = description
				}
			}
		case strings.HasPrefix(text, memoryIndexUpdateTag) && ok:
			removing := false
			for _, line := range strings.Split(text, "\n")[1:] {
				switch line {
				case memoryIndexAddedHdr:
					removing = false
					continue
				case memoryIndexRemoveHdr:
					removing = true
					continue
				}
				if removing {
					if name, found := strings.CutPrefix(line, "- "); found {
						delete(seen, name)
					}
					continue
				}
				if name, description, found := parseMemoryIndexLine(line); found {
					seen[name] = description
				}
			}
		}
	}
	return seen, ok
}

func parseMemoryIndexLine(line string) (name, description string, ok bool) {
	rest, found := strings.CutPrefix(line, "- ")
	if !found {
		return "", "", false
	}
	name, description, found = strings.Cut(rest, ": ")
	if !found || !memoryNameRE.MatchString(name) {
		return "", "", false
	}
	return name, description, true
}

// DiffMemoryIndex returns the entries that are new or whose description
// changed since seen, and the names no longer present, both sorted by name.
func DiffMemoryIndex(seen map[string]string, current []MemoryIndexEntry) (changed []MemoryIndexEntry, removed []string) {
	present := make(map[string]struct{}, len(current))
	for _, entry := range current {
		present[entry.Name] = struct{}{}
		if description, ok := seen[entry.Name]; !ok || description != entry.Description {
			changed = append(changed, entry)
		}
	}
	for name := range seen {
		if _, ok := present[name]; !ok {
			removed = append(removed, name)
		}
	}
	slices.SortFunc(changed, func(a, b MemoryIndexEntry) int { return strings.Compare(a.Name, b.Name) })
	slices.Sort(removed)
	return changed, removed
}

type readMemoryArgs struct {
	Name string `json:"name" description:"The name of the memory to read."`
}
type saveMemoryArgs struct {
	Name        string `json:"name" description:"Stable lowercase name for the memory."`
	Description string `json:"description" description:"One-line summary shown in the memory index."`
	Body        string `json:"body" description:"Full durable markdown memory body."`
}
type deleteMemoryArgs struct {
	Name string `json:"name" description:"The name of the memory to delete."`
}
type consolidateMemoryArgs struct {
	Delete []string         `json:"delete,omitempty" description:"Names of memories to remove: stale, wrong, superseded, or folded into a memory in save."`
	Save   []saveMemoryArgs `json:"save,omitempty" description:"Memories to create, such as merged replacements for deleted ones. A name in delete may be reused here to replace that memory."`
}

// consolidateNudge asks the agent to consolidate once memory nears the cap,
// modeled on the in-turn nudge Claude Code gives when its memory index nears
// its read limit.
func consolidateNudge(count int64) string {
	return fmt.Sprintf("memory is %d/%d, approaching the limit. Consolidate it to under %d memories now with %s: "+
		"merge overlapping memories into one, and drop stale, wrong, or superseded ones.",
		count, MaxMemories, MemoryConsolidateTarget, ConsolidateMemoryToolName)
}

func memoryWriteResult(result map[string]any, count int64) fantasy.ToolResponse {
	result["count"] = count
	if count >= MemoryConsolidateThreshold {
		result["warning"] = consolidateNudge(count)
	}
	return toolResponse(result)
}

// ReadMemory returns a tool that reads a full memory body.
// Its description is fixed so memory writes never change tool definitions; the
// index the model picks names from is in the conversation.
func ReadMemory(store MemoryStore) fantasy.AgentTool {
	return fantasy.NewAgentTool(ReadMemoryToolName, "Read a memory by name. The memory index in the conversation lists every saved memory.", func(ctx context.Context, args readMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		if store == nil {
			return fantasy.NewTextErrorResponse("memory store is not configured"), nil
		}
		name, err := NormalizeMemoryName(args.Name)
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		memory, err := store.Get(ctx, name)
		switch {
		case errors.Is(err, ErrMemoryNotFound):
			return fantasy.NewTextErrorResponse("memory was not found"), nil
		case err != nil:
			return fantasy.NewTextErrorResponse("failed to read memory"), nil
		}
		return toolResponse(map[string]any{"name": memory.Name, "description": memory.Description, "body": memory.Body, "created_at": memory.CreatedAt, "created_by": memory.CreatedByUsername}), nil
	})
}

// SaveMemory returns a tool that creates a durable memory.
func SaveMemory(store MemoryStore, projectName string) fantasy.AgentTool {
	return fantasy.NewAgentTool(SaveMemoryToolName, "Save a new durable memory. "+memoryIntro(projectName)+
		" Memories cannot be edited; to change one, replace it with "+ConsolidateMemoryToolName+".", func(ctx context.Context, args saveMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		if store == nil {
			return fantasy.NewTextErrorResponse("memory store is not configured"), nil
		}
		input, err := NormalizeMemoryInput(args.Name, args.Description, args.Body)
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		count, err := store.Insert(ctx, input)
		switch {
		case errors.Is(err, ErrMemoryExists):
			return fantasy.NewTextErrorResponse(fmt.Sprintf("a memory named %q already exists; read it, then replace it with %s by deleting %q and saving the new version in one call", input.Name, ConsolidateMemoryToolName, input.Name)), nil
		case errors.Is(err, ErrMemoryLimit):
			// The agent resolves a full project itself, in this turn, with
			// the tools it already has; there is no background cleanup.
			return fantasy.NewTextErrorResponse(fmt.Sprintf("memory is full (%d/%d); use %s to merge overlapping memories and drop stale ones, and include this memory in its save list", MaxMemories, MaxMemories, ConsolidateMemoryToolName)), nil
		case err != nil:
			return fantasy.NewTextErrorResponse("failed to save memory"), nil
		}
		return memoryWriteResult(map[string]any{"saved": input.Name}, count), nil
	})
}

// DeleteMemory returns a tool that deletes a durable memory.
func DeleteMemory(store MemoryStore, projectName string) fantasy.AgentTool {
	return fantasy.NewAgentTool(DeleteMemoryToolName, "Delete a durable memory by name. "+memoryIntro(projectName), func(ctx context.Context, args deleteMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		if store == nil {
			return fantasy.NewTextErrorResponse("memory store is not configured"), nil
		}
		name, err := NormalizeMemoryName(args.Name)
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		err = store.Delete(ctx, name)
		switch {
		case errors.Is(err, ErrMemoryNotFound):
			return fantasy.NewTextErrorResponse("memory was not found"), nil
		case err != nil:
			return fantasy.NewTextErrorResponse("failed to delete memory"), nil
		}
		return toolResponse(map[string]any{"deleted": name}), nil
	})
}

// ConsolidateMemory returns a tool that deletes and saves memories in one
// transaction, so the agent can rewrite the memory set without leaving it
// half-consolidated or letting another chat claim the freed room.
func ConsolidateMemory(store MemoryStore, projectName string) fantasy.AgentTool {
	return fantasy.NewAgentTool(ConsolidateMemoryToolName, "Delete and save memories in one atomic step: either every change applies or none does. "+memoryIntro(projectName)+
		" Use it when memory is nearly full, or to replace a memory that no longer holds. Read the memories you are merging first, then: "+
		"merge overlapping memories into one, delete memories that are stale, contradicted, or superseded, and convert relative dates to absolute dates. "+
		"Keep what future chats need; drop what they do not.", func(ctx context.Context, args consolidateMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		if store == nil {
			return fantasy.NewTextErrorResponse("memory store is not configured"), nil
		}
		if len(args.Delete) == 0 && len(args.Save) == 0 {
			return fantasy.NewTextErrorResponse("delete or save at least one memory"), nil
		}
		if len(args.Delete) > MaxMemories || len(args.Save) > MaxMemories {
			return fantasy.NewTextErrorResponse(fmt.Sprintf("delete and save take at most %d memories each", MaxMemories)), nil
		}
		deleteNames := make([]string, len(args.Delete))
		for i, raw := range args.Delete {
			name, err := NormalizeMemoryName(raw)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			deleteNames[i] = name
		}
		saves := make([]MemoryInput, len(args.Save))
		savedNames := make([]string, len(args.Save))
		for i, save := range args.Save {
			input, err := NormalizeMemoryInput(save.Name, save.Description, save.Body)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			saves[i] = input
			savedNames[i] = input.Name
		}
		count, err := store.Consolidate(ctx, deleteNames, saves)
		switch {
		case errors.Is(err, ErrMemoryNotFound), errors.Is(err, ErrMemoryExists):
			return fantasy.NewTextErrorResponse(err.Error() + "; no changes were applied"), nil
		case errors.Is(err, ErrMemoryLimit):
			return fantasy.NewTextErrorResponse(fmt.Sprintf("this would leave more than %d memories; delete more and retry. No changes were applied", MaxMemories)), nil
		case err != nil:
			return fantasy.NewTextErrorResponse("failed to consolidate memory; no changes were applied"), nil
		}
		return memoryWriteResult(map[string]any{"deleted": deleteNames, "saved": savedNames}, count), nil
	})
}
