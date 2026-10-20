package chatd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/experiments"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// automationNameMaxLength matches the chat_automations_name_length
	// check constraint.
	automationNameMaxLength = 128
	// automationWebhookSecretPrefix identifies automation webhook secrets,
	// for example in secret scanners.
	automationWebhookSecretPrefix = "coder_automation_"
	// automationWebhookSecretBytes is the number of random bytes in a
	// webhook secret.
	automationWebhookSecretBytes = 32
)

var (
	// ErrAutomationNotFound is returned when the automation does not exist
	// or the caller cannot read it.
	ErrAutomationNotFound = xerrors.New("chat automation not found")
	// ErrAutomationLimitReached is returned when the owner already owns the
	// maximum number of automations.
	ErrAutomationLimitReached = xerrors.New("chat automation limit reached")
)

// AutomationValidationError reports an invalid automation field. Field is
// the JSON name of the field in the request.
type AutomationValidationError struct {
	Field  string
	Detail string
}

func (e *AutomationValidationError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Detail)
}

func automationFieldError(field, detail string) error {
	return &AutomationValidationError{Field: field, Detail: detail}
}

// AutomationsEnabled reports whether the chat-automations experiment is on
// for subject. Every automation surface calls it with its own subject
// (management routes: the caller). It fails closed on a nil evaluator or a
// missing subject; the evaluator itself fails closed on read errors.
func AutomationsEnabled(ctx context.Context, ev *experiments.Evaluator, subject uuid.UUID) bool {
	if ev == nil || subject == uuid.Nil {
		return false
	}
	return ev.Enabled(ctx, subject, codersdk.ExperimentChatAutomations)
}

// CreateAutomationParams are the inputs of CreateAutomation.
type CreateAutomationParams struct {
	OrganizationID uuid.UUID
	OwnerID        uuid.UUID
	// CreatedByChatID records the chat that created the automation, if any.
	CreatedByChatID uuid.NullUUID
	Request         codersdk.CreateChatAutomationRequest
}

// CreateAutomation validates and inserts an enabled automation. ctx must
// carry the caller's authorization: referenced chats, model configs, and
// projects are loaded as the caller. For webhook automations it returns the
// plaintext webhook secret, which is not stored and cannot be read again.
func (p *Server) CreateAutomation(ctx context.Context, params CreateAutomationParams) (database.ChatAutomation, string, error) {
	if params.OrganizationID == uuid.Nil || params.OwnerID == uuid.Nil {
		return database.ChatAutomation{}, "", xerrors.New("create automation: organization and owner are required")
	}
	req := params.Request
	now := dbtime.Time(p.clock.Now())

	name, err := validateAutomationName(req.Name)
	if err != nil {
		return database.ChatAutomation{}, "", err
	}
	if err := validateAutomationPrompt(req.Prompt); err != nil {
		return database.ChatAutomation{}, "", err
	}
	kind := database.ChatAutomationKind(req.Kind)
	if !kind.Valid() {
		return database.ChatAutomation{}, "", automationFieldError("kind", fmt.Sprintf("must be one of %q", database.AllChatAutomationKindValues()))
	}
	targetMode := database.ChatAutomationTargetMode(req.TargetMode)
	if !targetMode.Valid() {
		return database.ChatAutomation{}, "", automationFieldError("target_mode", fmt.Sprintf("must be one of %q", database.AllChatAutomationTargetModeValues()))
	}

	arg := database.InsertChatAutomationParams{
		ID:              uuid.New(),
		OrganizationID:  params.OrganizationID,
		OwnerID:         params.OwnerID,
		Name:            name,
		CreatedByChatID: params.CreatedByChatID,
		Kind:            kind,
		// Omitted keeps the default of creating the automation turned on.
		Enabled:    req.Enabled == nil || *req.Enabled,
		TargetMode: targetMode,
		Prompt:     req.Prompt,
		CreatedAt:  now,
		UpdatedAt:  now,
		// The kind and target mode below fill the fields they use.
		TargetChatID:         uuid.NullUUID{},
		NewChatModelConfigID: uuid.NullUUID{},
		ReasoningEffort:      database.NullChatReasoningEffort{},
		WhenBusy:             database.NullChatAutomationWhenBusy{},
		WebhookUse:           database.NullChatAutomationWebhookUse{},
		WebhookSecretHash:    nil,
		WebhookSecretVersion: 0,
		ScheduleCron:         sql.NullString{},
		ScheduleTimeZone:     sql.NullString{},
		ScheduleNextRunAt:    sql.NullTime{},
		ProjectID:            uuid.NullUUID{},
	}

	var (
		secret string
		sched  *cron.Schedule
	)
	switch kind {
	case database.ChatAutomationKindSchedule:
		if req.WebhookUse != nil {
			return database.ChatAutomation{}, "", automationFieldError("webhook_use", "must be omitted for schedule automations")
		}
		if req.ScheduleCron == nil {
			return database.ChatAutomation{}, "", automationFieldError("schedule_cron", "is required for schedule automations")
		}
		if req.ScheduleTimeZone == nil {
			return database.ChatAutomation{}, "", automationFieldError("schedule_time_zone", "is required for schedule automations")
		}
		sched, err = validateAutomationSchedule(*req.ScheduleCron, *req.ScheduleTimeZone, now)
		if err != nil {
			return database.ChatAutomation{}, "", err
		}
		if err := validateAutomationScheduleInterval(sched, now, p.chatLimits.MinAutomationScheduleInterval); err != nil {
			return database.ChatAutomation{}, "", err
		}
		arg.ScheduleCron = sql.NullString{String: sched.Cron(), Valid: true}
		arg.ScheduleTimeZone = sql.NullString{String: *req.ScheduleTimeZone, Valid: true}
	case database.ChatAutomationKindWebhook:
		if req.ScheduleCron != nil {
			return database.ChatAutomation{}, "", automationFieldError("schedule_cron", "must be omitted for webhook automations")
		}
		if req.ScheduleTimeZone != nil {
			return database.ChatAutomation{}, "", automationFieldError("schedule_time_zone", "must be omitted for webhook automations")
		}
		webhookUse := database.ChatAutomationWebhookUseMulti
		if req.WebhookUse != nil {
			webhookUse = database.ChatAutomationWebhookUse(*req.WebhookUse)
			if !webhookUse.Valid() {
				return database.ChatAutomation{}, "", automationFieldError("webhook_use", fmt.Sprintf("must be one of %q", database.AllChatAutomationWebhookUseValues()))
			}
		}
		var hash []byte
		secret, hash, err = newAutomationWebhookSecret()
		if err != nil {
			return database.ChatAutomation{}, "", err
		}
		arg.WebhookUse = database.NullChatAutomationWebhookUse{ChatAutomationWebhookUse: webhookUse, Valid: true}
		arg.WebhookSecretHash = hash
		arg.WebhookSecretVersion = 1
	}

	switch targetMode {
	case database.ChatAutomationTargetModeExistingChat:
		if req.NewChatModelConfigID != nil {
			return database.ChatAutomation{}, "", automationFieldError("new_chat_model_config_id", "must be omitted for existing_chat automations")
		}
		if req.ReasoningEffort != nil {
			return database.ChatAutomation{}, "", automationFieldError("reasoning_effort", "must be omitted for existing_chat automations")
		}
		if req.ProjectID != nil {
			return database.ChatAutomation{}, "", automationFieldError("project_id", "must be omitted for existing_chat automations")
		}
		if req.TargetChatID == nil {
			return database.ChatAutomation{}, "", automationFieldError("target_chat_id", "is required for existing_chat automations")
		}
		if err := validateAutomationTargetChat(ctx, p.db, params.OrganizationID, params.OwnerID, *req.TargetChatID); err != nil {
			return database.ChatAutomation{}, "", err
		}
		// Schedules skip a busy chat by default so occurrences do not pile
		// up; webhooks queue so deliveries are not lost.
		whenBusy := database.ChatAutomationWhenBusySkip
		if kind == database.ChatAutomationKindWebhook {
			whenBusy = database.ChatAutomationWhenBusyQueue
		}
		if req.WhenBusy != nil {
			whenBusy, err = validateAutomationWhenBusy(*req.WhenBusy)
			if err != nil {
				return database.ChatAutomation{}, "", err
			}
		}
		arg.TargetChatID = uuid.NullUUID{UUID: *req.TargetChatID, Valid: true}
		arg.WhenBusy = database.NullChatAutomationWhenBusy{ChatAutomationWhenBusy: whenBusy, Valid: true}
	case database.ChatAutomationTargetModeNewChat:
		if req.TargetChatID != nil {
			return database.ChatAutomation{}, "", automationFieldError("target_chat_id", "must be omitted for new_chat automations")
		}
		if req.WhenBusy != nil {
			return database.ChatAutomation{}, "", automationFieldError("when_busy", "must be omitted for new_chat automations")
		}
		if req.NewChatModelConfigID == nil {
			return database.ChatAutomation{}, "", automationFieldError("new_chat_model_config_id", "is required for new_chat automations")
		}
		modelConfig, err := validateAutomationModelConfig(ctx, p.db, params.OrganizationID, *req.NewChatModelConfigID)
		if err != nil {
			return database.ChatAutomation{}, "", err
		}
		arg.NewChatModelConfigID = uuid.NullUUID{UUID: *req.NewChatModelConfigID, Valid: true}
		if req.ReasoningEffort != nil {
			effort, err := validateAutomationReasoningEffort(*req.ReasoningEffort)
			if err != nil {
				return database.ChatAutomation{}, "", err
			}
			arg.ReasoningEffort = database.NullChatReasoningEffort{ChatReasoningEffort: effort, Valid: true}
			if err := validateAutomationEffortForModel(modelConfig, arg.ReasoningEffort); err != nil {
				return database.ChatAutomation{}, "", err
			}
		}
		if req.ProjectID != nil {
			// Only an update clears the project with the all-zero UUID.
			if *req.ProjectID == uuid.Nil {
				return database.ChatAutomation{}, "", automationFieldError("project_id", "must be a project ID or omitted")
			}
			if err := p.validateAutomationProject(ctx, p.db, params.OrganizationID, params.OwnerID, *req.ProjectID); err != nil {
				return database.ChatAutomation{}, "", err
			}
			arg.ProjectID = uuid.NullUUID{UUID: *req.ProjectID, Valid: true}
		}
	}

	limit := p.chatLimits.MaxAutomationsPerOwner
	var automation database.ChatAutomation
	err = p.db.InTx(func(tx database.Store) error {
		// Serialize creates per owner so concurrent requests cannot both
		// pass the count check.
		if err := tx.AcquireLock(ctx, database.GenLockID("chat-automations-owner:"+params.OwnerID.String())); err != nil {
			return xerrors.Errorf("acquire owner lock: %w", err)
		}
		// The cap counts automations in every organization, including ones
		// the caller cannot read, so the count runs as chatd.
		//nolint:gocritic // Cap enforcement counts rows across organizations the caller may not be able to read.
		count, err := tx.CountChatAutomationsByOwnerID(dbauthz.AsChatd(ctx), params.OwnerID)
		if err != nil {
			return xerrors.Errorf("count owner automations: %w", err)
		}
		if count >= int64(limit) {
			return xerrors.Errorf("%w: a user can own at most %d chat automations", ErrAutomationLimitReached, limit)
		}
		// Read the time after the lock wait, so the first cursor is never
		// already in the past when the automation is created.
		lockedNow := dbtime.Time(p.clock.Now())
		arg.CreatedAt, arg.UpdatedAt = lockedNow, lockedNow
		// An automation created off has no pending occurrence; turning it
		// on computes the next one.
		if sched != nil && arg.Enabled {
			arg.ScheduleNextRunAt = sql.NullTime{Time: sched.Next(lockedNow), Valid: true}
		}
		automation, err = tx.InsertChatAutomation(ctx, arg)
		if err != nil {
			if isAutomationProjectDeleted(err) {
				return automationFieldError("project_id", "project not found")
			}
			return xerrors.Errorf("insert chat automation: %w", err)
		}
		return nil
	}, &database.TxOptions{Isolation: sql.LevelReadCommitted, TxIdentifier: "create_chat_automation"})
	if err != nil {
		return database.ChatAutomation{}, "", err
	}
	return automation, secret, nil
}

// AutomationGuard is an extra check a caller runs on an automation row
// the service holds locked, inside the service's transaction. store is
// that transaction's store. An error aborts the operation, which then
// changes nothing, and is returned unchanged.
type AutomationGuard func(store database.Store, automation database.ChatAutomation) error

// UpdateAutomation applies the set fields of req to the automation. RBAC
// decides who may make the change, through the caller's subject in ctx: a
// request that only disables the automation needs the disable action,
// which administrators also hold on automations they do not own, and
// every other request needs update, which only the owner holds. A denied
// request returns a dbauthz.NotAuthorizedError. Each set field is
// validated like on create, and fields that do not apply to the
// automation's kind or target mode are rejected.
//
// Disabling bumps the automation's queue generation in the update
// transaction, which touches no chat rows, and then removes the messages
// the automation queued before that cutoff; see
// deleteStaleAutomationQueuedMessages. Re-enabling a schedule moves its
// cursor to the next future occurrence, so missed occurrences never run.
//
// A prompt edit does not start a new queue generation: messages the
// automation already queued keep the prompt they were sent with, and only
// disabling or deleting the automation removes them.
//
// guard, when set, runs on the locked stored row and again on the row as
// the update would leave it, before the write.
//
// old is the row as read under the lock before the change. Callers record
// it as the audit before-image, because a row read before the call can be
// stale by the time the lock is held.
func (p *Server) UpdateAutomation(ctx context.Context, id uuid.UUID, req codersdk.UpdateChatAutomationRequest, guard AutomationGuard) (old, updated database.ChatAutomation, err error) {
	disabling := req.Enabled != nil && !*req.Enabled
	disableOnly := disabling && req == codersdk.UpdateChatAutomationRequest{Enabled: req.Enabled}
	err = p.db.InTx(func(tx database.Store) error {
		row, err := lockAutomationForChange(ctx, tx, id, disableOnly)
		if err != nil {
			return err
		}
		old = row
		// Read the time after the lock wait, so a cursor computed below is
		// never already in the past when the update commits.
		now := dbtime.Time(p.clock.Now())
		if guard != nil {
			if err := guard(tx, row); err != nil {
				return err
			}
		}

		arg := automationUpdateParams(row, now)
		// newModelConfig is the model config the request selects, read once
		// for its validation.
		var newModelConfig *database.ChatModelConfig
		// Work computed from an older prompt or schedule is stale once
		// either changes.
		invalidatesPendingRuns := false

		if req.Name != nil {
			arg.Name, err = validateAutomationName(*req.Name)
			if err != nil {
				return err
			}
		}
		if req.Prompt != nil {
			if err := validateAutomationPrompt(*req.Prompt); err != nil {
				return err
			}
			invalidatesPendingRuns = invalidatesPendingRuns || *req.Prompt != row.Prompt
			arg.Prompt = *req.Prompt
		}
		if req.ScheduleCron != nil || req.ScheduleTimeZone != nil {
			if row.Kind != database.ChatAutomationKindSchedule {
				field := "schedule_cron"
				if req.ScheduleCron == nil {
					field = "schedule_time_zone"
				}
				return automationFieldError(field, "applies only to schedule automations")
			}
			spec, timeZone := row.ScheduleCron.String, row.ScheduleTimeZone.String
			if req.ScheduleCron != nil {
				spec = *req.ScheduleCron
			}
			if req.ScheduleTimeZone != nil {
				timeZone = *req.ScheduleTimeZone
			}
			sched, err := validateAutomationSchedule(spec, timeZone, now)
			if err != nil {
				return err
			}
			if sched.Cron() != row.ScheduleCron.String || timeZone != row.ScheduleTimeZone.String {
				// Only a changed schedule must meet the minimum interval,
				// so automations created before a stricter minimum keep
				// working.
				if err := validateAutomationScheduleInterval(sched, now, p.chatLimits.MinAutomationScheduleInterval); err != nil {
					return err
				}
				invalidatesPendingRuns = true
				arg.ScheduleCron = sql.NullString{String: sched.Cron(), Valid: true}
				arg.ScheduleTimeZone = sql.NullString{String: timeZone, Valid: true}
				// A disabled automation has no pending occurrence.
				if row.Enabled {
					arg.ScheduleNextRunAt = sql.NullTime{Time: sched.Next(now), Valid: true}
				}
			}
		}
		if req.ReasoningEffort != nil {
			if row.TargetMode != database.ChatAutomationTargetModeNewChat {
				return automationFieldError("reasoning_effort", "applies only to new_chat automations")
			}
			// An empty value clears the override, so new chats use the
			// model's default effort.
			arg.ReasoningEffort = database.NullChatReasoningEffort{}
			if *req.ReasoningEffort != "" {
				effort, err := validateAutomationReasoningEffort(*req.ReasoningEffort)
				if err != nil {
					return err
				}
				arg.ReasoningEffort = database.NullChatReasoningEffort{ChatReasoningEffort: effort, Valid: true}
			}
		}
		if req.WhenBusy != nil {
			if row.TargetMode != database.ChatAutomationTargetModeExistingChat {
				return automationFieldError("when_busy", "applies only to existing_chat automations")
			}
			whenBusy, err := validateAutomationWhenBusy(*req.WhenBusy)
			if err != nil {
				return err
			}
			arg.WhenBusy = database.NullChatAutomationWhenBusy{ChatAutomationWhenBusy: whenBusy, Valid: true}
		}
		if req.TargetChatID != nil {
			if row.TargetMode != database.ChatAutomationTargetModeExistingChat {
				return automationFieldError("target_chat_id", "applies only to existing_chat automations")
			}
			if err := validateAutomationTargetChat(ctx, tx, row.OrganizationID, row.OwnerID, *req.TargetChatID); err != nil {
				return err
			}
			arg.TargetChatID = uuid.NullUUID{UUID: *req.TargetChatID, Valid: true}
		}
		if req.NewChatModelConfigID != nil {
			if row.TargetMode != database.ChatAutomationTargetModeNewChat {
				return automationFieldError("new_chat_model_config_id", "applies only to new_chat automations")
			}
			modelConfig, err := validateAutomationModelConfig(ctx, tx, row.OrganizationID, *req.NewChatModelConfigID)
			if err != nil {
				return err
			}
			arg.NewChatModelConfigID = uuid.NullUUID{UUID: *req.NewChatModelConfigID, Valid: true}
			newModelConfig = &modelConfig
		}
		// A change to the model or the effort must leave a pair the model
		// offers. Other edits are not refused when an admin later changes
		// what the model offers.
		if req.ReasoningEffort != nil || req.NewChatModelConfigID != nil {
			if err := validateAutomationEffortChange(ctx, tx, newModelConfig, arg); err != nil {
				return err
			}
		}
		if req.ProjectID != nil {
			if row.TargetMode != database.ChatAutomationTargetModeNewChat {
				return automationFieldError("project_id", "applies only to new_chat automations")
			}
			// The all-zero UUID removes the project. Removing it needs no
			// experiment, so an owner can always undo the setting. A
			// changed project only affects chats created later, so pending
			// runs stay valid.
			arg.ProjectID = uuid.NullUUID{}
			if *req.ProjectID != uuid.Nil {
				if err := p.validateAutomationProject(ctx, tx, row.OrganizationID, row.OwnerID, *req.ProjectID); err != nil {
					return err
				}
				arg.ProjectID = uuid.NullUUID{UUID: *req.ProjectID, Valid: true}
			}
		}
		enabling := req.Enabled != nil && *req.Enabled && !row.Enabled
		// Every enable-state transition also starts a new revision. A
		// re-enable can compute the cursor the disable cleared, for example
		// on a replica whose clock is behind, and a publish that observed
		// that cursor before the disable must then fail its recheck instead
		// of running its prompt hooks a second time.
		if row.Kind == database.ChatAutomationKindSchedule && (invalidatesPendingRuns || (disabling && row.Enabled) || enabling) {
			arg.ScheduleRevision = row.ScheduleRevision + 1
		}
		switch {
		case disabling:
			// Disabling an already disabled automation bumps the cutoff
			// again, so a retry also removes rows an earlier attempt
			// missed.
			arg.Enabled = false
			arg.QueueGeneration = row.QueueGeneration + 1
			// A disabled automation has no pending occurrence; re-enabling
			// computes the next one.
			arg.ScheduleNextRunAt = sql.NullTime{}
		case enabling:
			// A used single-use webhook accepts no more events, so turning
			// it on would show an automation that never runs.
			if chatstate.UsedSingleUseWebhook(row) {
				return ErrAutomationWebhookConsumedEnable
			}
			arg.Enabled = true
			if row.Kind == database.ChatAutomationKindSchedule {
				sched, err := validateAutomationSchedule(arg.ScheduleCron.String, arg.ScheduleTimeZone.String, now)
				if err != nil {
					return err
				}
				arg.ScheduleNextRunAt = sql.NullTime{Time: sched.Next(now), Valid: true}
			}
		}

		if guard != nil {
			if err := guard(tx, automationAfterUpdate(row, arg)); err != nil {
				return err
			}
		}
		if disableOnly {
			// The disable query writes the same values as arg, computed in
			// SQL, so the disable action cannot change anything else.
			updated, err = tx.DisableChatAutomationByID(ctx, database.DisableChatAutomationByIDParams{ID: row.ID, UpdatedAt: now})
			if err != nil {
				return xerrors.Errorf("disable chat automation: %w", err)
			}
			return nil
		}
		// A request that changes nothing a reader can see keeps updated_at,
		// so clients and agents do not read a change into it. Disabling a
		// disabled automation still bumps the queue generation to retry
		// the cleanup, which is not a visible change.
		visible := automationAfterUpdate(row, arg)
		visible.QueueGeneration, visible.UpdatedAt = row.QueueGeneration, row.UpdatedAt
		if reflect.DeepEqual(visible, row) {
			arg.UpdatedAt = row.UpdatedAt
			if arg.QueueGeneration == row.QueueGeneration {
				updated = row
				return nil
			}
		}
		updated, err = tx.UpdateChatAutomationByID(ctx, arg)
		if err != nil {
			if isAutomationProjectDeleted(err) {
				return automationFieldError("project_id", "project not found")
			}
			return xerrors.Errorf("update chat automation: %w", err)
		}
		return nil
	}, &database.TxOptions{Isolation: sql.LevelReadCommitted, TxIdentifier: "update_chat_automation"})
	if err != nil {
		return database.ChatAutomation{}, database.ChatAutomation{}, err
	}
	if disabling {
		p.deleteStaleAutomationQueuedMessages(ctx, updated.ID, updated.QueueGeneration)
	}
	return old, updated, nil
}

// DeleteAutomation deletes an automation and then removes the messages it
// queued. Deleting the row is the cutoff: the queue promotion guard
// discards every queued message whose automation no longer exists, so a
// message that survives the cleanup, for example because the server
// stopped between the steps, never runs. Deleting needs the delete
// action, which administrators also hold.
func (p *Server) DeleteAutomation(ctx context.Context, id uuid.UUID) error {
	if err := p.db.DeleteChatAutomationByID(ctx, id); err != nil {
		return xerrors.Errorf("delete chat automation: %w", err)
	}
	p.deleteStaleAutomationQueuedMessages(ctx, id, math.MaxInt64)
	return nil
}

// lockAutomationForChange locks the automation as the caller in ctx. A
// change that only disables it locks the row with read, and the disable
// query then requires the disable action. Every other change locks the
// row with update, so a caller without update is refused before the
// change is validated.
func lockAutomationForChange(ctx context.Context, tx database.Store, id uuid.UUID, disableOnly bool) (database.ChatAutomation, error) {
	if disableOnly {
		rows, err := tx.GetChatAutomationsByIDsForUpdate(ctx, []uuid.UUID{id})
		if err != nil {
			return database.ChatAutomation{}, xerrors.Errorf("lock chat automation: %w", err)
		}
		if len(rows) == 0 {
			return database.ChatAutomation{}, ErrAutomationNotFound
		}
		return rows[0], nil
	}
	row, err := tx.GetChatAutomationByIDForUpdate(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return database.ChatAutomation{}, ErrAutomationNotFound
	}
	if err != nil {
		return database.ChatAutomation{}, xerrors.Errorf("lock chat automation: %w", err)
	}
	return row, nil
}

// automationUpdateParams returns update parameters that leave row
// unchanged apart from its update time.
func automationUpdateParams(row database.ChatAutomation, now time.Time) database.UpdateChatAutomationByIDParams {
	return database.UpdateChatAutomationByIDParams{
		ID:                   row.ID,
		Name:                 row.Name,
		Prompt:               row.Prompt,
		TargetChatID:         row.TargetChatID,
		NewChatModelConfigID: row.NewChatModelConfigID,
		ReasoningEffort:      row.ReasoningEffort,
		WhenBusy:             row.WhenBusy,
		ScheduleCron:         row.ScheduleCron,
		ScheduleTimeZone:     row.ScheduleTimeZone,
		ScheduleRevision:     row.ScheduleRevision,
		ScheduleNextRunAt:    row.ScheduleNextRunAt,
		Enabled:              row.Enabled,
		QueueGeneration:      row.QueueGeneration,
		ProjectID:            row.ProjectID,
		UpdatedAt:            now,
	}
}

// automationAfterUpdate returns row with the values arg writes.
func automationAfterUpdate(row database.ChatAutomation, arg database.UpdateChatAutomationByIDParams) database.ChatAutomation {
	row.Name = arg.Name
	row.Prompt = arg.Prompt
	row.TargetChatID = arg.TargetChatID
	row.NewChatModelConfigID = arg.NewChatModelConfigID
	row.ReasoningEffort = arg.ReasoningEffort
	row.WhenBusy = arg.WhenBusy
	row.ScheduleCron = arg.ScheduleCron
	row.ScheduleTimeZone = arg.ScheduleTimeZone
	row.ScheduleRevision = arg.ScheduleRevision
	row.ScheduleNextRunAt = arg.ScheduleNextRunAt
	row.Enabled = arg.Enabled
	row.QueueGeneration = arg.QueueGeneration
	row.ProjectID = arg.ProjectID
	row.UpdatedAt = arg.UpdatedAt
	return row
}

// deleteStaleAutomationQueuedMessages removes the queued messages that
// automationID delivered before its queue generation reached cutoff. Each
// row is deleted in its own chat transaction through the same transition
// as a manual delete, so queue versions and clients update as usual.
// Running turns are not interrupted. Failures are only logged: the queue
// promotion guard discards any stale row that is left behind.
func (p *Server) deleteStaleAutomationQueuedMessages(ctx context.Context, automationID uuid.UUID, cutoff int64) {
	// Organization admins can disable automations but cannot write the
	// chats of other members, so the cleanup runs as chatd. The cutoff
	// is fixed once the disable commits, so no automation lock is needed.
	//nolint:gocritic // Disabling must clean up every chat the automation delivered to.
	chatdCtx := dbauthz.AsChatd(ctx)
	logger := p.logger.With(slog.F("automation_id", automationID), slog.F("queue_generation_cutoff", cutoff))
	rows, err := p.db.GetChatQueuedMessagesByAutomationBelowGeneration(chatdCtx, database.GetChatQueuedMessagesByAutomationBelowGenerationParams{
		AutomationID: automationID,
		Cutoff:       cutoff,
	})
	if err != nil {
		logger.Warn(ctx, "list stale automation queued messages", slog.Error(err))
		return
	}
	for _, row := range rows {
		err := p.DeleteQueued(chatdCtx, row.ChatID, row.ID)
		if err == nil || errors.Is(err, chatstate.ErrQueuedMessageNotFound) {
			// A missing row was already promoted or deleted.
			continue
		}
		logger.Warn(ctx, "delete stale automation queued message",
			slog.F("chat_id", row.ChatID),
			slog.F("queued_message_id", row.ID),
			slog.Error(err),
		)
	}
}

// RotateAutomationSecret replaces the webhook secret of a webhook
// automation and returns the new plaintext secret, which is not stored.
// The previous secret stops matching when the rotation commits. Rotating
// needs update on the automation, which only the owner holds; a denied
// caller gets a dbauthz.NotAuthorizedError.
func (p *Server) RotateAutomationSecret(ctx context.Context, id uuid.UUID) (database.ChatAutomation, string, error) {
	var (
		rotated database.ChatAutomation
		secret  string
	)
	err := p.db.InTx(func(tx database.Store) error {
		row, err := lockAutomationForChange(ctx, tx, id, false)
		if err != nil {
			return err
		}
		if row.Kind != database.ChatAutomationKindWebhook {
			return automationFieldError("kind", "only webhook automations have a secret")
		}
		// Every delivery to a used single-use webhook is refused, so a new
		// secret would be useless.
		if isSingleUseWebhook(row) && row.WebhookConsumedAt.Valid {
			return ErrAutomationWebhookConsumed
		}
		var hash []byte
		secret, hash, err = newAutomationWebhookSecret()
		if err != nil {
			return err
		}
		rotated, err = tx.UpdateChatAutomationWebhookSecretByID(ctx, database.UpdateChatAutomationWebhookSecretByIDParams{
			ID:                row.ID,
			WebhookSecretHash: hash,
			// Read after the lock wait so updated_at never moves backward.
			UpdatedAt: dbtime.Time(p.clock.Now()),
		})
		if err != nil {
			return xerrors.Errorf("rotate webhook secret: %w", err)
		}
		return nil
	}, &database.TxOptions{Isolation: sql.LevelReadCommitted, TxIdentifier: "rotate_chat_automation_secret"})
	if err != nil {
		return database.ChatAutomation{}, "", err
	}
	return rotated, secret, nil
}

// RunAutomation publishes the saved prompt of an enabled schedule
// automation now, through the same admission as a scheduled occurrence
// but without one: the schedule cursor and revision do not change. The
// run sends the prompt as the owner, so the caller in ctx needs update on
// the automation, which only the owner holds; a denied caller gets a
// dbauthz.NotAuthorizedError.
//
// guard, when set, runs on the automation read before the send and again
// at admission on the locked row whose input is accepted.
func (p *Server) RunAutomation(ctx context.Context, id uuid.UUID, guard AutomationGuard) (PublishAutomationResult, error) {
	// The lock only authorizes the caller and ends with the statement.
	// Admission locks the row again and rechecks it.
	row, err := lockAutomationForChange(ctx, p.db, id, false)
	if err != nil {
		return PublishAutomationResult{}, err
	}
	if row.Kind != database.ChatAutomationKindSchedule {
		return PublishAutomationResult{}, automationFieldError("kind", "only schedule automations can run now")
	}
	if !row.Enabled {
		return PublishAutomationResult{}, ErrAutomationDisabled
	}
	// The kind cannot change, and admission checks enabled again on the
	// locked row.
	return p.publishAutomation(ctx, automationPublish{
		automationID: id,
		content: func(automation database.ChatAutomation) []codersdk.ChatMessagePart {
			return []codersdk.ChatMessagePart{codersdk.ChatMessageText(automation.Prompt)}
		},
		guard: guard,
	})
}

// AutomationNextRuns returns up to n upcoming runs of an enabled schedule
// automation after now, in UTC. It returns nil for webhooks, disabled
// automations, and schedules that no longer parse.
func AutomationNextRuns(row database.ChatAutomation, now time.Time, n int) []time.Time {
	if n <= 0 || !row.Enabled || row.Kind != database.ChatAutomationKindSchedule || !row.ScheduleCron.Valid || !row.ScheduleTimeZone.Valid {
		return nil
	}
	sched, err := cron.Standard(row.ScheduleCron.String, row.ScheduleTimeZone.String)
	if err != nil {
		return nil
	}
	return scheduleNextRuns(sched, now, n)
}

// AutomationScheduleBelowMinimum reports whether row is a schedule
// automation that runs more often than minimum allows. It reports false
// for webhooks and schedules that no longer parse.
func AutomationScheduleBelowMinimum(row database.ChatAutomation, now time.Time, minimum time.Duration) bool {
	if row.Kind != database.ChatAutomationKindSchedule || !row.ScheduleCron.Valid || !row.ScheduleTimeZone.Valid {
		return false
	}
	sched, err := cron.Standard(row.ScheduleCron.String, row.ScheduleTimeZone.String)
	if err != nil {
		return false
	}
	return validateAutomationScheduleInterval(sched, now, minimum) != nil
}

// AutomationSchedulePreview is a validated schedule's upcoming runs.
type AutomationSchedulePreview struct {
	// NextRuns are the next runs after now, in UTC.
	NextRuns []time.Time
	// ClockChangeNote explains the daylight saving rules when the
	// schedule's time zone changes its UTC offset within a year.
	ClockChangeNote string
	// IntervalWarning is set when the schedule runs more often than the
	// deployment's minimum interval, so creating it, or changing a
	// schedule to it, fails. An automation that already has it keeps
	// running.
	IntervalWarning string
}

// automationClockChangeNote is the preview text for schedules whose time
// zone changes its UTC offset. It states the rules cron.Standard applies.
const automationClockChangeNote = "When clocks go back, a time that occurs twice runs once, at its first occurrence. When clocks go forward, a time that is skipped runs at the first valid time after the change."

// PreviewAutomationSchedule validates a schedule like CreateAutomation
// does and returns its next n runs after now. A schedule that runs more
// often than minimum is not an error here: the preview carries a warning
// instead, because an automation saved under a lower minimum keeps its
// schedule.
func PreviewAutomationSchedule(spec, timeZone string, now time.Time, n int, minimum time.Duration) (AutomationSchedulePreview, error) {
	sched, err := validateAutomationSchedule(spec, timeZone, now)
	if err != nil {
		return AutomationSchedulePreview{}, err
	}
	preview := AutomationSchedulePreview{NextRuns: scheduleNextRuns(sched, now, n)}
	// The preview still lists the runs of a schedule below the minimum,
	// because an automation saved under an earlier, lower minimum keeps
	// that schedule and keeps running.
	if interval, ok := sched.ShortestInterval(now); ok && interval < minimum {
		preview.IntervalWarning = fmt.Sprintf(
			"Runs as often as every %s, but this deployment allows at most one run every %s. Saving a new or changed schedule like this fails.",
			formatAutomationInterval(interval), formatAutomationInterval(minimum))
	}
	if offsetChangesWithinYear(sched.Location(), now) {
		preview.ClockChangeNote = automationClockChangeNote
	}
	return preview, nil
}

// offsetChangesWithinYear reports whether loc changes its UTC offset
// within one year after now.
func offsetChangesWithinYear(loc *time.Location, now time.Time) bool {
	_, offset := now.In(loc).Zone()
	end := now.AddDate(1, 0, 0)
	for at := now; at.Before(end); {
		_, periodEnd := at.In(loc).ZoneBounds()
		// A zero end means the current zone period never ends.
		if periodEnd.IsZero() || !periodEnd.Before(end) {
			return false
		}
		if _, next := periodEnd.In(loc).Zone(); next != offset {
			return true
		}
		at = periodEnd
	}
	return false
}

func scheduleNextRuns(sched *cron.Schedule, now time.Time, n int) []time.Time {
	runs := make([]time.Time, 0, n)
	next := now
	for range n {
		next = sched.Next(next)
		if next.IsZero() {
			break
		}
		runs = append(runs, next.UTC())
	}
	return runs
}

func validateAutomationName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if length := utf8.RuneCountInString(name); length < 1 || length > automationNameMaxLength {
		return "", automationFieldError("name", fmt.Sprintf("must be between 1 and %d characters", automationNameMaxLength))
	}
	return name, nil
}

func validateAutomationPrompt(prompt string) error {
	if strings.TrimSpace(prompt) == "" {
		return automationFieldError("prompt", "must not be empty")
	}
	return nil
}

// validateAutomationSchedule parses spec in timeZone and requires at least
// one future occurrence.
func validateAutomationSchedule(spec, timeZone string, now time.Time) (*cron.Schedule, error) {
	if err := cron.ValidateTimeZone(timeZone); err != nil {
		return nil, automationFieldError("schedule_time_zone", err.Error())
	}
	sched, err := cron.Standard(spec, timeZone)
	if err != nil {
		return nil, automationFieldError("schedule_cron", err.Error())
	}
	if sched.Next(now).IsZero() {
		return nil, automationFieldError("schedule_cron", "never runs")
	}
	return sched, nil
}

// validateAutomationScheduleInterval requires sched to leave at least
// minimum between two runs on its wall clock.
func validateAutomationScheduleInterval(sched *cron.Schedule, now time.Time, minimum time.Duration) error {
	interval, ok := sched.ShortestInterval(now)
	if !ok || interval >= minimum {
		return nil
	}
	return automationFieldError("schedule_cron", fmt.Sprintf(
		"runs as often as every %s; this deployment allows at most one run every %s",
		formatAutomationInterval(interval), formatAutomationInterval(minimum)))
}

// formatAutomationInterval writes d in whole hours when it is a multiple
// of an hour, and otherwise in minutes and, when present, seconds.
func formatAutomationInterval(d time.Duration) string {
	if d >= time.Hour && d%time.Hour == 0 {
		return pluralAutomationUnit(int64(d/time.Hour), "hour")
	}
	minutes := pluralAutomationUnit(int64(d/time.Minute), "minute")
	seconds := int64((d % time.Minute) / time.Second)
	switch {
	case seconds == 0:
		return minutes
	case d < time.Minute:
		return pluralAutomationUnit(seconds, "second")
	default:
		return minutes + " " + pluralAutomationUnit(seconds, "second")
	}
}

func pluralAutomationUnit(n int64, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func validateAutomationWhenBusy(value codersdk.ChatAutomationWhenBusy) (database.ChatAutomationWhenBusy, error) {
	whenBusy := database.ChatAutomationWhenBusy(value)
	if !whenBusy.Valid() {
		return "", automationFieldError("when_busy", fmt.Sprintf("must be one of %q", database.AllChatAutomationWhenBusyValues()))
	}
	return whenBusy, nil
}

func validateAutomationReasoningEffort(value string) (database.ChatReasoningEffort, error) {
	effort := database.ChatReasoningEffort(value)
	if !effort.Valid() {
		return "", automationFieldError("reasoning_effort", fmt.Sprintf("must be one of %q", database.AllChatReasoningEffortValues()))
	}
	return effort, nil
}

// validateAutomationTargetChat requires the chat to be readable by the
// caller, in the automation's organization, a root chat, and owned by the
// automation owner.
func validateAutomationTargetChat(ctx context.Context, store database.Store, organizationID, ownerID, chatID uuid.UUID) error {
	chat, err := store.GetChatByID(ctx, chatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
			return automationFieldError("target_chat_id", "chat not found")
		}
		return xerrors.Errorf("get target chat: %w", err)
	}
	if chat.OrganizationID != organizationID {
		return automationFieldError("target_chat_id", "chat is not in the automation's organization")
	}
	if chat.ParentChatID.Valid {
		return automationFieldError("target_chat_id", "must be a root chat, not a subagent chat")
	}
	if chat.OwnerID != ownerID {
		return automationFieldError("target_chat_id", "chat must be owned by the automation owner")
	}
	return nil
}

// validateAutomationModelConfig requires the model config to be readable
// by the caller, in the automation's organization, and enabled. Deleted
// configs are not found.
func validateAutomationModelConfig(ctx context.Context, store database.Store, organizationID, modelConfigID uuid.UUID) (database.ChatModelConfig, error) {
	config, err := store.GetChatModelConfigByID(ctx, modelConfigID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
			return database.ChatModelConfig{}, automationFieldError("new_chat_model_config_id", "model config not found")
		}
		return database.ChatModelConfig{}, xerrors.Errorf("get model config: %w", err)
	}
	if config.OrganizationID != organizationID {
		return database.ChatModelConfig{}, automationFieldError("new_chat_model_config_id", "model config is not in the automation's organization")
	}
	if !config.Enabled {
		return database.ChatModelConfig{}, automationFieldError("new_chat_model_config_id", "model config is disabled")
	}
	return config, nil
}

// validateAutomationEffortForModel requires effort, when set, to be one
// the model offers: the list chatprovider.SelectableReasoningEfforts
// returns, which the API reports as ChatModel.reasoning_efforts and the
// editor offers. A model without such a list takes any valid effort, as
// the editor offers every effort for it.
func validateAutomationEffortForModel(config database.ChatModelConfig, effort database.NullChatReasoningEffort) error {
	if !effort.Valid {
		return nil
	}
	callConfig, err := parseModelConfigOptions(config.Options)
	if err != nil {
		return xerrors.Errorf("read model reasoning efforts: %w", err)
	}
	offered := chatprovider.SelectableReasoningEfforts(callConfig.ReasoningEffort)
	if len(offered) == 0 || slices.Contains(offered, string(effort.ChatReasoningEffort)) {
		return nil
	}
	return automationFieldError("reasoning_effort", fmt.Sprintf("%q is not offered by the model, which offers %s", effort.ChatReasoningEffort, strings.Join(offered, ", ")))
}

// validateAutomationEffortChange checks the effort of an update against the
// model it leaves selected. selected is the newly selected model config,
// or nil when the model does not change; the stored one is then read as
// the caller. A stored model the caller can no longer read is not checked:
// runs are refused for it anyway, as model_unavailable.
func validateAutomationEffortChange(ctx context.Context, store database.Store, selected *database.ChatModelConfig, arg database.UpdateChatAutomationByIDParams) error {
	if !arg.ReasoningEffort.Valid || !arg.NewChatModelConfigID.Valid {
		return nil
	}
	if selected == nil {
		config, err := store.GetChatModelConfigByID(ctx, arg.NewChatModelConfigID.UUID)
		if errors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("get model config: %w", err)
		}
		selected = &config
	}
	return validateAutomationEffortForModel(*selected, arg.ReasoningEffort)
}

// validateAutomationProject requires the chat-projects experiment for the
// automation owner and the project to be readable by the caller, owned by
// the automation owner, and in the automation's organization. Projects are
// private to their owner, so a project of another user is reported like a
// missing one and callers cannot probe for project IDs.
func (p *Server) validateAutomationProject(ctx context.Context, store database.Store, organizationID, ownerID, projectID uuid.UUID) error {
	if !p.experimentEvaluator.Enabled(ctx, ownerID, codersdk.ExperimentChatProjects) {
		return automationFieldError("project_id", "chat projects experiment is not enabled")
	}
	project, err := store.GetChatProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
			return automationFieldError("project_id", "project not found")
		}
		return xerrors.Errorf("get project: %w", err)
	}
	if project.OwnerID != ownerID {
		return automationFieldError("project_id", "project not found")
	}
	if project.OrganizationID != organizationID {
		return automationFieldError("project_id", "project is not in the automation's organization")
	}
	return nil
}

// automationProjectOrganizationCheck is raised by the
// enforce_chat_automation_chat_organization trigger, which runs before the
// foreign key and also fails when the project row is gone.
const automationProjectOrganizationCheck database.CheckConstraint = "chat_automations_project_organization"

// isAutomationProjectDeleted reports whether a write failed because the
// project was deleted after validation read it.
func isAutomationProjectDeleted(err error) bool {
	return database.IsForeignKeyViolation(err, database.ForeignKeyChatAutomationsProjectID) ||
		database.IsCheckViolation(err, automationProjectOrganizationCheck)
}

// newAutomationWebhookSecret returns a new webhook secret and the SHA-256
// hash of the full secret string, which is what gets stored.
func newAutomationWebhookSecret() (string, []byte, error) {
	raw := make([]byte, automationWebhookSecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, xerrors.Errorf("generate webhook secret: %w", err)
	}
	secret := automationWebhookSecretPrefix + base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(secret))
	return secret, hash[:], nil
}
