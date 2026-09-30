package chattool

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
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

	MaxMemoryIndexLines = 200
	// MaxMemoryIndexBytes fits MaxMemories entries at the longest name and
	// description, so a project at the cap always has every name listed;
	// read_memory takes an exact name and there is no other way to find one.
	MaxMemoryIndexBytes       = 48 * 1024
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
// transaction and returns the project's memory count afterward. Deletes run
// first, so an insert may reuse a deleted name to replace that memory. Any
// missing delete, duplicate insert, or result over the cap rolls back every
// change.
func ConsolidateProjectMemories(ctx context.Context, db database.Store, projectID uuid.UUID, deleteNames []string, inserts []database.InsertChatProjectMemoryParams) (int64, error) {
	return writeProjectMemories(ctx, db, projectID, func(tx database.Store) error {
		for _, name := range deleteNames {
			if err := deleteProjectMemory(ctx, tx, projectID, name); err != nil {
				return err
			}
		}
		for _, params := range inserts {
			if _, err := insertProjectMemoryTx(ctx, tx, params); err != nil {
				return err
			}
		}
		return nil
	})
}

func insertProjectMemoryTx(ctx context.Context, db database.Store, params database.InsertChatProjectMemoryParams) (database.ChatProjectMemory, error) {
	memory, err := db.InsertChatProjectMemory(ctx, params)
	if database.IsUniqueViolation(err) {
		return database.ChatProjectMemory{}, xerrors.Errorf("%w: %q", ErrMemoryExists, params.Name)
	}
	return memory, err
}

func deleteProjectMemory(ctx context.Context, db database.Store, projectID uuid.UUID, name string) error {
	rows, err := db.DeleteChatProjectMemoryByName(ctx, database.DeleteChatProjectMemoryByNameParams{ProjectID: projectID, Name: name})
	if errors.Is(err, sql.ErrNoRows) || (err == nil && rows == 0) {
		return xerrors.Errorf("%w: %q", ErrMemoryNotFound, name)
	}
	return err
}

func memoryIntro(projectName string) string {
	return fmt.Sprintf("Memory is durable context shared by every chat in the project %q.", projectName)
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

type projectMemoryStore struct {
	db             database.Store
	projectID      uuid.UUID
	organizationID uuid.UUID
	ownerID        uuid.UUID
}

// NewProjectMemoryStore returns a store scoped to a chat project. New
// memories are attributed to ownerID.
func NewProjectMemoryStore(db database.Store, projectID, organizationID, ownerID uuid.UUID) MemoryStore {
	return projectMemoryStore{db: db, projectID: projectID, organizationID: organizationID, ownerID: ownerID}
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
	_, count, err := InsertProjectMemory(ctx, s.db, s.insertParams(input))
	return count, err
}

func (s projectMemoryStore) Delete(ctx context.Context, name string) error {
	return deleteProjectMemory(ctx, s.db, s.projectID, name)
}

func (s projectMemoryStore) Consolidate(ctx context.Context, deleteNames []string, saves []MemoryInput) (int64, error) {
	inserts := make([]database.InsertChatProjectMemoryParams, len(saves))
	for i, input := range saves {
		inserts[i] = s.insertParams(input)
	}
	return ConsolidateProjectMemories(ctx, s.db, s.projectID, deleteNames, inserts)
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
	"Never save that something is unknown or undecided. Convert relative dates to absolute dates. " +
	"When a question might be answered by a memory in the index, call read_memory before answering or asking the user. " +
	"Memories may be stale or wrong; verify before relying on one. Memories cannot be edited: replace one that no longer holds with consolidate_memory, or delete it."

// FormatMemoryGuidance renders the stable durable-memory prompt block. It
// carries no per-turn state so the system prompt prefix stays identical
// across turns and remains cacheable; the live index is in the read tool's
// description instead.
func FormatMemoryGuidance(projectName string) string {
	return "<memory>\n" + memoryIntro(projectName) + "\n" + memoryGuidance + "\n</memory>"
}

// FormatMemoryIndexForTool renders the compact memory index for read_memory.
func FormatMemoryIndexForTool(entries []MemoryIndexEntry) string {
	if len(entries) == 0 {
		return "No memories saved yet."
	}

	var b strings.Builder
	_, _ = b.WriteString("Available memories:\n")
	shown := 0
	truncationReserve := len(fmt.Sprintf("%d more memories not shown.", len(entries)))
	for _, entry := range entries {
		if shown >= MaxMemoryIndexLines {
			break
		}
		line := fmt.Sprintf("- %s: %s", entry.Name, entry.Description)
		if b.Len()+len(line)+1+truncationReserve > MaxMemoryIndexBytes {
			break
		}
		_, _ = b.WriteString(line)
		_ = b.WriteByte('\n')
		shown++
	}
	if omitted := len(entries) - shown; omitted > 0 {
		_, _ = b.WriteString(fmt.Sprintf("%d more memories not shown.", omitted))
		return b.String()
	}
	return strings.TrimSuffix(b.String(), "\n")
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
func ReadMemory(store MemoryStore, entries []MemoryIndexEntry) fantasy.AgentTool {
	return fantasy.NewAgentTool(ReadMemoryToolName, "Read a memory by name. "+FormatMemoryIndexForTool(entries), func(ctx context.Context, args readMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		if store == nil {
			return fantasy.NewTextErrorResponse("memory store is not configured"), nil
		}
		name, err := NormalizeMemoryName(args.Name)
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		memory, err := store.Get(ctx, name)
		if err != nil {
			return fantasy.NewTextErrorResponse("memory was not found"), nil
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
