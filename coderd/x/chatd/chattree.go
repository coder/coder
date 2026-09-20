package chatd

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

// ChatTreeRootTitle is the title given to a lazily created tree root.
const ChatTreeRootTitle = "Root"

var (
	// ErrChatTreeRootUnavailable indicates the tree root could not be created
	// because no model config could be resolved for it.
	ErrChatTreeRootUnavailable = xerrors.New("chat tree root unavailable")
	// ErrChatTreeDepthExceeded indicates the parent already sits at the
	// maximum tree depth.
	ErrChatTreeDepthExceeded = xerrors.New("chat tree depth limit exceeded")
	// ErrChatTreeParentIsSubagent indicates the requested parent is a
	// subagent chat, which cannot have named children.
	ErrChatTreeParentIsSubagent = xerrors.New("subagent chats cannot have child chats")
	// ErrChatTreeParentArchived indicates the requested parent is archived.
	ErrChatTreeParentArchived = xerrors.New("parent chat is archived")
	// ErrChatTreeParentMismatch indicates the requested parent does not
	// exist, is not readable, or belongs to another owner or organization.
	// The cases share one error so callers cannot probe for chat IDs.
	ErrChatTreeParentMismatch = xerrors.New("parent chat not found")
	// ErrChatTreeParentUnanchored indicates the requested parent is owned
	// by the caller but its ancestor chain does not end at a tree root.
	ErrChatTreeParentUnanchored = xerrors.New("parent chat is not attached to a chat tree")
)

// EnsureChatTreeRootOptions configures [Server.EnsureChatTreeRoot].
type EnsureChatTreeRootOptions struct {
	OwnerID        uuid.UUID
	OrganizationID uuid.UUID
	// ResolveModelConfigID is called only when no root exists yet. It
	// returns the model config the root is created with.
	ResolveModelConfigID func(ctx context.Context) (uuid.UUID, error)
}

// EnsureChatTreeRoot returns the owner's tree root in the organization,
// creating it when absent, and reparents every parentless user chat of
// that owner in that organization under it. The second return value
// reports whether the root was created by this call.
//
// Adoption changes parent_chat_id only: updated_at, snapshot versions,
// pin state, and archive state are untouched and no watch event is
// published. The whole slow path runs in one transaction under a per
// (owner, organization) advisory lock, so concurrent callers observe a
// single root; the partial unique index on kind = 'root' rejects a
// second root regardless. The root starts with the same system messages
// as any new chat and no user message, so it stays in waiting and is
// never acquired by a worker.
//
// Every query runs as the acting user. Creating the root requires
// chat create permission for the owner in the organization; adopting
// requires chat update permission on the owner's chats.
func (p *Server) EnsureChatTreeRoot(ctx context.Context, opts EnsureChatTreeRootOptions) (database.Chat, bool, error) {
	if opts.OwnerID == uuid.Nil {
		return database.Chat{}, false, xerrors.New("owner_id is required")
	}
	if opts.OrganizationID == uuid.Nil {
		return database.Chat{}, false, xerrors.New("organization_id is required")
	}
	if opts.ResolveModelConfigID == nil {
		return database.Chat{}, false, xerrors.New("model config resolver is required")
	}

	stateParams := database.GetChatTreeRootStateByOwnerAndOrganizationParams{
		OwnerID:        opts.OwnerID,
		OrganizationID: opts.OrganizationID,
	}
	state, err := p.db.GetChatTreeRootStateByOwnerAndOrganization(ctx, stateParams)
	if err != nil {
		return database.Chat{}, false, xerrors.Errorf("get chat tree root state: %w", err)
	}
	if state.RootChatID != uuid.Nil && state.AdoptableCount == 0 {
		root, err := p.db.GetChatByID(ctx, state.RootChatID)
		if err != nil {
			return database.Chat{}, false, xerrors.Errorf("get chat tree root: %w", err)
		}
		return root, false, nil
	}

	// The model config and the system messages are resolved before the
	// transaction opens so no other connection is used while the advisory
	// lock is held. A concurrent caller that wins the lock makes this
	// work unused.
	var rootInput chatstate.CreateIdleChatInput
	if state.RootChatID == uuid.Nil {
		modelConfigID, err := opts.ResolveModelConfigID(ctx)
		if err != nil {
			return database.Chat{}, false, errors.Join(ErrChatTreeRootUnavailable, xerrors.Errorf("resolve chat tree root model: %w", err))
		}
		if modelConfigID == uuid.Nil {
			return database.Chat{}, false, ErrChatTreeRootUnavailable
		}
		systemMessages, err := initialSystemMessages(p.resolveDeploymentSystemPrompt(ctx), "", false, modelConfigID)
		if err != nil {
			return database.Chat{}, false, err
		}
		rootInput = chatstate.CreateIdleChatInput{
			OrganizationID:    opts.OrganizationID,
			OwnerID:           opts.OwnerID,
			Kind:              database.ChatKindRoot,
			LastModelConfigID: modelConfigID,
			Title:             ChatTreeRootTitle,
			ClientType:        database.ChatClientTypeApi,
			InitialMessages:   systemMessages,
		}
	}

	var (
		root    database.Chat
		created bool
	)
	err = p.db.InTx(func(tx database.Store) error {
		lockID := database.GenLockID("chat_tree_root:" + opts.OwnerID.String() + ":" + opts.OrganizationID.String())
		if err := tx.AcquireLock(ctx, lockID); err != nil {
			return xerrors.Errorf("acquire chat tree lock: %w", err)
		}
		state, err := tx.GetChatTreeRootStateByOwnerAndOrganization(ctx, stateParams)
		if err != nil {
			return xerrors.Errorf("get chat tree root state: %w", err)
		}
		switch {
		case state.RootChatID != uuid.Nil:
			root, err = tx.GetChatByID(ctx, state.RootChatID)
			if err != nil {
				return xerrors.Errorf("get chat tree root: %w", err)
			}
		case rootInput.Kind == "":
			// The pre-check saw a root that is gone now; the caller
			// retries and resolves a model config on the next call.
			return ErrChatTreeRootUnavailable
		default:
			root, err = chatstate.CreateIdleChat(ctx, tx, rootInput)
			if err != nil {
				return xerrors.Errorf("create chat tree root: %w", err)
			}
			created = true
		}
		if state.AdoptableCount > 0 || created {
			_, err = tx.AdoptParentlessChatsIntoTreeRoot(ctx, database.AdoptParentlessChatsIntoTreeRootParams{
				RootChatID:     root.ID,
				OwnerID:        opts.OwnerID,
				OrganizationID: opts.OrganizationID,
			})
			if err != nil {
				return xerrors.Errorf("adopt parentless chats: %w", err)
			}
		}
		return nil
	}, &database.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return database.Chat{}, false, err
	}
	return root, created, nil
}

// ChatTreeParent returns the parent of chat. The boolean is false when
// the chat has no parent.
func (p *Server) ChatTreeParent(ctx context.Context, chat database.Chat) (database.Chat, bool, error) {
	if !chat.ParentChatID.Valid {
		return database.Chat{}, false, nil
	}
	parent, err := p.db.GetChatByID(ctx, chat.ParentChatID.UUID)
	if err != nil {
		return database.Chat{}, false, xerrors.Errorf("get parent chat: %w", err)
	}
	return parent, true, nil
}

// validateChatTreeParent locks the parent row and checks that a named
// child may be created under it for ownerID in orgID. The parent must be
// a root or chat kind row owned by the same user in the same organization,
// unarchived, and above the depth limit. It runs inside the creating
// transaction so the parent cannot be archived concurrently.
func validateChatTreeParent(
	ctx context.Context,
	tx database.Store,
	ownerID uuid.UUID,
	orgID uuid.UUID,
	parentID uuid.UUID,
) error {
	// The row lock is taken through a read-authorized fetch, so a chat
	// readable through an ACL grant is locked until this transaction ends
	// even when the owner check below rejects it.
	parent, err := tx.GetChatByIDForUpdate(ctx, parentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
			return ErrChatTreeParentMismatch
		}
		return xerrors.Errorf("lock parent chat: %w", err)
	}
	if parent.OwnerID != ownerID || parent.OrganizationID != orgID {
		return ErrChatTreeParentMismatch
	}
	if parent.Kind == database.ChatKindSubagent {
		return ErrChatTreeParentIsSubagent
	}
	if parent.Archived {
		return ErrChatTreeParentArchived
	}
	depth, err := tx.GetChatTreeDepthByID(ctx, parentID)
	if err != nil {
		return xerrors.Errorf("get parent chat depth: %w", err)
	}
	// Depth 0 means the parent's ancestor chain does not end at a root.
	if depth == 0 {
		return ErrChatTreeParentUnanchored
	}
	if int(depth) >= codersdk.ChatTreeMaxDepth {
		return ErrChatTreeDepthExceeded
	}
	return nil
}
