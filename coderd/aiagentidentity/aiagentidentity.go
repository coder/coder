// Package aiagentidentity creates and resolves delegated AI agent identities.
package aiagentidentity

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

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
	OriginType     entity.CreationSiteType
	OriginID       uuid.UUID
}

// AIAgentActor is the request attribution identity for an AI agent.
type AIAgentActor struct {
	AgentUserID uuid.UUID
	OwnerUserID uuid.UUID
	OriginType  entity.CreationSiteType
	OriginID    uuid.UUID
}

// ResolvedIdentity contains the authoritative AI agent state and the users
// rows still consulted alongside it.
//
// Ledger is the authority on the agent: who owns it, what it was created in,
// and whether it is still live. AgentUser is the mirrored users row and is kept
// only for the name and the status checks that have not yet moved.
type ResolvedIdentity struct {
	Actor     AIAgentActor
	Ledger    database.AIAgentLedger
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
func Create(ctx context.Context, db database.Store, params CreateParams) (database.User, error) {
	if params.OwnerID == uuid.Nil {
		return database.User{}, xerrors.New("owner ID must be non-nil")
	}
	if params.OrganizationID == uuid.Nil {
		return database.User{}, xerrors.New("organization ID must be non-nil")
	}
	if params.OriginID == uuid.Nil {
		return database.User{}, xerrors.New("origin ID must be non-nil")
	}
	if !params.OriginType.Valid() {
		return database.User{}, xerrors.Errorf("invalid AI agent origin type %q", params.OriginType)
	}

	var createdUser database.User
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

		createdUser, err = mirror(systemCtx, tx, created.ID, params)
		return err
	}, nil)
	if err != nil {
		return database.User{}, err
	}
	return createdUser, nil
}

// creationSite reads the params as the pair the model calls a creation site.
func creationSite(params CreateParams) entity.CreationSite {
	return entity.CreationSite{Type: params.OriginType, ID: params.OriginID}
}

// mirror writes the users row for an AI agent the ledger has already named.
//
// It is all that is left of the mirror. The ai_agents row went when nothing
// read it; this row survives because six places still route on
// users.kind = 'ai_agent' and because the username is read from it.
func mirror(ctx context.Context, tx database.Store, id uuid.UUID, params CreateParams) (database.User, error) {
	// One derivation, so the mirrored username and the name the authorizer
	// carries as a friendly name cannot drift apart. It exceeds the 32
	// character limit codersdk.NameValid states, which nothing enforces here:
	// the column is plain text, and an AI agent never logs in or is renamed,
	// which are the only paths that validate.
	name := entity.DisplayName(creationSite(params).Type, id)

	now := dbtime.Now()
	createdUser, err := tx.InsertAIAgentUser(ctx, database.InsertAIAgentUserParams{
		ID:        id,
		Username:  name,
		CreatedAt: now,
	})
	if err != nil {
		return database.User{}, xerrors.Errorf("insert AI agent user: %w", err)
	}

	rewrite2026augustlog.AIAgentCreated(ctx, rewrite2026augustlog.F{
		"ai_agent_user_id": id,
		"owner_user_id":    params.OwnerID,
		"origin_type":      params.OriginType,
		"origin_id":        params.OriginID,
	})
	return createdUser, nil
}

// DropKey deletes one of an AI agent's mirrored keys without recording an
// ending, named by the token name its profile carries.
//
// **It records nothing, which is why it is not called Revoke.** Two callers
// want exactly that: a retirement, where the credential has already lapsed and
// only the mirror is left, and a rotation, where the credential is superseded
// rather than finished and both halves belong in one entry once WP13's atomic
// group exists.
//
// **An ending calls RevokeKey or DischargeKey instead.** Ending one is currently a delete from api_keys and nothing else;
// gathering the callers here is what lets that become a posting to the
// credential ledger in one place rather than four. Nothing else should delete
// a key an AI agent holds.
//
// **A credential that is not there is not an error.** More than one ending can
// apply to the same credential, so this is defined to be idempotent rather
// than to report that something got there first.
//
// The context is used as given. Callers needing system access escalate before
// calling, which keeps that decision where the caller can see it.
func DropKey(ctx context.Context, db database.Store, agentID uuid.UUID, tokenName string) error {
	key, err := db.GetAPIKeyByName(ctx, database.GetAPIKeyByNameParams{
		HolderID:  database.HolderID(agentID),
		TokenName: tokenName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("get AI agent API key by name: %w", err)
	}
	if err := db.DeleteAPIKeyByID(ctx, key.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("delete AI agent API key: %w", err)
	}
	return nil
}

// RevokeKey ends one of an AI agent's credentials because a party withdrew it,
// posting the revocation and deleting the mirrored key.
//
// **Commanded, so it carries the party.** Its sites are a workspace stopping
// and a workspace being deleted, where the credential ends because somebody
// decided the workspace should stop or go, and the decision is captured in the
// build. The material ending has not happened yet at that point, which is why
// this is a revocation following a decision rather than a discharge following
// an ending.
//
// store may be a transaction handle.
func RevokeKey(ctx context.Context, db database.Store, agentID uuid.UUID, tokenName string, actor entity.Ref) error {
	return endKey(ctx, db, agentID, tokenName, func(credentialID uuid.UUID) error {
		return entity.RevokeCredential(ctx, db, credentialID, actor)
	})
}

// DischargeKey ends one of an AI agent's credentials because the thing it was
// accessory to has ended, posting the discharge and deleting the mirrored key.
//
// **This is the endings' door and DropKey is not.** A retirement also uses
// DropKey, to delete the mirror after the credential has already lapsed, so a
// posting inside that function would give a retirement two endings for one
// credential. Which ending a call is making is said by which function it calls,
// not by a parameter.
//
// `entailedBy` says what ended, and today is always an annotation: neither a
// sandbox nor a workspace keeps a journal to reference. See "The reference has
// two forms, and one of them is words" in
// poc_audit/implementation_patterns.md.
//
// store may be a transaction handle, so the ending can commit with the ending
// that caused it.
func DischargeKey(ctx context.Context, db database.Store, agentID uuid.UUID, tokenName string, entailedBy entity.EntailedBy) error {
	return endKey(ctx, db, agentID, tokenName, func(credentialID uuid.UUID) error {
		return entity.DischargeCredential(ctx, db, credentialID, entailedBy, time.Time{})
	})
}

// endKey posts an ending against the credential a mirrored key stands for, and
// deletes the key. What differs between the endings is the transition, so that
// is what the caller supplies and everything else is written once.
//
// **A credential that is not there is not an error.** More than one ending can
// apply to the same credential, so this is idempotent rather than reporting
// that something got there first.
func endKey(ctx context.Context, db database.Store, agentID uuid.UUID, tokenName string, post func(credentialID uuid.UUID) error) error {
	key, err := db.GetAPIKeyByName(ctx, database.GetAPIKeyByNameParams{
		HolderID:  database.HolderID(agentID),
		TokenName: tokenName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("get AI agent API key by name: %w", err)
	}

	// The ledger is the index: it knows which credential this key mirrors. A
	// key without one cannot arise, every AI agent key being minted through
	// the ledger, so its absence is an error rather than a case to handle.
	mirrored, err := db.GetCredentialAPIKeyByKeyID(ctx, key.ID)
	if err != nil {
		return xerrors.Errorf("read the credential behind AI agent key %q: %w", tokenName, err)
	}
	if err := post(mirrored.ID); err != nil {
		return xerrors.Errorf("end AI agent credential %q: %w", tokenName, err)
	}
	if err := db.DeleteAPIKeyByID(ctx, key.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("delete AI agent API key: %w", err)
	}
	return nil
}

// DropAllKeys deletes every mirrored key an AI agent holds, recording nothing.
// Its caller is the orphan sweep, which retires the agent first, so the
// credentials have already lapsed and only the mirrors remain.
func DropAllKeys(ctx context.Context, db database.Store, agentID uuid.UUID) error {
	if err := db.DeleteAPIKeysByHolderID(ctx, database.HolderID(agentID)); err != nil {
		return xerrors.Errorf("delete AI agent API keys: %w", err)
	}
	return nil
}

// MintKey issues an AI agent's api_key credential through the credential
// ledger, which mirrors it into api_keys in the same transaction.
//
// **The ledger is where the credential is recorded and api_keys is the copy.**
// Authentication still reads api_keys, so nothing about verifying this key
// changes; what changes is that the credential an agent actually presents now
// exists in the record. See "Issuance can move to the journal before
// authentication moves" in poc_audit/rewrite_rbac.md.
//
// **The actor is the agent's owner**, which is what creating the agent records
// and for the same reason: the owner is the party the credential lets act.
// Nobody commands this minting as such, a build or a sandbox creation reaching
// it as a consequence, so naming the owner is the closest true attribution
// available until an actor kind exists for the party that does.
//
// The profile is validated first and is stricter than the general api key
// generator on every axis: it requires scopes and an allow list rather than
// defaulting them, rejects the broad scopes, and refuses a wildcard.
func MintKey(ctx context.Context, db database.Store, agentUserID uuid.UUID, profile Profile) (database.APIKey, string, error) {
	if agentUserID == uuid.Nil {
		return database.APIKey{}, "", xerrors.New("AI agent user ID must be non-nil")
	}

	profile, err := validateProfile(profile)
	if err != nil {
		return database.APIKey{}, "", err
	}
	resolved, err := Resolve(ctx, db, agentUserID)
	if err != nil {
		return database.APIKey{}, "", xerrors.Errorf("resolve AI agent before minting key: %w", err)
	}

	// Key minting is internal and limited by the validated profile.
	systemCtx := dbauthz.AsSystemRestricted(ctx) //nolint:gocritic

	issued, err := entity.IssueCredential(systemCtx, db, entity.IssueCredentialParams{
		Holder: entity.Ref{Type: entity.TypeAIAgent, ID: agentUserID},
		Type:   entity.CredentialTypeAPIKey,
		Actor:  entity.Ref{Type: entity.TypeUser, ID: resolved.Ledger.OwnerID},
		APIKey: &entity.APIKeyCredential{
			TokenName:      profile.TokenName,
			Scopes:         profile.Scopes,
			AllowList:      profile.AllowList,
			MirrorLifetime: keyLifetime,
		},
	})
	if err != nil {
		return database.APIKey{}, "", xerrors.Errorf("issue AI agent credential: %w", err)
	}

	// The mirrored row, read back rather than assembled here, so that what
	// callers receive is what the table holds. The ledger is the index: it
	// knows which key id the credential was mirrored under, where a lookup by
	// token name would fail for a profile that has no name.
	mirrored, err := db.GetCredentialAPIKeyByID(systemCtx, issued.ID)
	if err != nil {
		return database.APIKey{}, "", xerrors.Errorf("read the credential's key id: %w", err)
	}
	key, err := db.GetAPIKeyByID(systemCtx, mirrored.KeyID)
	if err != nil {
		return database.APIKey{}, "", xerrors.Errorf("read back the mirrored AI agent API key: %w", err)
	}
	return key, issued.Authenticator, nil
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

	// The ledger, not ai_agents. The two agree, one being written from the
	// other, and reading the authority is what lets the mirror go.
	ledger, err := db.GetAIAgentLedgerRowByID(systemCtx, agentUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ResolvedIdentity{}, ErrNotAIAgent
		}
		return ResolvedIdentity{}, xerrors.Errorf("get AI agent ledger row: %w", err)
	}
	// Anything but active is unusable, which today means retired. Dormant is in
	// the state set and unreachable, and a dormant agent is not a live one
	// either, so refusing everything but active stays right when it arrives.
	if ledger.State != entity.AIAgentStateActive {
		return ResolvedIdentity{}, ErrAIAgentDeleted
	}

	owner, err := db.GetUserByID(systemCtx, ledger.OwnerID)
	if err != nil {
		return ResolvedIdentity{}, xerrors.Errorf("get AI agent owner: %w", err)
	}

	actor := AIAgentActor{
		AgentUserID: ledger.ID,
		OwnerUserID: ledger.OwnerID,
		OriginType:  entity.CreationSiteType(ledger.CreationSiteType),
		OriginID:    ledger.CreationSiteID,
	}
	return ResolvedIdentity{
		Actor:     actor,
		Ledger:    ledger,
		AgentUser: agentUser,
		OwnerUser: owner,
	}, nil
}
