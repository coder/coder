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
	"github.com/coder/coder/v2/codersdk"
)

const (
	MaxMemories               = 200
	MaxMemoryIndexLines       = 200
	MaxMemoryIndexBytes       = 25 * 1024
	MaxMemoryBodyBytes        = 8192
	MaxMemoryDescriptionChars = 150

	ReadMemoryToolName   = "read_memory"
	SaveMemoryToolName   = "save_memory"
	DeleteMemoryToolName = "delete_memory"
)

var (
	memoryNameRE      = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	ErrMemoryNotFound = xerrors.New("memory not found")
	ErrMemoryExists   = xerrors.New("memory already exists")
)

type MemoryScopeKind string

const (
	MemoryScopeProject  MemoryScopeKind = "project"
	MemoryScopePersonal MemoryScopeKind = "personal"
)

// MemoryScope identifies where durable memory is available.
type MemoryScope struct {
	Kind  MemoryScopeKind
	Label string
}

// Intro explains the durable-memory scope to the model.
func (s MemoryScope) Intro() string {
	switch s.Kind {
	case MemoryScopeProject:
		return fmt.Sprintf("Memory is durable context shared by every chat in the project %q.", s.Label)
	case MemoryScopePersonal:
		return "Memory is personal to you and used by your chats that are not in a project. Save preferences and facts about how you work, not project details that belong in a project."
	default:
		return "Memory is durable context."
	}
}

// Memory is a durable memory with its provenance.
type Memory struct {
	Name              string
	Description       string
	Body              string
	UpdatedAt         time.Time
	CreatedByUsername string
}

// MemoryInput is the mutable content of a durable memory.
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

// MemoryStore stores durable memories in one scope.
type MemoryStore interface {
	Get(ctx context.Context, name string) (Memory, error)
	List(ctx context.Context) ([]MemoryIndexEntry, error)
	ListFull(ctx context.Context) ([]Memory, error)
	Count(ctx context.Context) (int64, error)
	Insert(ctx context.Context, input MemoryInput) (Memory, error)
	Upsert(ctx context.Context, input MemoryInput) (Memory, error)
	Delete(ctx context.Context, name string) error
	InTx(func(MemoryStore) error) error
}

type projectMemoryStore struct {
	db             database.Store
	projectID      uuid.UUID
	organizationID uuid.UUID
	chatID         uuid.UUID
	ownerID        uuid.UUID
}

// NewProjectMemoryStore returns a store scoped to a chat project.
func NewProjectMemoryStore(db database.Store, projectID, organizationID, chatID, ownerID uuid.UUID) MemoryStore {
	return projectMemoryStore{db: db, projectID: projectID, organizationID: organizationID, chatID: chatID, ownerID: ownerID}
}

func (s projectMemoryStore) Get(ctx context.Context, name string) (Memory, error) {
	row, err := s.db.GetChatProjectMemoryByName(ctx, database.GetChatProjectMemoryByNameParams{ProjectID: s.projectID, Name: name})
	if errors.Is(err, sql.ErrNoRows) {
		return Memory{}, ErrMemoryNotFound
	}
	if err != nil {
		return Memory{}, err
	}
	return Memory{Name: row.ChatProjectMemory.Name, Description: row.ChatProjectMemory.Description, Body: row.ChatProjectMemory.Body, UpdatedAt: row.ChatProjectMemory.UpdatedAt, CreatedByUsername: row.CreatedByUsername}, nil
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

func (s projectMemoryStore) ListFull(ctx context.Context) ([]Memory, error) {
	rows, err := s.db.GetChatProjectMemoriesByProjectID(ctx, s.projectID)
	if err != nil {
		return nil, err
	}
	memories := make([]Memory, len(rows))
	for i, row := range rows {
		memories[i] = Memory{Name: row.ChatProjectMemory.Name, Description: row.ChatProjectMemory.Description, Body: row.ChatProjectMemory.Body, UpdatedAt: row.ChatProjectMemory.UpdatedAt, CreatedByUsername: row.CreatedByUsername}
	}
	return memories, nil
}

func (s projectMemoryStore) Count(ctx context.Context) (int64, error) {
	return s.db.CountChatProjectMemoriesByProjectID(ctx, s.projectID)
}

func (s projectMemoryStore) Insert(ctx context.Context, input MemoryInput) (Memory, error) {
	memory, err := s.db.InsertChatProjectMemory(ctx, database.InsertChatProjectMemoryParams{
		ID: uuid.NullUUID{}, ProjectID: s.projectID, OrganizationID: s.organizationID,
		Name: input.Name, Description: input.Description, Body: input.Body,
		SourceChatID: uuid.NullUUID{UUID: s.chatID, Valid: s.chatID != uuid.Nil}, CreatedBy: s.ownerID,
	})
	if database.IsUniqueViolation(err) {
		return Memory{}, ErrMemoryExists
	}
	if err != nil {
		return Memory{}, err
	}
	return Memory{Name: memory.Name, Description: memory.Description, Body: memory.Body, UpdatedAt: memory.UpdatedAt}, nil
}

func (s projectMemoryStore) Upsert(ctx context.Context, input MemoryInput) (Memory, error) {
	memory, err := s.db.UpsertChatProjectMemoryByName(ctx, database.UpsertChatProjectMemoryByNameParams{
		ProjectID: s.projectID, OrganizationID: s.organizationID, Name: input.Name, Description: input.Description, Body: input.Body,
		SourceChatID: uuid.NullUUID{UUID: s.chatID, Valid: s.chatID != uuid.Nil}, CreatedBy: s.ownerID,
	})
	if err != nil {
		return Memory{}, err
	}
	return Memory{Name: memory.Name, Description: memory.Description, Body: memory.Body, UpdatedAt: memory.UpdatedAt}, nil
}

func (s projectMemoryStore) Delete(ctx context.Context, name string) error {
	err := s.db.DeleteChatProjectMemoryByName(ctx, database.DeleteChatProjectMemoryByNameParams{ProjectID: s.projectID, Name: name})
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMemoryNotFound
	}
	return err
}

func (s projectMemoryStore) InTx(fn func(MemoryStore) error) error {
	return s.db.InTx(func(tx database.Store) error {
		return fn(projectMemoryStore{db: tx, projectID: s.projectID, organizationID: s.organizationID, chatID: s.chatID, ownerID: s.ownerID})
	}, nil)
}

type personalMemoryStore struct {
	db                             database.Store
	userID, organizationID, chatID uuid.UUID
}

// NewPersonalMemoryStore returns a store scoped to one user and organization.
func NewPersonalMemoryStore(db database.Store, userID, organizationID, chatID uuid.UUID) MemoryStore {
	return personalMemoryStore{db: db, userID: userID, organizationID: organizationID, chatID: chatID}
}

func (s personalMemoryStore) Get(ctx context.Context, name string) (Memory, error) {
	row, err := s.db.GetChatUserMemoryByName(ctx, database.GetChatUserMemoryByNameParams{UserID: s.userID, OrganizationID: s.organizationID, Name: name})
	if errors.Is(err, sql.ErrNoRows) {
		return Memory{}, ErrMemoryNotFound
	}
	if err != nil {
		return Memory{}, err
	}
	return Memory{Name: row.ChatUserMemory.Name, Description: row.ChatUserMemory.Description, Body: row.ChatUserMemory.Body, UpdatedAt: row.ChatUserMemory.UpdatedAt, CreatedByUsername: row.CreatedByUsername}, nil
}

func (s personalMemoryStore) List(ctx context.Context) ([]MemoryIndexEntry, error) {
	rows, err := s.db.GetChatUserMemoriesByUserAndOrganization(ctx, database.GetChatUserMemoriesByUserAndOrganizationParams{UserID: s.userID, OrganizationID: s.organizationID})
	if err != nil {
		return nil, err
	}
	entries := make([]MemoryIndexEntry, len(rows))
	for i, row := range rows {
		entries[i] = MemoryIndexEntry{Name: row.ChatUserMemory.Name, Description: row.ChatUserMemory.Description}
	}
	return entries, nil
}

func (s personalMemoryStore) ListFull(ctx context.Context) ([]Memory, error) {
	rows, err := s.db.GetChatUserMemoriesByUserAndOrganization(ctx, database.GetChatUserMemoriesByUserAndOrganizationParams{UserID: s.userID, OrganizationID: s.organizationID})
	if err != nil {
		return nil, err
	}
	memories := make([]Memory, len(rows))
	for i, row := range rows {
		memories[i] = Memory{Name: row.ChatUserMemory.Name, Description: row.ChatUserMemory.Description, Body: row.ChatUserMemory.Body, UpdatedAt: row.ChatUserMemory.UpdatedAt, CreatedByUsername: row.CreatedByUsername}
	}
	return memories, nil
}

func (s personalMemoryStore) Count(ctx context.Context) (int64, error) {
	return s.db.CountChatUserMemoriesByUserAndOrganization(ctx, database.CountChatUserMemoriesByUserAndOrganizationParams{UserID: s.userID, OrganizationID: s.organizationID})
}

func (s personalMemoryStore) Insert(ctx context.Context, input MemoryInput) (Memory, error) {
	memory, err := s.db.InsertChatUserMemory(ctx, database.InsertChatUserMemoryParams{
		ID: uuid.NullUUID{}, OrganizationID: s.organizationID, UserID: s.userID,
		Name: input.Name, Description: input.Description, Body: input.Body,
		SourceChatID: uuid.NullUUID{UUID: s.chatID, Valid: s.chatID != uuid.Nil},
	})
	if database.IsUniqueViolation(err) {
		return Memory{}, ErrMemoryExists
	}
	if err != nil {
		return Memory{}, err
	}
	return Memory{Name: memory.Name, Description: memory.Description, Body: memory.Body, UpdatedAt: memory.UpdatedAt}, nil
}

func (s personalMemoryStore) Upsert(ctx context.Context, input MemoryInput) (Memory, error) {
	memory, err := s.db.UpsertChatUserMemoryByName(ctx, database.UpsertChatUserMemoryByNameParams{OrganizationID: s.organizationID, UserID: s.userID, Name: input.Name, Description: input.Description, Body: input.Body, SourceChatID: uuid.NullUUID{UUID: s.chatID, Valid: s.chatID != uuid.Nil}})
	if err != nil {
		return Memory{}, err
	}
	return Memory{Name: memory.Name, Description: memory.Description, Body: memory.Body, UpdatedAt: memory.UpdatedAt}, nil
}

func (s personalMemoryStore) Delete(ctx context.Context, name string) error {
	err := s.db.DeleteChatUserMemoryByName(ctx, database.DeleteChatUserMemoryByNameParams{UserID: s.userID, OrganizationID: s.organizationID, Name: name})
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMemoryNotFound
	}
	return err
}

func (s personalMemoryStore) InTx(fn func(MemoryStore) error) error {
	return s.db.InTx(func(tx database.Store) error {
		return fn(personalMemoryStore{db: tx, userID: s.userID, organizationID: s.organizationID, chatID: s.chatID})
	}, nil)
}

// ValidateMemoryName validates a stable memory identifier.
func ValidateMemoryName(name string) error {
	if !memoryNameRE.MatchString(name) {
		return xerrors.Errorf("name must match %q", memoryNameRE.String())
	}
	return nil
}

// NormalizeMemoryText sanitizes durable memory text and removes prompt-index tags.
func NormalizeMemoryText(text string) string {
	text = codersdk.SanitizePromptText(text)
	for _, tag := range []string{"<memory>", "</memory>", "<project-memory>", "</project-memory>"} {
		text = strings.ReplaceAll(text, tag, "")
	}
	return strings.TrimSpace(text)
}

func normalizeMemoryInput(name, description, body string) (MemoryInput, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if err := ValidateMemoryName(name); err != nil {
		return MemoryInput{}, err
	}
	description, body = NormalizeMemoryText(description), NormalizeMemoryText(body)
	if description == "" {
		return MemoryInput{}, xerrors.New("description is required")
	}
	if utf8.RuneCountInString(description) > MaxMemoryDescriptionChars {
		return MemoryInput{}, xerrors.Errorf("description must be at most %d characters", MaxMemoryDescriptionChars)
	}
	if body == "" {
		return MemoryInput{}, xerrors.New("body is required")
	}
	if len(body) > MaxMemoryBodyBytes {
		return MemoryInput{}, xerrors.Errorf("body must be at most %d bytes", MaxMemoryBodyBytes)
	}
	return MemoryInput{Name: name, Description: description, Body: body}, nil
}

// Guidance tells the model what belongs in durable memory for this scope.
// Personal memory deliberately excludes project facts so that chats outside
// a project do not accumulate people, decisions, and deadlines that belong
// in a project's shared memory.
func (s MemoryScope) Guidance() string {
	switch s.Kind {
	case MemoryScopePersonal:
		return personalMemoryGuidance + "\n" + sharedMemoryGuidance
	default:
		return projectMemoryGuidance + "\n" + sharedMemoryGuidance
	}
}

const (
	projectMemoryGuidance = "Save facts that will matter in future chats: who the people on this project are and how they like to work; " +
		"corrections you received and approaches that were explicitly confirmed; ongoing work, deadlines, and decisions that cannot be derived from the code or git history; " +
		"and where to find information outside the project, such as an issue tracker or dashboard."
	personalMemoryGuidance = "Save facts about how this user works that will matter in every future chat: preferences for tone, verbosity, and response format; " +
		"tools, languages, and conventions they favor; and corrections they gave you or approaches they explicitly confirmed for their own workflow. " +
		"Do not save project details: the people, decisions, deadlines, ongoing work, and external links of any project belong in that project's memory, not here."
	sharedMemoryGuidance = "Save a memory as soon as durable information surfaces, without waiting to be asked. " +
		"Do not save anything derivable from the codebase (architecture, file paths, debugging fixes), anything already stated in instructions, or temporary in-progress state. " +
		"Never save that something is unknown or undecided. " +
		"When a question might be answered by a memory in the index, call read_memory before answering or asking the user. " +
		"Memories may be stale or wrong; verify before relying on one and update or delete it when it no longer holds."
)

// FormatMemoryGuidance renders the stable durable-memory prompt block.
func FormatMemoryGuidance(scope MemoryScope) string {
	return "<memory>\n" + scope.Intro() + "\n" + scope.Guidance() + "\n</memory>"
}

// FormatMemoryIndexForTool renders the compact memory index for read_memory.
func FormatMemoryIndexForTool(entries []MemoryIndexEntry) string {
	if len(entries) == 0 {
		return "No memories saved yet."
	}

	const prefix = "Available memories (newest first):\n"
	var b strings.Builder
	_, _ = b.WriteString(prefix)
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

// ReadMemory returns a tool that reads a full memory body.
func ReadMemory(store MemoryStore, _ MemoryScope, entries []MemoryIndexEntry) fantasy.AgentTool {
	return fantasy.NewAgentTool(ReadMemoryToolName, "Read a memory by name. "+FormatMemoryIndexForTool(entries), func(ctx context.Context, args readMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		if store == nil {
			return fantasy.NewTextErrorResponse("memory store is not configured"), nil
		}
		name := strings.ToLower(strings.TrimSpace(args.Name))
		if err := ValidateMemoryName(name); err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		memory, err := store.Get(ctx, name)
		if err != nil {
			return fantasy.NewTextErrorResponse("memory was not found"), nil
		}
		return toolResponse(map[string]any{"name": memory.Name, "description": memory.Description, "body": memory.Body, "updated_at": memory.UpdatedAt, "created_by": memory.CreatedByUsername}), nil
	})
}

// SaveMemory returns a tool that upserts a durable memory.
func SaveMemory(store MemoryStore, scope MemoryScope) fantasy.AgentTool {
	return fantasy.NewAgentTool(SaveMemoryToolName, "Save or update a durable memory by name. Scope: "+scope.Intro(), func(ctx context.Context, args saveMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		if store == nil {
			return fantasy.NewTextErrorResponse("memory store is not configured"), nil
		}
		input, err := normalizeMemoryInput(args.Name, args.Description, args.Body)
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		_, getErr := store.Get(ctx, input.Name)
		if errors.Is(getErr, ErrMemoryNotFound) {
			count, countErr := store.Count(ctx)
			if countErr != nil {
				return fantasy.NewTextErrorResponse("failed to count memories"), nil
			}
			if count >= MaxMemories {
				return fantasy.NewTextErrorResponse("memory limit reached; merge or delete existing memories first"), nil
			}
		} else if getErr != nil {
			return fantasy.NewTextErrorResponse("failed to load memory"), nil
		}
		memory, err := store.Upsert(ctx, input)
		if err != nil {
			return fantasy.NewTextErrorResponse("failed to save memory"), nil
		}
		return toolResponse(map[string]any{"name": memory.Name, "updated_at": memory.UpdatedAt}), nil
	})
}

// DeleteMemory returns a tool that deletes a durable memory.
func DeleteMemory(store MemoryStore, scope MemoryScope) fantasy.AgentTool {
	return fantasy.NewAgentTool(DeleteMemoryToolName, "Delete a durable memory by name. Scope: "+scope.Intro(), func(ctx context.Context, args deleteMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		if store == nil {
			return fantasy.NewTextErrorResponse("memory store is not configured"), nil
		}
		name := strings.ToLower(strings.TrimSpace(args.Name))
		if err := ValidateMemoryName(name); err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		if err := store.Delete(ctx, name); err != nil {
			return fantasy.NewTextErrorResponse("memory was not found"), nil
		}
		return toolResponse(map[string]any{"deleted": name}), nil
	})
}
