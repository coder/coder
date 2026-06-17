package chatd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
)

func contentHash(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

// pinnedInstruction builds a pinned instruction-file row with an explicit
// content hash so diff tests can control add/modify/remove independently of
// the body bytes.
func pinnedInstruction(t *testing.T, source, content string, hash []byte) database.ChatContextResource {
	t.Helper()
	return database.ChatContextResource{
		Source:      source,
		BodyKind:    database.WorkspaceAgentContextBodyKindInstructionFile,
		Body:        mustMarshalContextBody(t, &agentproto.InstructionFileBody{Content: []byte(content)}),
		ContentHash: hash,
		SizeBytes:   int64(len(content)),
		Status:      database.WorkspaceAgentContextResourceStatusOk,
	}
}

func snapshotInstruction(t *testing.T, source, content string, hash []byte) database.WorkspaceAgentContextResource {
	t.Helper()
	return database.WorkspaceAgentContextResource{
		Source:      source,
		BodyKind:    database.WorkspaceAgentContextBodyKindInstructionFile,
		Body:        mustMarshalContextBody(t, &agentproto.InstructionFileBody{Content: []byte(content)}),
		ContentHash: hash,
		SizeBytes:   int64(len(content)),
		Status:      database.WorkspaceAgentContextResourceStatusOk,
	}
}

func pinnedSkill(t *testing.T, source, name, description string, hash []byte) database.ChatContextResource {
	t.Helper()
	return database.ChatContextResource{
		Source:      source,
		BodyKind:    database.WorkspaceAgentContextBodyKindSkill,
		Body:        mustMarshalContextBody(t, &agentproto.SkillMetaBody{Meta: []byte("# " + name), Name: name, Description: description}),
		ContentHash: hash,
		Status:      database.WorkspaceAgentContextResourceStatusOk,
	}
}

func snapshotSkill(t *testing.T, source, name, description string, hash []byte) database.WorkspaceAgentContextResource {
	t.Helper()
	return database.WorkspaceAgentContextResource{
		Source:      source,
		BodyKind:    database.WorkspaceAgentContextBodyKindSkill,
		Body:        mustMarshalContextBody(t, &agentproto.SkillMetaBody{Meta: []byte("# " + name), Name: name, Description: description}),
		ContentHash: hash,
		Status:      database.WorkspaceAgentContextResourceStatusOk,
	}
}

func TestDiffContextResources(t *testing.T) {
	t.Parallel()

	t.Run("AddedModifiedRemovedUnchanged", func(t *testing.T) {
		t.Parallel()

		pinned := []database.ChatContextResource{
			pinnedInstruction(t, "/keep.md", "same", contentHash("same")),
			pinnedInstruction(t, "/edit.md", "old body", contentHash("old body")),
			pinnedInstruction(t, "/gone.md", "removed body", contentHash("removed body")),
		}
		snapshot := []database.WorkspaceAgentContextResource{
			snapshotInstruction(t, "/keep.md", "same", contentHash("same")),
			snapshotInstruction(t, "/edit.md", "new body", contentHash("new body")),
			snapshotInstruction(t, "/new.md", "added body", contentHash("added body")),
		}

		changes := diffContextResources(pinned, snapshot)
		// Ordered by source: /edit.md, /gone.md, /new.md. /keep.md is omitted.
		require.Len(t, changes, 3)

		require.Equal(t, codersdk.ChatContextResourceChange{
			Source:     "/edit.md",
			Kind:       codersdk.ChatContextResourceKindInstructionFile,
			Status:     codersdk.ChatContextResourceChangeStatusModified,
			OldContent: "old body",
			NewContent: "new body",
		}, changes[0])

		require.Equal(t, codersdk.ChatContextResourceChange{
			Source:     "/gone.md",
			Kind:       codersdk.ChatContextResourceKindInstructionFile,
			Status:     codersdk.ChatContextResourceChangeStatusRemoved,
			OldContent: "removed body",
		}, changes[1])

		require.Equal(t, codersdk.ChatContextResourceChange{
			Source:     "/new.md",
			Kind:       codersdk.ChatContextResourceKindInstructionFile,
			Status:     codersdk.ChatContextResourceChangeStatusAdded,
			NewContent: "added body",
		}, changes[2])
	})

	t.Run("SkillIdentitySides", func(t *testing.T) {
		t.Parallel()

		pinned := []database.ChatContextResource{
			pinnedSkill(t, "/skills/edit", "edit-old", "old desc", contentHash("edit-old")),
			pinnedSkill(t, "/skills/gone", "gone", "leaving", contentHash("gone")),
		}
		snapshot := []database.WorkspaceAgentContextResource{
			snapshotSkill(t, "/skills/edit", "edit-new", "new desc", contentHash("edit-new")),
			snapshotSkill(t, "/skills/add", "added", "joining", contentHash("added")),
		}

		changes := diffContextResources(pinned, snapshot)
		require.Len(t, changes, 3)

		// Modified skill reports the snapshot identity (what a refresh adopts).
		require.Equal(t, codersdk.ChatContextResourceChange{
			Source:           "/skills/add",
			Kind:             codersdk.ChatContextResourceKindSkill,
			Status:           codersdk.ChatContextResourceChangeStatusAdded,
			SkillName:        "added",
			SkillDescription: "joining",
		}, changes[0])
		require.Equal(t, codersdk.ChatContextResourceChange{
			Source:           "/skills/edit",
			Kind:             codersdk.ChatContextResourceKindSkill,
			Status:           codersdk.ChatContextResourceChangeStatusModified,
			SkillName:        "edit-new",
			SkillDescription: "new desc",
		}, changes[1])
		// Removed skill reports the pinned identity (only side that exists).
		require.Equal(t, codersdk.ChatContextResourceChange{
			Source:           "/skills/gone",
			Kind:             codersdk.ChatContextResourceKindSkill,
			Status:           codersdk.ChatContextResourceChangeStatusRemoved,
			SkillName:        "gone",
			SkillDescription: "leaving",
		}, changes[2])
	})

	t.Run("SkipsNonPromptKinds", func(t *testing.T) {
		t.Parallel()

		pinned := []database.ChatContextResource{
			{Source: ".mcp.json", BodyKind: database.WorkspaceAgentContextBodyKindMcpConfig, ContentHash: contentHash("old")},
		}
		snapshot := []database.WorkspaceAgentContextResource{
			{Source: ".mcp.json", BodyKind: database.WorkspaceAgentContextBodyKindMcpConfig, ContentHash: contentHash("new")},
		}
		require.Empty(t, diffContextResources(pinned, snapshot))
	})

	t.Run("SanitizesAndCapsContent", func(t *testing.T) {
		t.Parallel()

		// CRLF is normalized by SanitizePromptText, and content beyond the cap
		// is truncated.
		large := strings.Repeat("a", maxContextChangeContentBytes+500)
		pinned := []database.ChatContextResource{
			pinnedInstruction(t, "/a.md", "line1\r\nline2", contentHash("old")),
			pinnedInstruction(t, "/big.md", large, contentHash("big-old")),
		}
		snapshot := []database.WorkspaceAgentContextResource{
			snapshotInstruction(t, "/a.md", "line1\r\nchanged", contentHash("new")),
			snapshotInstruction(t, "/big.md", large+"-changed", contentHash("big-new")),
		}

		changes := diffContextResources(pinned, snapshot)
		require.Len(t, changes, 2)
		require.Equal(t, "line1\nline2", changes[0].OldContent)
		require.Equal(t, "line1\nchanged", changes[0].NewContent)
		require.Len(t, changes[1].OldContent, maxContextChangeContentBytes)
		require.Len(t, changes[1].NewContent, maxContextChangeContentBytes)
	})
}

func TestTruncateUTF8(t *testing.T) {
	t.Parallel()

	require.Equal(t, "abc", truncateUTF8("abc", 10))
	require.Equal(t, "abc", truncateUTF8("abc", 3))
	require.Equal(t, "ab", truncateUTF8("abc", 2))
	require.Equal(t, "", truncateUTF8("abc", 0))

	// "é" is two bytes (0xC3 0xA9); a cap landing inside it backs off so the
	// rune is not split.
	require.Equal(t, "a", truncateUTF8("aé", 2))
	require.Equal(t, "aé", truncateUTF8("aé", 3))
}

func TestContextDetail(t *testing.T) {
	t.Parallel()

	t.Run("NotDirtySkipsSnapshotRead", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return([]database.ChatContextResource{
				instructionResource(t, "/home/coder/AGENTS.md", "be helpful", database.WorkspaceAgentContextResourceStatusOk),
			}, nil)
		// No ListWorkspaceAgentContextResources call is configured: a clean
		// chat must not read the snapshot.
		server := newPinServer(t, db)

		resources, changes, err := server.ContextDetail(context.Background(), database.Chat{ID: chatID})
		require.NoError(t, err)
		require.Len(t, resources, 1)
		require.Nil(t, changes)
	})

	t.Run("DirtyComputesChanges", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		agentID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return([]database.ChatContextResource{
				pinnedInstruction(t, "/home/coder/AGENTS.md", "old", contentHash("old")),
			}, nil)
		db.EXPECT().ListWorkspaceAgentContextResources(gomock.Any(), agentID).
			Return([]database.WorkspaceAgentContextResource{
				snapshotInstruction(t, "/home/coder/AGENTS.md", "new", contentHash("new")),
			}, nil)
		server := newPinServer(t, db)

		chat := database.Chat{
			ID:                chatID,
			AgentID:           uuid.NullUUID{UUID: agentID, Valid: true},
			ContextDirtySince: sql.NullTime{Time: dbtime.Now(), Valid: true},
		}
		resources, changes, err := server.ContextDetail(context.Background(), chat)
		require.NoError(t, err)
		require.Len(t, resources, 1)
		require.Len(t, changes, 1)
		require.Equal(t, codersdk.ChatContextResourceChangeStatusModified, changes[0].Status)
		require.Equal(t, "old", changes[0].OldContent)
		require.Equal(t, "new", changes[0].NewContent)
	})

	t.Run("DirtyWithoutAgentSkipsSnapshot", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return([]database.ChatContextResource{}, nil)
		server := newPinServer(t, db)

		chat := database.Chat{
			ID:                chatID,
			ContextDirtySince: sql.NullTime{Time: dbtime.Now(), Valid: true},
		}
		resources, changes, err := server.ContextDetail(context.Background(), chat)
		require.NoError(t, err)
		require.Empty(t, resources)
		require.Nil(t, changes)
	})

	t.Run("PinnedListError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return(nil, xerrors.New("boom"))
		server := newPinServer(t, db)

		_, _, err := server.ContextDetail(context.Background(), database.Chat{ID: chatID})
		require.Error(t, err)
	})

	t.Run("SnapshotListError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		agentID := uuid.New()
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chatID).
			Return([]database.ChatContextResource{}, nil)
		db.EXPECT().ListWorkspaceAgentContextResources(gomock.Any(), agentID).
			Return(nil, xerrors.New("boom"))
		server := newPinServer(t, db)

		chat := database.Chat{
			ID:                chatID,
			AgentID:           uuid.NullUUID{UUID: agentID, Valid: true},
			ContextDirtySince: sql.NullTime{Time: dbtime.Now(), Valid: true},
		}
		_, _, err := server.ContextDetail(context.Background(), chat)
		require.Error(t, err)
	})
}
