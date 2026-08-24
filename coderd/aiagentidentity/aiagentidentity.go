// Package aiagentidentity creates and resolves delegated AI agent identities.
package aiagentidentity

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/coderd/rewrite2026augustlog"
)

var (
	// ErrNotAIAgent means the user is not backed by a live AI agent identity.
	ErrNotAIAgent = xerrors.New("AI agent identity not found")
	// ErrAIAgentDeleted means the identity has been revoked by its origin.
	ErrAIAgentDeleted = xerrors.New("AI agent identity is deleted")
)

// CreateParams describes an AI agent identity and its human owner.
type CreateParams struct {
	OwnerID        uuid.UUID
	OrganizationID uuid.UUID
	OriginType     database.AIAgentOrigin
	OriginID       uuid.UUID
}

// AIAgentActor is the request attribution identity for an AI agent.
type AIAgentActor struct {
	AgentUserID uuid.UUID
	OwnerUserID uuid.UUID
	OriginType  database.AIAgentOrigin
	OriginID    uuid.UUID
}

// ResolvedIdentity contains the authoritative AI agent metadata and users.
type ResolvedIdentity struct {
	Actor     AIAgentActor
	AIAgent   database.AIAgent
	AgentUser database.User
	OwnerUser database.User
}

type actorContextKey struct{}

// WithActor stores AI agent request attribution in a context.
func WithActor(ctx context.Context, actor AIAgentActor) context.Context {
	return context.WithValue(ctx, actorContextKey{}, actor)
}

// ActorFromContext returns AI agent request attribution from a context.
func ActorFromContext(ctx context.Context) (AIAgentActor, bool) {
	actor, ok := ctx.Value(actorContextKey{}).(AIAgentActor)
	return actor, ok
}

// Create creates an AI agent and mirrors it into the tables the identity code
// reads.
//
// **The ledger mints and these two rows follow.** entity.CreateAIAgent names
// the agent; the users row and the ai_agents row are written under the
// identifier it returns, so there is one identifier for one agent rather than
// two spaces to reconcile. Every column referring to an AI agent therefore
// resolves against the ledger, which is what lets those references carry a
// foreign key to it.
//
// The mirror is an interim and is one way. Nothing here reports divergence,
// and nothing should come to rely on these rows being the authority. Later work
// deletes the mirror rather than untangling it.
func Create(ctx context.Context, db database.Store, params CreateParams) (database.User, database.AIAgent, error) {
	if params.OwnerID == uuid.Nil {
		return database.User{}, database.AIAgent{}, xerrors.New("owner ID must be non-nil")
	}
	if params.OrganizationID == uuid.Nil {
		return database.User{}, database.AIAgent{}, xerrors.New("organization ID must be non-nil")
	}
	if params.OriginID == uuid.Nil {
		return database.User{}, database.AIAgent{}, xerrors.New("origin ID must be non-nil")
	}
	if !params.OriginType.Valid() {
		return database.User{}, database.AIAgent{}, xerrors.Errorf("invalid AI agent origin type %q", params.OriginType)
	}

	var (
		createdUser  database.User
		createdAgent database.AIAgent
	)
	// Identity creation is an internal operation with explicit owner checks.
	systemCtx := dbauthz.AsSystemRestricted(ctx) //nolint:gocritic

	err := db.InTx(func(tx database.Store) error {
		owner, err := tx.GetUserByID(systemCtx, params.OwnerID)
		if err != nil {
			return xerrors.Errorf("get AI agent owner: %w", err)
		}
		if owner.Kind != database.UserKindHuman {
			return xerrors.Errorf("AI agent owner %s is not a human user", params.OwnerID)
		}

		members, err := tx.OrganizationMembers(systemCtx, database.OrganizationMembersParams{
			OrganizationID: params.OrganizationID,
			UserID:         params.OwnerID,
			IncludeSystem:  true,
			GithubUserID:   0,
		})
		if err != nil {
			return xerrors.Errorf("get AI agent owner organization membership: %w", err)
		}
		if len(members) != 1 {
			return xerrors.Errorf("AI agent owner %s is not a member of organization %s", params.OwnerID, params.OrganizationID)
		}

		created, err := entity.CreateAIAgent(systemCtx, tx, entity.CreateAIAgentParams{
			Owner:        entity.Ref{Type: entity.TypeUser, ID: params.OwnerID},
			CreationSite: creationSite(params),
		})
		if err != nil {
			return xerrors.Errorf("create AI agent: %w", err)
		}

		createdUser, createdAgent, err = mirror(systemCtx, tx, created.ID, params)
		return err
	}, nil)
	if err != nil {
		return database.User{}, database.AIAgent{}, err
	}
	return createdUser, createdAgent, nil
}

// creationSite restates the identity code's origin pair in the model's terms.
// The two name the same thing; only the word differs, and this is the one place
// that has to know both.
func creationSite(params CreateParams) entity.CreationSite {
	site := entity.CreationSite{ID: params.OriginID}
	switch params.OriginType {
	case database.AIAgentOriginChat:
		site.Type = entity.CreationSiteTypeChat
	case database.AIAgentOriginWorkspace:
		site.Type = entity.CreationSiteTypeWorkspace
	}
	return site
}

// mirror writes the users row and the ai_agents row for an AI agent the ledger
// has already named.
func mirror(ctx context.Context, tx database.Store, id uuid.UUID, params CreateParams) (database.User, database.AIAgent, error) {
	name, err := username(params.OriginType, id)
	if err != nil {
		return database.User{}, database.AIAgent{}, err
	}

	now := dbtime.Now()
	createdUser, err := tx.InsertAIAgentUser(ctx, database.InsertAIAgentUserParams{
		ID:        id,
		Username:  name,
		CreatedAt: now,
	})
	if err != nil {
		return database.User{}, database.AIAgent{}, xerrors.Errorf("insert AI agent user: %w", err)
	}

	createdAgent, err := tx.InsertAIAgent(ctx, database.InsertAIAgentParams{
		UserID:      id,
		OwnerUserID: params.OwnerID,
		OriginType:  params.OriginType,
		OriginID:    params.OriginID,
		CreatedAt:   now,
	})
	if err != nil {
		return database.User{}, database.AIAgent{}, xerrors.Errorf("insert AI agent metadata: %w", err)
	}
	rewrite2026augustlog.AIAgentCreated(ctx, rewrite2026augustlog.F{
		"ai_agent_user_id": createdAgent.UserID,
		"owner_user_id":    createdAgent.OwnerUserID,
		"origin_type":      createdAgent.OriginType,
		"origin_id":        createdAgent.OriginID,
	})
	return createdUser, createdAgent, nil
}

// MintKey creates a token-login API key for an AI agent identity.
func MintKey(ctx context.Context, db database.Store, agentUserID uuid.UUID, profile Profile) (database.APIKey, string, error) {
	if agentUserID == uuid.Nil {
		return database.APIKey{}, "", xerrors.New("AI agent user ID must be non-nil")
	}

	profile, err := validateProfile(profile)
	if err != nil {
		return database.APIKey{}, "", err
	}
	if _, err := Resolve(ctx, db, agentUserID); err != nil {
		return database.APIKey{}, "", xerrors.Errorf("resolve AI agent before minting key: %w", err)
	}

	params, token, err := apikey.Generate(apikey.CreateParams{
		UserID:          agentUserID,
		LoginType:       database.LoginTypeToken,
		DefaultLifetime: keyLifetime,
		Scopes:          profile.Scopes,
		AllowList:       profile.AllowList,
		TokenName:       profile.TokenName,
	})
	if err != nil {
		return database.APIKey{}, "", xerrors.Errorf("generate AI agent API key: %w", err)
	}

	// Key minting is internal and limited by the validated profile.
	systemCtx := dbauthz.AsSystemRestricted(ctx) //nolint:gocritic
	key, err := db.InsertAPIKey(systemCtx, params)
	if err != nil {
		return database.APIKey{}, "", xerrors.Errorf("insert AI agent API key: %w", err)
	}
	return key, token, nil
}

// Resolve loads authoritative AI agent metadata and its human owner.
func Resolve(ctx context.Context, db database.Store, agentUserID uuid.UUID) (ResolvedIdentity, error) {
	if agentUserID == uuid.Nil {
		return ResolvedIdentity{}, xerrors.New("AI agent user ID must be non-nil")
	}

	// Authentication must resolve identity metadata before an actor exists.
	systemCtx := dbauthz.AsSystemRestricted(ctx) //nolint:gocritic
	agentUser, err := db.GetUserByID(systemCtx, agentUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ResolvedIdentity{}, ErrNotAIAgent
		}
		return ResolvedIdentity{}, xerrors.Errorf("get AI agent user: %w", err)
	}
	if agentUser.Kind != database.UserKindAIAgent {
		return ResolvedIdentity{}, ErrNotAIAgent
	}

	agent, err := db.GetAIAgentByUserID(systemCtx, agentUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ResolvedIdentity{}, ErrNotAIAgent
		}
		return ResolvedIdentity{}, xerrors.Errorf("get AI agent metadata: %w", err)
	}
	if agent.Deleted {
		return ResolvedIdentity{}, ErrAIAgentDeleted
	}

	owner, err := db.GetUserByID(systemCtx, agent.OwnerUserID)
	if err != nil {
		return ResolvedIdentity{}, xerrors.Errorf("get AI agent owner: %w", err)
	}

	actor := AIAgentActor{
		AgentUserID: agent.UserID,
		OwnerUserID: agent.OwnerUserID,
		OriginType:  agent.OriginType,
		OriginID:    agent.OriginID,
	}
	return ResolvedIdentity{
		Actor:     actor,
		AIAgent:   agent,
		AgentUser: agentUser,
		OwnerUser: owner,
	}, nil
}

// username derives the mirrored users row's name from the identifier the ledger
// minted, so that a name is a rendering of an identity rather than a value with
// a life of its own. Deriving it also removes the collision retry the previous
// random name needed, which could not have survived joining the caller's
// transaction: a unique violation aborts a transaction, so a retry inside one
// cannot succeed.
//
// Half the identifier, the whole of it not fitting the 32 character limit on a
// username. Collision is not defended against. Two agents would have to share
// 64 bits for it to arise.
func username(origin database.AIAgentOrigin, id uuid.UUID) (string, error) {
	suffix := strings.ReplaceAll(id.String(), "-", "")[:16]
	switch origin {
	case database.AIAgentOriginChat:
		return "ai-chat-" + suffix, nil
	case database.AIAgentOriginWorkspace:
		return "ai-ws-" + suffix, nil
	default:
		return "", xerrors.Errorf("unsupported AI agent origin %q", origin)
	}
}
