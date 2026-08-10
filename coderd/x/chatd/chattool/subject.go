package chattool

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
)

// PlatformSubject builds the liveness-checked authorization subject and acting
// principal for an in-process chat platform tool call. Chats with an AI agent
// actor use the owner's live roles narrowed to the chat agent profile. Chats
// without an actor use the live owner's full subject for compatibility.
func PlatformSubject(ctx context.Context, db database.Store, ownerID uuid.UUID) (rbac.Subject, uuid.UUID, error) {
	agentActor, ok := aiagentidentity.ActorFromContext(ctx)
	if !ok {
		//nolint:gocritic // Detached chat workers must verify owner liveness.
		owner, err := db.GetUserByID(dbauthz.AsSystemRestricted(ctx), ownerID)
		if err != nil {
			return rbac.Subject{}, uuid.Nil, xerrors.Errorf("load chat owner: %w", err)
		}
		if owner.Deleted || owner.Status != database.UserStatusActive {
			return rbac.Subject{}, uuid.Nil, xerrors.Errorf("chat owner %s is not active", ownerID)
		}
		actor, userStatus, err := httpmw.UserRBACSubject(ctx, db, ownerID, rbac.ScopeAll)
		if err != nil {
			return rbac.Subject{}, uuid.Nil, xerrors.Errorf("load user authorization: %w", err)
		}
		if userStatus != database.UserStatusActive {
			return rbac.Subject{}, uuid.Nil, xerrors.Errorf("chat owner %s authorization is not active", ownerID)
		}
		return actor, ownerID, nil
	}

	identity, err := aiagentidentity.Resolve(ctx, db, agentActor.AgentUserID)
	if err != nil {
		return rbac.Subject{}, uuid.Nil, xerrors.Errorf("resolve chat AI agent identity: %w", err)
	}
	if identity.Actor != agentActor {
		return rbac.Subject{}, uuid.Nil, xerrors.New("chat AI agent actor does not match its authoritative identity")
	}
	if identity.AgentUser.Deleted || identity.AgentUser.Status != database.UserStatusActive {
		return rbac.Subject{}, uuid.Nil, xerrors.Errorf("chat AI agent user %s is not active", identity.AgentUser.ID)
	}
	if identity.OwnerUser.ID != ownerID || identity.OwnerUser.Kind != database.UserKindHuman ||
		identity.OwnerUser.Deleted || identity.OwnerUser.Status != database.UserStatusActive {
		return rbac.Subject{}, uuid.Nil, xerrors.Errorf("chat AI agent owner %s is not an active human user", ownerID)
	}

	profile := aiagentidentity.ChatAgentProfile(identity.Actor.OriginID)
	scopeSet := database.APIKeyScopeSet{
		Scopes:    profile.Scopes,
		AllowList: profile.AllowList,
	}
	actor, userStatus, err := httpmw.UserRBACSubject(ctx, db, ownerID, scopeSet)
	if err != nil {
		return rbac.Subject{}, uuid.Nil, xerrors.Errorf("load AI agent authorization: %w", err)
	}
	if userStatus != database.UserStatusActive {
		return rbac.Subject{}, uuid.Nil, xerrors.Errorf("chat AI agent owner %s authorization is not active", ownerID)
	}
	actor.Type = rbac.SubjectTypeAIAgent
	actor.FriendlyName = identity.AgentUser.Username
	return actor, identity.AgentUser.ID, nil
}

// asOwner adds the liveness-checked chat platform subject to the database
// authorization context.
func asOwner(ctx context.Context, db database.Store, ownerID uuid.UUID) (context.Context, error) {
	actor, _, err := PlatformSubject(ctx, db, ownerID)
	if err != nil {
		return ctx, err
	}
	return dbauthz.As(ctx, actor), nil
}
