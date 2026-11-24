package chatautomation

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// ownerDeletedConstraint is raised by the insert trigger of chat_automations
// when the owner is deleted.
const ownerDeletedConstraint database.CheckConstraint = "chat_automation_owner_deleted"

// Create creates a schedule automation that the actor in ctx owns. now is the
// time that the first run follows. It returns *InvalidRequestError for a field
// that breaks a rule, ErrTargetChatForbidden for a target chat that the actor
// cannot use, ErrLimitReached at the per-owner limit, ErrCreateForbidden when
// RBAC denies the insert, and ErrReferenceUnavailable when a referenced row
// was deleted after the checks.
func Create(ctx context.Context, db database.Store, organizationID uuid.UUID, req codersdk.CreateChatAutomationRequest, now time.Time) (database.ChatAutomation, error) {
	return create(ctx, db, organizationID, uuid.NullUUID{}, req, now)
}

// create is Create with the chat whose agent made the automation, or NULL
// for a person.
func create(ctx context.Context, db database.Store, organizationID uuid.UUID, createdByChatID uuid.NullUUID, req codersdk.CreateChatAutomationRequest, now time.Time) (database.ChatAutomation, error) {
	actor, ok := dbauthz.ActorFromContext(ctx)
	if !ok {
		return database.ChatAutomation{}, dbauthz.ErrNoActor
	}
	owner, err := uuid.Parse(actor.ID)
	if err != nil {
		return database.ChatAutomation{}, xerrors.Errorf("parse actor ID: %w", err)
	}

	sched, rejected := validateRequest(req)
	if len(rejected) == 0 {
		rejected, err = checkReferences(ctx, db, organizationID, owner, req)
		if err != nil {
			return database.ChatAutomation{}, err
		}
	}
	if len(rejected) > 0 {
		return database.ChatAutomation{}, &InvalidRequestError{Validations: rejected}
	}

	enabled := req.Enabled == nil || *req.Enabled
	params := database.InsertChatAutomationParams{
		ID:                   uuid.NullUUID{}, // The database generates it.
		OrganizationID:       organizationID,
		OwnerID:              owner,
		Name:                 strings.TrimSpace(req.Name),
		CreatedByChatID:      createdByChatID,
		Kind:                 database.ChatAutomationKindSchedule,
		Enabled:              enabled,
		Prompt:               req.Prompt,
		TargetMode:           database.ChatAutomationTargetMode(req.TargetMode),
		TargetChatID:         nullUUID(req.TargetChatID),
		NewChatModelConfigID: nullUUID(req.NewChatModelConfigID),
		ProjectID:            nullUUID(req.ProjectID),
		// Store the parts: the string form of a schedule is not a stable
		// format.
		ScheduleCron:      sql.NullString{String: sched.Cron(), Valid: true},
		ScheduleTimeZone:  sql.NullString{String: req.ScheduleTimeZone, Valid: true},
		ScheduleNextRunAt: sql.NullTime{Time: sched.Next(now), Valid: enabled},
	}
	var created database.ChatAutomation
	err = db.InTx(func(tx database.Store) error {
		// One lock per owner keeps two creates from both passing the limit.
		if err := tx.AcquireLock(ctx, database.GenLockID("chat-automations-owner:"+owner.String())); err != nil {
			return xerrors.Errorf("lock owner: %w", err)
		}
		// The count spans organizations, which no organization-scoped grant of
		// the actor covers.
		count, err := tx.CountChatAutomationsByOwnerID(dbauthz.AsChatAutomationCounter(ctx, owner), owner)
		if err != nil {
			return xerrors.Errorf("count chat automations: %w", err)
		}
		if count >= MaxAutomationsPerOwner {
			return ErrLimitReached
		}
		// dbauthz checks create on the automation, update on the target chat
		// and read on the model configuration and the project.
		created, err = tx.InsertChatAutomation(ctx, params)
		switch {
		case dbauthz.IsNotAuthorizedError(err), database.IsCheckViolation(err, ownerDeletedConstraint):
			return ErrCreateForbidden
		case errors.Is(err, sql.ErrNoRows), database.IsForeignKeyViolation(err):
			return ErrReferenceUnavailable
		case err != nil:
			return xerrors.Errorf("insert chat automation: %w", err)
		}
		return nil
	}, nil)
	return created, err
}

// checkReferences reads the chat, model configuration or project that the
// request names, as the actor. A row that does not exist and one that the
// actor cannot read give the same answer.
func checkReferences(ctx context.Context, db database.Store, organizationID, owner uuid.UUID, req codersdk.CreateChatAutomationRequest) (violations, error) {
	var v violations
	if req.TargetChatID != nil {
		chat, err := db.GetChatByID(ctx, *req.TargetChatID)
		switch {
		// Every run sends the prompt as the owner of the automation and
		// processes it with the chat owner's credentials, so only the chat
		// owner may target a chat, as in checkSendChatMessage.
		case httpapi.Is404Error(err), err == nil && chat.OwnerID != owner:
			return nil, ErrTargetChatForbidden
		case err != nil:
			return nil, xerrors.Errorf("get target chat: %w", err)
		case chat.OrganizationID != organizationID:
			v.add("target_chat_id", "Must be a chat in this organization.")
		case chat.ParentChatID.Valid:
			v.add("target_chat_id", "Must be a root chat, not a sub-agent chat.")
		}
	}
	if req.NewChatModelConfigID != nil {
		config, err := db.GetChatModelConfigByID(ctx, *req.NewChatModelConfigID)
		switch {
		case httpapi.Is404Error(err), err == nil && (config.Deleted || config.OrganizationID != organizationID):
			v.add("new_chat_model_config_id", "Model configuration not found in this organization.")
		case err != nil:
			return nil, xerrors.Errorf("get model configuration: %w", err)
		}
	}
	if req.ProjectID != nil {
		project, err := db.GetChatProjectByID(ctx, *req.ProjectID)
		if err != nil && !httpapi.Is404Error(err) {
			return nil, xerrors.Errorf("get project: %w", err)
		}
		usable := err == nil && project.OrganizationID == organizationID
		if usable && project.OwnerID != owner {
			// As chat create does (chatd.ChatProjectUsableBy), count only an
			// ACL grant: a role that can read the project does not share its
			// memory, instructions and skills with the owner's chats.
			//nolint:gocritic // The check answers for the owner, who is the actor.
			usable, err = db.IsChatProjectUsableByUserID(dbauthz.AsChatd(ctx), database.IsChatProjectUsableByUserIDParams{
				ProjectID: project.ID,
				UserID:    owner,
			})
			if err != nil {
				return nil, xerrors.Errorf("check project access: %w", err)
			}
		}
		if !usable {
			v.add("project_id", "Project not found in this organization.")
		}
	}
	return v, nil
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}
