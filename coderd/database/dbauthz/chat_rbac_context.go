package dbauthz

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
)

// CacheableChatRBAC returns the chat's RBAC object without ACLs. A chat's
// ID, owner, and organization never change, so this object is safe to
// cache for the chat's lifetime; ACLs are omitted because sharing changes
// at runtime.
func CacheableChatRBAC(chat database.Chat) rbac.Object {
	return CacheableChatRBACFor(chat.ID, chat.OwnerID, chat.OrganizationID)
}

// CacheableChatRBACFor is CacheableChatRBAC for callers holding only the
// identifying fields, such as a lean chat read.
func CacheableChatRBACFor(chatID, ownerID, organizationID uuid.UUID) rbac.Object {
	return rbac.ResourceChat.
		WithID(chatID).
		WithOwner(ownerID.String()).
		InOrg(organizationID)
}

func isChatRBACObjectEmpty(rbacObj rbac.Object) bool {
	return rbacObj.ID == "" || rbacObj.ID == uuid.Nil.String() ||
		rbacObj.Owner == "" || rbacObj.Owner == uuid.Nil.String() ||
		rbacObj.OrgID == "" || rbacObj.OrgID == uuid.Nil.String()
}

type chatRBACContextKey struct{}

// WithChatRBAC attaches a chat RBAC object to the context so chat-scoped
// dbauthz methods can authorize without re-reading the chat row. The
// object must carry the chat ID, owner, and organization and must not
// carry ACLs: sharing changes at runtime, so callers that need ACL-based
// access fall back to the fetch path.
func WithChatRBAC(ctx context.Context, rbacObj rbac.Object) (context.Context, error) {
	if rbacObj.Type != rbac.ResourceChat.Type {
		return ctx, xerrors.New("RBAC object must be of type Chat")
	}
	if isChatRBACObjectEmpty(rbacObj) {
		return ctx, xerrors.Errorf("cannot attach empty chat RBAC object to context: %+v", rbacObj)
	}
	if len(rbacObj.ACLGroupList) != 0 || len(rbacObj.ACLUserList) != 0 {
		return ctx, xerrors.New("ACL fields for chat RBAC object must be nullified; they can change at runtime and must not be cached")
	}
	return context.WithValue(ctx, chatRBACContextKey{}, rbacObj), nil
}

// ChatRBACFromContext returns the chat RBAC object cached on the context, if any.
func ChatRBACFromContext(ctx context.Context) (rbac.Object, bool) {
	obj, ok := ctx.Value(chatRBACContextKey{}).(rbac.Object)
	return obj, ok
}
