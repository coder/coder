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

// The triggers of chat_automations raise these constraints when a chat or
// project of the row is gone or in another organization. checkReferences has
// read them in this organization, so a trigger fires only when a concurrent
// delete removes the chat or project between that read and the insert. A
// delete that is still open at the trigger makes the foreign key check wait
// and then fail, and Create maps that to the same answer.
const (
	chatOrganizationTrigger    database.CheckConstraint = "chat_automations_chat_organization"
	projectOrganizationTrigger database.CheckConstraint = "chat_automations_project_organization"
)

// createObject is the RBAC object of a new automation. dbauthz checks the same
// object in InsertChatAutomation.
func createObject(organizationID, ownerID uuid.UUID) rbac.Object {
	return rbac.ResourceChatAutomation.InOrg(organizationID).WithOwner(ownerID.String())
}

// Create creates an automation that actor owns. It returns ErrCreateForbidden
// when actor cannot create it, *InvalidRequestError for a request that breaks a
// field rule or names a chat, model or project that the actor cannot use,
// *LimitError at the per-owner limit, ErrOwnerUnavailable when the owner is
// deleted or unreadable, and ErrReferenceUnavailable when a concurrent
// delete removed a chat, model configuration or project after the check. No
// error text holds the prompt or a secret.
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
		rejected, err = c.checkReferences(ctx, actor, owner, p)
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
		row, err := tx.InsertChatAutomation(actorCtx, insert)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return missingRow(actorCtx, tx, owner)
		case dbauthz.IsNotAuthorizedError(err):
			return ErrCreateForbidden
		case database.IsCheckViolation(err, chatOrganizationTrigger, projectOrganizationTrigger),
			database.IsForeignKeyViolation(err, database.ForeignKeyChatAutomationsTargetChatID,
				database.ForeignKeyChatAutomationsCreatedByChatID, database.ForeignKeyChatAutomationsProjectID):
			return ErrReferenceUnavailable
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

// missingRow says why the insert returned no row. The owner guard of the insert
// refuses a deleted owner. The insert also reads the target chat, creating
// chat, model configuration and project again, so when the owner is fine, one
// of them was deleted since checkReferences read it.
func missingRow(ctx context.Context, tx database.Store, owner uuid.UUID) error {
	user, err := tx.GetUserByID(ctx, owner)
	switch {
	case httpapi.Is404Error(err), dbauthz.IsNotAuthorizedError(err):
		return ErrOwnerUnavailable
	case err != nil:
		return xerrors.Errorf("get owner: %w", err)
	case user.Deleted:
		return ErrOwnerUnavailable
	}
	return ErrReferenceUnavailable
}

// checkReferences reads the chat, model configuration and project that the
// request names, as the actor. A reference that does not exist and one that the
// actor cannot read give the same detail.
func (c *Creator) checkReferences(ctx context.Context, actor rbac.Subject, owner uuid.UUID, p CreateParams) ([]codersdk.ValidationError, error) {
	actorCtx := dbauthz.As(ctx, actor)
	req := p.Request
	var v violations

	// Only the agent tool sets a creating chat. The insert would report a
	// missing one as a missing owner, and the database would refuse one from
	// another organization.
	if p.CreatedByChatID.Valid {
		chat, err := c.db.GetChatByID(actorCtx, p.CreatedByChatID.UUID)
		switch {
		case dbauthz.IsNotAuthorizedError(err):
			return nil, ErrCreateForbidden
		case err != nil:
			return nil, xerrors.Errorf("get creating chat: %w", err)
		case chat.OrganizationID != p.OrganizationID:
			return nil, xerrors.New("creating chat is not in this organization")
		}
	}

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
		// Only the chat owner may send a message to a chat, as in
		// checkSendChatMessage (coderd/exp_chats.go): organization admins pass
		// the update check above, but chat processing forwards the owner's
		// credentials (OIDC tokens, provider API keys) to external services.
		// A run into a chat that the automation's owner does not own would
		// act with the credentials of another user.
		case chat.OwnerID != owner:
			v.add("target_chat_id", "Only the owner of a chat can use it as the target of an automation.")
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
			// cron.Standard refuses a schedule that never runs, so Next of
			// a real time is never zero. A zero Now is a caller bug.
			if p.Now.IsZero() {
				return database.InsertChatAutomationParams{}, xerrors.New("create params have no time for the first run of the schedule")
			}
			nextRun = sql.NullTime{Time: n.schedule.Next(p.Now), Valid: true}
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
