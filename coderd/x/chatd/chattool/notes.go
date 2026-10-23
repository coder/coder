package chattool

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

const (
	// NotesToolName is the chat-scoped notes tool. Notes stay when a new
	// context window starts.
	NotesToolName = "notes"
	// MaxNoteBytes is the size cap of one note. The chat_notes table
	// enforces the same cap.
	MaxNoteBytes = 16384
	// MaxNotesPerChat is the number of notes one chat can keep.
	MaxNotesPerChat = 20
	// CheckpointNotePath is the note the context budget nudges ask the
	// agent to keep current.
	CheckpointNotePath = "checkpoint.md"
)

var notePathRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,127}$`)

const notesDescription = "Notes for this chat. They stay when a new context window starts; earlier messages do not. " +
	"Keep a concise checkpoint in checkpoint.md: the goal, decisions, progress, learnings, next steps, " +
	"and the [id: N] of every user request you still work on. Update it while you work. " +
	"Other chats cannot read these notes. " +
	"Actions: list, read (path), write (path, content; replaces the note), append (path, content; adds a line). " +
	"Paths use a-z, 0-9, '.', '_', '/' and '-' (128 characters at most). " +
	"Each note holds 16 KiB at most, and a chat keeps 20 notes at most. There is no delete: write empty content instead."

// NotesStore reads and writes the notes of one chat.
type NotesStore struct {
	db     database.Store
	chatID uuid.UUID
}

// NewNotesStore returns the notes store of chatID.
func NewNotesStore(db database.Store, chatID uuid.UUID) NotesStore {
	return NotesStore{db: db, chatID: chatID}
}

// ChatID returns the chat that owns the notes.
func (s NotesStore) ChatID() uuid.UUID { return s.chatID }

// List returns the path, size and update time of every note.
func (s NotesStore) List(ctx context.Context) ([]database.ListChatNotesRow, error) {
	return s.db.ListChatNotes(ctx, s.chatID)
}

// Read returns one note. It returns sql.ErrNoRows when the note does not
// exist.
func (s NotesStore) Read(ctx context.Context, path string) (database.ChatNote, error) {
	return s.db.GetChatNote(ctx, database.GetChatNoteParams{ChatID: s.chatID, Path: path})
}

// ChangedSince reports whether a note was written after since.
func (s NotesStore) ChangedSince(ctx context.Context, since time.Time) (bool, error) {
	return s.db.GetChatNotesChangedSince(ctx, database.GetChatNotesChangedSinceParams{ChatID: s.chatID, Since: since})
}

// NotesArgs are the arguments of the notes tool.
type NotesArgs struct {
	Action  string `json:"action" description:"list, read, write or append"`
	Path    string `json:"path,omitempty" description:"The note path, for example checkpoint.md"`
	Content string `json:"content,omitempty" description:"write replaces the note with this content; append adds it as a new line"`
}

// Notes returns the chat-scoped notes tool.
func Notes(store NotesStore) fantasy.AgentTool {
	return fantasy.NewAgentTool(NotesToolName, notesDescription,
		func(ctx context.Context, args NotesArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			switch args.Action {
			case "list":
				rows, err := store.List(ctx)
				if err != nil {
					return fantasy.NewTextErrorResponse("failed to list notes"), nil
				}
				notes := make([]map[string]any, 0, len(rows))
				for _, row := range rows {
					notes = append(notes, map[string]any{"path": row.Path, "bytes": row.Bytes, "updated_at": row.UpdatedAt})
				}
				return toolResponse(map[string]any{"notes": notes}), nil
			case "read":
				if !notePathRE.MatchString(args.Path) {
					return fantasy.NewTextErrorResponse(badNotePathMessage(args.Path)), nil
				}
				note, err := store.Read(ctx, args.Path)
				switch {
				case errors.Is(err, sql.ErrNoRows):
					return fantasy.NewTextErrorResponse(fmt.Sprintf("note %q does not exist", args.Path)), nil
				case err != nil:
					return fantasy.NewTextErrorResponse("failed to read note"), nil
				}
				return toolResponse(map[string]any{"path": note.Path, "content": note.Content, "updated_at": note.UpdatedAt}), nil
			case "write", "append":
				return writeNote(ctx, store, args), nil
			default:
				return fantasy.NewTextErrorResponse(fmt.Sprintf("unknown action %q: use list, read, write or append", args.Action)), nil
			}
		})
}

func badNotePathMessage(path string) string {
	return fmt.Sprintf("invalid note path %q: use 1 to 128 characters from a-z, 0-9, '.', '_', '/' and '-', starting with a letter or digit", path)
}

func writeNote(ctx context.Context, store NotesStore, args NotesArgs) fantasy.ToolResponse {
	if !notePathRE.MatchString(args.Path) {
		return fantasy.NewTextErrorResponse(badNotePathMessage(args.Path))
	}
	existing, err := store.Read(ctx, args.Path)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fantasy.NewTextErrorResponse("failed to read note")
	}
	size := len(args.Content)
	if args.Action == "append" && exists && existing.Content != "" {
		size = len(existing.Content) + 1 + len(args.Content)
	}
	if size > MaxNoteBytes {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("note %q would be %d bytes: a note holds %d bytes at most", args.Path, size, MaxNoteBytes))
	}
	if !exists {
		count, err := store.db.CountChatNotes(ctx, store.chatID)
		if err != nil {
			return fantasy.NewTextErrorResponse("failed to count notes")
		}
		if count >= MaxNotesPerChat {
			return fantasy.NewTextErrorResponse(fmt.Sprintf("this chat has %d notes, the limit: write to an existing note instead", count))
		}
	}
	var note database.ChatNote
	if args.Action == "append" {
		note, err = store.db.AppendChatNote(ctx, database.AppendChatNoteParams{ChatID: store.chatID, Path: args.Path, Content: args.Content})
	} else {
		note, err = store.db.UpsertChatNote(ctx, database.UpsertChatNoteParams{ChatID: store.chatID, Path: args.Path, Content: args.Content})
	}
	if err != nil {
		if database.IsCheckViolation(err) {
			return fantasy.NewTextErrorResponse(fmt.Sprintf("note %q is over %d bytes", args.Path, MaxNoteBytes))
		}
		return fantasy.NewTextErrorResponse("failed to write note")
	}
	return toolResponse(map[string]any{"path": note.Path, "bytes": len(note.Content), "updated_at": note.UpdatedAt})
}
