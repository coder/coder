// Package aiagentidentity creates and resolves delegated AI agent identities.
package aiagentidentity

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rewrite2026augustlog"
	"github.com/coder/coder/v2/cryptorand"
)

const usernameAttempts = 5

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

// Create inserts an AI agent user and its authoritative metadata atomically.
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
	userID := uuid.New()
	// Identity creation is an internal operation with explicit owner checks.
	systemCtx := dbauthz.AsSystemRestricted(ctx) //nolint:gocritic

	for attempt := range usernameAttempts {
		username, err := username(params.OriginType)
		if err != nil {
			return database.User{}, database.AIAgent{}, err
		}

		err = db.InTx(func(tx database.Store) error {
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

			now := dbtime.Now()
			createdUser, err = tx.InsertAIAgentUser(systemCtx, database.InsertAIAgentUserParams{
				ID:        userID,
				Username:  username,
				CreatedAt: now,
			})
			if err != nil {
				return xerrors.Errorf("insert AI agent user: %w", err)
			}

			createdAgent, err = tx.InsertAIAgent(systemCtx, database.InsertAIAgentParams{
				UserID:      userID,
				OwnerUserID: params.OwnerID,
				OriginType:  params.OriginType,
				OriginID:    params.OriginID,
				CreatedAt:   now,
			})
			if err != nil {
				return xerrors.Errorf("insert AI agent metadata: %w", err)
			}
			rewrite2026augustlog.AIAgentCreated(ctx, rewrite2026augustlog.F{
				"ai_agent_user_id": createdAgent.UserID,
				"owner_user_id":    createdAgent.OwnerUserID,
				"origin_type":      createdAgent.OriginType,
				"origin_id":        createdAgent.OriginID,
			})
			return nil
		}, nil)
		if err == nil {
			return createdUser, createdAgent, nil
		}
		if !database.IsUniqueViolation(err, database.UniqueIndexUsersUsername, database.UniqueUsersUsernameLowerIndex) {
			return database.User{}, database.AIAgent{}, err
		}
		if attempt == usernameAttempts-1 {
			return database.User{}, database.AIAgent{}, xerrors.Errorf("generate unique AI agent username after %d attempts: %w", usernameAttempts, err)
		}
	}

	panic("unreachable")
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

func username(origin database.AIAgentOrigin) (string, error) {
	suffix, err := cryptorand.HexString(8)
	if err != nil {
		return "", xerrors.Errorf("generate AI agent username suffix: %w", err)
	}

	switch origin {
	case database.AIAgentOriginChat:
		return "ai-chat-" + suffix, nil
	case database.AIAgentOriginWorkspace:
		return "ai-ws-" + suffix, nil
	default:
		return "", xerrors.Errorf("unsupported AI agent origin %q", origin)
	}
}
