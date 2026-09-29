package chatd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
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
	// ErrAutomationOwnerOnly is returned when someone other than the owner
	// tries to change an automation.
	ErrAutomationOwnerOnly = xerrors.New("only the owner of a chat automation can change it")
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
// carry the caller's authorization: referenced chats and model configs are
// loaded as the caller. For webhook automations it returns the plaintext
// webhook secret, which is not stored and cannot be read again.
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
		Enabled:         true,
		TargetMode:      targetMode,
		Prompt:          req.Prompt,
		CreatedAt:       now,
		UpdatedAt:       now,
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
	}

	var secret string
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
		sched, err := validateAutomationSchedule(*req.ScheduleCron, *req.ScheduleTimeZone, now)
		if err != nil {
			return database.ChatAutomation{}, "", err
		}
		arg.ScheduleCron = sql.NullString{String: sched.Cron(), Valid: true}
		arg.ScheduleTimeZone = sql.NullString{String: *req.ScheduleTimeZone, Valid: true}
		arg.ScheduleNextRunAt = sql.NullTime{Time: sched.Next(now), Valid: true}
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
		if err := validateAutomationModelConfig(ctx, p.db, params.OrganizationID, *req.NewChatModelConfigID); err != nil {
			return database.ChatAutomation{}, "", err
		}
		arg.NewChatModelConfigID = uuid.NullUUID{UUID: *req.NewChatModelConfigID, Valid: true}
		if req.ReasoningEffort != nil {
			effort, err := validateAutomationReasoningEffort(*req.ReasoningEffort)
			if err != nil {
				return database.ChatAutomation{}, "", err
			}
			arg.ReasoningEffort = database.NullChatReasoningEffort{ChatReasoningEffort: effort, Valid: true}
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
		automation, err = tx.InsertChatAutomation(ctx, arg)
		if err != nil {
			return xerrors.Errorf("insert chat automation: %w", err)
		}
		return nil
	}, &database.TxOptions{Isolation: sql.LevelReadCommitted, TxIdentifier: "create_chat_automation"})
	if err != nil {
		return database.ChatAutomation{}, "", err
	}
	return automation, secret, nil
}

// UpdateAutomation applies the set fields of req to the automation. Only
// the owner can update an automation, even when actorID has broader
// permissions, with one exception: anyone allowed to update it may send a
// request that only disables it. Each set field is validated like on
// create, and fields that do not apply to the automation's kind or target
// mode are rejected.
//
// Disabling bumps the automation's queue generation in the update
// transaction, which touches no chat rows, and then removes the messages
// the automation queued before that cutoff; see
// deleteStaleAutomationQueuedMessages. Re-enabling a schedule moves its
// cursor to the next future occurrence, so missed occurrences never run.
func (p *Server) UpdateAutomation(ctx context.Context, actorID, id uuid.UUID, req codersdk.UpdateChatAutomationRequest) (database.ChatAutomation, error) {
	now := dbtime.Time(p.clock.Now())
	disabling := req.Enabled != nil && !*req.Enabled
	var updated database.ChatAutomation
	err := p.db.InTx(func(tx database.Store) error {
		rows, err := tx.GetChatAutomationsByIDsForUpdate(ctx, []uuid.UUID{id})
		if err != nil {
			return xerrors.Errorf("lock chat automation: %w", err)
		}
		if len(rows) == 0 {
			return ErrAutomationNotFound
		}
		row := rows[0]
		// Administrators can stop someone else's automation, but every
		// other change, including enabling it again, is the owner's.
		disableOnly := disabling && req == codersdk.UpdateChatAutomationRequest{Enabled: req.Enabled}
		if actorID != row.OwnerID && !disableOnly {
			return ErrAutomationOwnerOnly
		}

		arg := automationUpdateParams(row, now)
		// Work computed from an older prompt or schedule is stale once
		// either changes.
		scheduleChanged := false

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
			scheduleChanged = scheduleChanged || *req.Prompt != row.Prompt
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
				scheduleChanged = true
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
			effort, err := validateAutomationReasoningEffort(*req.ReasoningEffort)
			if err != nil {
				return err
			}
			arg.ReasoningEffort = database.NullChatReasoningEffort{ChatReasoningEffort: effort, Valid: true}
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
			if err := validateAutomationModelConfig(ctx, tx, row.OrganizationID, *req.NewChatModelConfigID); err != nil {
				return err
			}
			arg.NewChatModelConfigID = uuid.NullUUID{UUID: *req.NewChatModelConfigID, Valid: true}
		}
		if scheduleChanged && row.Kind == database.ChatAutomationKindSchedule {
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
		case req.Enabled != nil && *req.Enabled && !row.Enabled:
			arg.Enabled = true
			if row.Kind == database.ChatAutomationKindSchedule {
				sched, err := validateAutomationSchedule(arg.ScheduleCron.String, arg.ScheduleTimeZone.String, now)
				if err != nil {
					return err
				}
				arg.ScheduleNextRunAt = sql.NullTime{Time: sched.Next(now), Valid: true}
			}
		}

		updated, err = tx.UpdateChatAutomationByID(ctx, arg)
		if err != nil {
			return xerrors.Errorf("update chat automation: %w", err)
		}
		return nil
	}, &database.TxOptions{Isolation: sql.LevelReadCommitted, TxIdentifier: "update_chat_automation"})
	if err != nil {
		return database.ChatAutomation{}, err
	}
	if disabling {
		p.deleteStaleAutomationQueuedMessages(ctx, updated.ID, updated.QueueGeneration)
	}
	return updated, nil
}

// DeleteAutomation disables an automation, which removes the messages it
// queued, and then deletes it. A queued message that survives, for example
// because the server stopped between the steps, is discarded when it would
// be promoted, because its automation no longer exists.
func (p *Server) DeleteAutomation(ctx context.Context, id uuid.UUID) error {
	now := dbtime.Time(p.clock.Now())
	var disabled database.ChatAutomation
	err := p.db.InTx(func(tx database.Store) error {
		rows, err := tx.GetChatAutomationsByIDsForUpdate(ctx, []uuid.UUID{id})
		if err != nil {
			return xerrors.Errorf("lock chat automation: %w", err)
		}
		if len(rows) == 0 {
			return ErrAutomationNotFound
		}
		arg := automationUpdateParams(rows[0], now)
		arg.Enabled = false
		arg.QueueGeneration = rows[0].QueueGeneration + 1
		disabled, err = tx.UpdateChatAutomationByID(ctx, arg)
		if err != nil {
			return xerrors.Errorf("disable chat automation: %w", err)
		}
		return nil
	}, &database.TxOptions{Isolation: sql.LevelReadCommitted, TxIdentifier: "disable_chat_automation"})
	if err != nil {
		return err
	}
	p.deleteStaleAutomationQueuedMessages(ctx, disabled.ID, disabled.QueueGeneration)
	if err := p.db.DeleteChatAutomationByID(ctx, id); err != nil {
		return xerrors.Errorf("delete chat automation: %w", err)
	}
	return nil
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
		UpdatedAt:            now,
	}
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
// The previous secret stops matching when the rotation commits. Only the
// owner can rotate the secret, even when actorID has broader permissions.
func (p *Server) RotateAutomationSecret(ctx context.Context, actorID, id uuid.UUID) (database.ChatAutomation, string, error) {
	now := dbtime.Time(p.clock.Now())
	var (
		rotated database.ChatAutomation
		secret  string
	)
	err := p.db.InTx(func(tx database.Store) error {
		rows, err := tx.GetChatAutomationsByIDsForUpdate(ctx, []uuid.UUID{id})
		if err != nil {
			return xerrors.Errorf("lock chat automation: %w", err)
		}
		if len(rows) == 0 {
			return ErrAutomationNotFound
		}
		row := rows[0]
		if actorID != row.OwnerID {
			return ErrAutomationOwnerOnly
		}
		if row.Kind != database.ChatAutomationKindWebhook {
			return automationFieldError("kind", "only webhook automations have a secret")
		}
		var hash []byte
		secret, hash, err = newAutomationWebhookSecret()
		if err != nil {
			return err
		}
		rotated, err = tx.UpdateChatAutomationWebhookSecretByID(ctx, database.UpdateChatAutomationWebhookSecretByIDParams{
			ID:                row.ID,
			WebhookSecretHash: hash,
			UpdatedAt:         now,
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

// PreviewAutomationSchedule validates a schedule like CreateAutomation
// does and returns its next n runs after now, in UTC.
func PreviewAutomationSchedule(spec, timeZone string, now time.Time, n int) ([]time.Time, error) {
	sched, err := validateAutomationSchedule(spec, timeZone, now)
	if err != nil {
		return nil, err
	}
	return scheduleNextRuns(sched, now, n), nil
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
func validateAutomationModelConfig(ctx context.Context, store database.Store, organizationID, modelConfigID uuid.UUID) error {
	config, err := store.GetChatModelConfigByID(ctx, modelConfigID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
			return automationFieldError("new_chat_model_config_id", "model config not found")
		}
		return xerrors.Errorf("get model config: %w", err)
	}
	if config.OrganizationID != organizationID {
		return automationFieldError("new_chat_model_config_id", "model config is not in the automation's organization")
	}
	if !config.Enabled {
		return automationFieldError("new_chat_model_config_id", "model config is disabled")
	}
	return nil
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
