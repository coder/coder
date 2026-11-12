package chatautomation

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// Creator creates chat automations. The API and the agent tool share it, so
// that every rule exists once.
type Creator struct {
	db         database.Store
	authorizer rbac.Authorizer
	limits     Limits
}

// NewCreator returns a Creator. db must be wrapped by dbauthz.
func NewCreator(db database.Store, authorizer rbac.Authorizer, limits Limits) *Creator {
	return &Creator{db: db, authorizer: authorizer, limits: limits}
}

// CreateParams is one create. The owner is always the actor: no caller sets
// another owner.
type CreateParams struct {
	OrganizationID uuid.UUID
	// CreatedByChatID is the chat of the agent tool that creates the
	// automation. The API leaves it unset.
	CreatedByChatID uuid.NullUUID
	Request         codersdk.CreateChatAutomationRequest
	// Now is the time that the first run of a schedule follows.
	Now time.Time
}

// CreateResult is the created automation.
type CreateResult struct {
	// Automation holds the webhook secret in plain text. Do not return it.
	Automation database.ChatAutomation
	// GeneratedSecret is the secret that Coder created for a bearer or github
	// webhook. It is empty for any other automation. The caller can return it
	// once, because no later call returns it.
	GeneratedSecret string
}

// createObject is the RBAC object of a new automation. dbauthz checks the same
// object in InsertChatAutomation.
func createObject(organizationID, ownerID uuid.UUID) rbac.Object {
	return rbac.ResourceChatAutomation.InOrg(organizationID).WithOwner(ownerID.String())
}

// Create creates an automation that actor owns. It returns ErrCreateForbidden
// when actor cannot create it, *InvalidRequestError for a request that breaks a
// field rule or names a chat, model or project that the actor cannot use,
// *LimitError at the per-owner limit, and ErrOwnerUnavailable when the owner is
// deleted or a system user. No error text holds the prompt or a secret.
func (c *Creator) Create(ctx context.Context, actor rbac.Subject, p CreateParams) (CreateResult, error) {
	if !dbauthz.IsWrapped(c.db) {
		return CreateResult{}, xerrors.New("chat automation store is not wrapped by dbauthz")
	}
	owner, err := uuid.Parse(actor.ID)
	if err != nil {
		return CreateResult{}, xerrors.Errorf("parse actor ID: %w", err)
	}
	// The same subject acts in the context and in the authorizer, so a check
	// here and a check in dbauthz cannot differ.
	actorCtx := dbauthz.As(ctx, actor)
	if err := c.authorizer.Authorize(ctx, actor, policy.ActionCreate, createObject(p.OrganizationID, owner)); err != nil {
		return CreateResult{}, ErrCreateForbidden
	}

	n, rejected, err := validateRequest(p.Request, c.limits)
	if err != nil {
		return CreateResult{}, err
	}
	if len(rejected) == 0 {
		rejected, err = c.checkReferences(ctx, actor, p)
		if err != nil {
			return CreateResult{}, err
		}
	}
	if len(rejected) > 0 {
		return CreateResult{}, &InvalidRequestError{Validations: rejected}
	}

	params, err := insertParams(owner, p, n)
	if err != nil {
		return CreateResult{}, err
	}
	var result CreateResult
	err = c.db.InTx(func(tx database.Store) error {
		// One lock per owner keeps two creates from both passing the limit.
		if err := tx.AcquireLock(actorCtx, database.GenLockID("chat-automations-owner:"+owner.String())); err != nil {
			return xerrors.Errorf("lock owner: %w", err)
		}
		// The counter subject can read the owner's automations in every
		// organization and nothing else, so it never reaches the insert.
		count, err := tx.CountChatAutomationsByOwnerID(dbauthz.AsChatAutomationCounter(ctx, owner), owner)
		if err != nil {
			return xerrors.Errorf("count chat automations: %w", err)
		}
		if count >= c.limits.MaxPerOwner {
			return &LimitError{Max: c.limits.MaxPerOwner}
		}

		insert := params
		var generated string
		if n.verifier != nil {
			secret, ok, err := n.verifier.GenerateSecret()
			if err != nil {
				return xerrors.Errorf("generate webhook secret: %w", err)
			}
			if ok {
				generated = secret
				insert.WebhookSecret = sql.NullString{String: secret, Valid: true}
			}
		}
		// A missing row means that the owner guard of the insert refused the
		// owner. The references were read before, so nothing else gives it.
		row, err := tx.InsertChatAutomation(actorCtx, insert)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrOwnerUnavailable
		case dbauthz.IsNotAuthorizedError(err):
			return ErrCreateForbidden
		case err != nil:
			return xerrors.Errorf("insert chat automation: %w", err)
		}
		result = CreateResult{Automation: row, GeneratedSecret: generated}
		return nil
	}, nil)
	if err != nil {
		return CreateResult{}, err
	}
	return result, nil
}

// checkReferences reads the chat, model configuration and project that the
// request names, as the actor. A reference that does not exist and one that the
// actor cannot read give the same detail.
func (c *Creator) checkReferences(ctx context.Context, actor rbac.Subject, p CreateParams) ([]codersdk.ValidationError, error) {
	actorCtx := dbauthz.As(ctx, actor)
	req := p.Request
	var v violations

	if req.TargetChatID != nil {
		chat, err := c.db.GetChatByID(actorCtx, *req.TargetChatID)
		switch {
		case httpapi.Is404Error(err):
			v.add("target_chat_id", "Chat not found.")
		case err != nil:
			return nil, xerrors.Errorf("get target chat: %w", err)
		case chat.OrganizationID != p.OrganizationID:
			v.add("target_chat_id", "Chat is not in this organization.")
		case chat.ParentChatID.Valid:
			v.add("target_chat_id", "Chat is not a root chat.")
		case c.authorizer.Authorize(ctx, actor, policy.ActionUpdate, chat.RBACObject()) != nil:
			v.add("target_chat_id", "You cannot send messages to this chat.")
		}
	}
	if req.NewChatModelConfigID != nil {
		config, err := c.db.GetChatModelConfigByID(actorCtx, *req.NewChatModelConfigID)
		switch {
		case httpapi.Is404Error(err):
			v.add("new_chat_model_config_id", "Model configuration not found.")
		case err != nil:
			return nil, xerrors.Errorf("get model configuration: %w", err)
		case config.OrganizationID != p.OrganizationID:
			v.add("new_chat_model_config_id", "Model configuration is not in this organization.")
		}
	}
	if req.ProjectID != nil {
		project, err := c.db.GetChatProjectByID(actorCtx, *req.ProjectID)
		switch {
		case httpapi.Is404Error(err):
			v.add("project_id", "Project not found.")
		case err != nil:
			return nil, xerrors.Errorf("get project: %w", err)
		case project.OrganizationID != p.OrganizationID:
			v.add("project_id", "Project is not in this organization.")
		}
	}
	return v, nil
}

// insertParams builds the insert of a validated request. A bearer or github
// secret is added later, inside the transaction.
func insertParams(owner uuid.UUID, p CreateParams, n normalized) (database.InsertChatAutomationParams, error) {
	req := p.Request
	var (
		auth     database.NullChatAutomationWebhookAuth
		secret   sql.NullString
		cronSpec sql.NullString
		zone     sql.NullString
		nextRun  sql.NullTime
	)
	if n.verifier != nil {
		auth = database.NullChatAutomationWebhookAuth{
			ChatAutomationWebhookAuth: database.ChatAutomationWebhookAuth(req.WebhookAuth),
			Valid:                     true,
		}
		secret = sql.NullString{String: n.secret, Valid: n.secret != ""}
	}
	if n.schedule != nil {
		// Store the parts. The string form of a schedule is not a stable format.
		cronSpec = sql.NullString{String: n.schedule.Cron(), Valid: true}
		zone = sql.NullString{String: req.ScheduleTimeZone, Valid: true}
		if n.enabled {
			next := n.schedule.Next(p.Now)
			if p.Now.IsZero() || next.IsZero() {
				return database.InsertChatAutomationParams{}, xerrors.New("no next run time for the schedule")
			}
			nextRun = sql.NullTime{Time: next, Valid: true}
		}
	}
	return database.InsertChatAutomationParams{
		ID:                   uuid.NullUUID{}, // The database generates it.
		OrganizationID:       p.OrganizationID,
		OwnerID:              owner,
		Name:                 n.name,
		CreatedByChatID:      p.CreatedByChatID,
		Kind:                 database.ChatAutomationKind(req.Kind),
		Enabled:              n.enabled,
		Prompt:               req.Prompt,
		TargetMode:           database.ChatAutomationTargetMode(req.TargetMode),
		TargetChatID:         nullUUID(req.TargetChatID),
		WhenBusy:             n.whenBusy,
		NewChatModelConfigID: nullUUID(req.NewChatModelConfigID),
		ReasoningEffort:      n.reasoningEffort,
		ProjectID:            nullUUID(req.ProjectID),
		WebhookAuth:          auth,
		WebhookSecret:        secret,
		WebhookSecretKeyID:   sql.NullString{}, // The dbcrypt layer sets it.
		ScheduleCron:         cronSpec,
		ScheduleTimeZone:     zone,
		ScheduleNextRunAt:    nextRun,
	}, nil
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}
