package chattool

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

const (
	MaxProjectMemories               = 200
	MaxProjectMemoryIndexLines       = 200
	MaxProjectMemoryIndexBytes       = 25 * 1024
	MaxProjectMemoryBodyBytes        = 8192
	MaxProjectMemoryDescriptionChars = 150

	ReadProjectMemoryToolName   = "read_project_memory"
	SaveProjectMemoryToolName   = "save_project_memory"
	DeleteProjectMemoryToolName = "delete_project_memory"
)

var projectMemoryNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ProjectMemoryOptions configures the project memory tools.
type ProjectMemoryOptions struct {
	Store          database.Store
	ProjectID      uuid.UUID
	OrganizationID uuid.UUID
	ChatID         uuid.UUID
	OwnerID        uuid.UUID
}

// ProjectMemoryIndexEntry is a compact memory entry for prompt injection.
type ProjectMemoryIndexEntry struct {
	Name        string
	Type        database.ChatProjectMemoryType
	Description string
}

type normalizedProjectMemory struct {
	Name        string
	Type        database.ChatProjectMemoryType
	Description string
	Body        string
}

// ValidateProjectMemoryName validates a stable project-memory identifier.
func ValidateProjectMemoryName(name string) error {
	if !projectMemoryNameRE.MatchString(name) {
		return xerrors.Errorf("name must match %q", projectMemoryNameRE.String())
	}
	return nil
}

// NormalizeProjectMemoryText sanitizes durable memory text and removes tags
// that could forge prompt-index boundaries.
func NormalizeProjectMemoryText(text string) string {
	text = codersdk.SanitizePromptText(text)
	text = strings.ReplaceAll(text, "<project-memory>", "")
	text = strings.ReplaceAll(text, "</project-memory>", "")
	return strings.TrimSpace(text)
}

func normalizeProjectMemoryInput(name string, memoryType database.ChatProjectMemoryType, description, body string) (normalizedProjectMemory, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if err := ValidateProjectMemoryName(name); err != nil {
		return normalizedProjectMemory{}, err
	}
	if !memoryType.Valid() {
		return normalizedProjectMemory{}, xerrors.Errorf("type must be one of %v", database.AllChatProjectMemoryTypeValues())
	}
	description = NormalizeProjectMemoryText(description)
	body = NormalizeProjectMemoryText(body)
	if description == "" {
		return normalizedProjectMemory{}, xerrors.New("description is required")
	}
	if utf8.RuneCountInString(description) > MaxProjectMemoryDescriptionChars {
		return normalizedProjectMemory{}, xerrors.Errorf("description must be at most %d characters", MaxProjectMemoryDescriptionChars)
	}
	if body == "" {
		return normalizedProjectMemory{}, xerrors.New("body is required")
	}
	if len(body) > MaxProjectMemoryBodyBytes {
		return normalizedProjectMemory{}, xerrors.Errorf("body must be at most %d bytes", MaxProjectMemoryBodyBytes)
	}
	return normalizedProjectMemory{Name: name, Type: memoryType, Description: description, Body: body}, nil
}

// ProjectMemoryGuidance tells the model what belongs in project memory. It
// is shared by the prompt index and the background extractor so both
// writers apply the same bar.
const ProjectMemoryGuidance = "Project memory is durable context shared by every chat in this project. " +
	"Types: user (who the people on this project are: role, expertise, working preferences), " +
	"feedback (corrections you received and approaches that were explicitly confirmed), " +
	"project (ongoing work, deadlines, and decisions that cannot be derived from the code or git history), " +
	"reference (where to find information outside the project, such as an issue tracker or dashboard).\n" +
	"Save a memory as soon as durable information surfaces, without waiting to be asked. " +
	"Do not save anything derivable from the codebase (architecture, file paths, debugging fixes), " +
	"anything already stated in instructions, or temporary in-progress state. " +
	"Never save that something is unknown or undecided. " +
	"When a question might be answered by a memory in the index, call read_project_memory before answering or asking the user. " +
	"Memories may be stale or wrong; verify before relying on one and update or delete it when it no longer holds."

// FormatProjectMemoryIndex renders the compact project-memory index for the
// system prompt. The guidance renders even when no memories exist so the
// model knows when to save its first one.
func FormatProjectMemoryIndex(entries []ProjectMemoryIndexEntry) string {
	var b strings.Builder
	_, _ = b.WriteString("<project-memory>\n")
	_, _ = b.WriteString(ProjectMemoryGuidance)
	_, _ = b.WriteString("\n\n")
	if len(entries) == 0 {
		_, _ = b.WriteString("No memories saved yet.\n")
		_, _ = b.WriteString("</project-memory>")
		return b.String()
	}

	shown := 0
	truncationReserve := len(fmt.Sprintf("%d more memories not shown.\n", len(entries))) + len("</project-memory>")
	for _, entry := range entries {
		if shown >= MaxProjectMemoryIndexLines {
			break
		}
		line := fmt.Sprintf("- %s [%s]: %s", entry.Name, entry.Type, entry.Description)
		if b.Len()+len(line)+1+truncationReserve > MaxProjectMemoryIndexBytes {
			break
		}
		_, _ = b.WriteString(line)
		_ = b.WriteByte('\n')
		shown++
	}
	if omitted := len(entries) - shown; omitted > 0 {
		_, _ = b.WriteString(fmt.Sprintf("%d more memories not shown.\n", omitted))
	}
	_, _ = b.WriteString("</project-memory>")
	return b.String()
}

type readProjectMemoryArgs struct {
	Name string `json:"name" description:"The name of the project memory to read."`
}

type saveProjectMemoryArgs struct {
	Name        string                         `json:"name" description:"Stable lowercase name for the memory."`
	Type        database.ChatProjectMemoryType `json:"type" description:"Memory type: user, feedback, project, or reference."`
	Description string                         `json:"description" description:"One-line summary shown in the memory index."`
	Body        string                         `json:"body" description:"Full durable markdown memory body."`
}

type deleteProjectMemoryArgs struct {
	Name string `json:"name" description:"The name of the project memory to delete."`
}

// ReadProjectMemory returns a tool that reads a project's full memory body.
func ReadProjectMemory(options ProjectMemoryOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(ReadProjectMemoryToolName, "Read a full project memory by name.", func(ctx context.Context, args readProjectMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		if options.Store == nil {
			return fantasy.NewTextErrorResponse("project memory store is not configured"), nil
		}
		name := strings.ToLower(strings.TrimSpace(args.Name))
		if err := ValidateProjectMemoryName(name); err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		memory, err := options.Store.GetChatProjectMemoryByName(ctx, database.GetChatProjectMemoryByNameParams{ProjectID: options.ProjectID, Name: name})
		if err != nil {
			return fantasy.NewTextErrorResponse("project memory was not found"), nil
		}
		return toolResponse(map[string]any{"name": memory.ChatProjectMemory.Name, "type": memory.ChatProjectMemory.Type, "description": memory.ChatProjectMemory.Description, "body": memory.ChatProjectMemory.Body, "updated_at": memory.ChatProjectMemory.UpdatedAt, "created_by": memory.CreatedByUsername}), nil
	})
}

// SaveProjectMemory returns a tool that upserts a durable project memory.
func SaveProjectMemory(options ProjectMemoryOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(SaveProjectMemoryToolName, "Save or update a durable project memory by name.", func(ctx context.Context, args saveProjectMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		if options.Store == nil {
			return fantasy.NewTextErrorResponse("project memory store is not configured"), nil
		}
		normalized, err := normalizeProjectMemoryInput(args.Name, args.Type, args.Description, args.Body)
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		_, getErr := options.Store.GetChatProjectMemoryByName(ctx, database.GetChatProjectMemoryByNameParams{ProjectID: options.ProjectID, Name: normalized.Name})
		if getErr != nil {
			count, countErr := options.Store.CountChatProjectMemoriesByProjectID(ctx, options.ProjectID)
			if countErr != nil {
				return fantasy.NewTextErrorResponse("failed to count project memories"), nil
			}
			if count >= MaxProjectMemories {
				return fantasy.NewTextErrorResponse("project memory limit reached; merge or delete existing memories first"), nil
			}
		}
		memory, err := options.Store.UpsertChatProjectMemoryByName(ctx, database.UpsertChatProjectMemoryByNameParams{
			ProjectID: options.ProjectID, OrganizationID: options.OrganizationID,
			Type: normalized.Type, Name: normalized.Name, Description: normalized.Description, Body: normalized.Body,
			SourceChatID: uuid.NullUUID{UUID: options.ChatID, Valid: options.ChatID != uuid.Nil}, CreatedBy: options.OwnerID,
		})
		if err != nil {
			return fantasy.NewTextErrorResponse("failed to save project memory"), nil
		}
		return toolResponse(map[string]any{"id": memory.ID, "name": memory.Name, "updated_at": memory.UpdatedAt}), nil
	})
}

// DeleteProjectMemory returns a tool that deletes a project memory by name.
func DeleteProjectMemory(options ProjectMemoryOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(DeleteProjectMemoryToolName, "Delete a project memory by name.", func(ctx context.Context, args deleteProjectMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		if options.Store == nil {
			return fantasy.NewTextErrorResponse("project memory store is not configured"), nil
		}
		name := strings.ToLower(strings.TrimSpace(args.Name))
		if err := ValidateProjectMemoryName(name); err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		if err := options.Store.DeleteChatProjectMemoryByName(ctx, database.DeleteChatProjectMemoryByNameParams{ProjectID: options.ProjectID, Name: name}); err != nil {
			return fantasy.NewTextErrorResponse("project memory was not found"), nil
		}
		return toolResponse(map[string]any{"deleted": name}), nil
	})
}
