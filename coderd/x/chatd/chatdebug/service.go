package chatdebug

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/quartz"
)

// Service persists chat debug rows and fans out lightweight change events.
type Service struct {
	db           database.Store
	log          slog.Logger
	pubsub       pubsub.Pubsub
	clock        quartz.Clock
	alwaysEnable bool
}

// ServiceOption configures optional Service behavior.
type ServiceOption func(*Service)

// WithAlwaysEnable forces debug logging on for every chat regardless
// of the runtime admin and user opt-in settings. This is used for the
// deployment-level serpent flag.
func WithAlwaysEnable(always bool) ServiceOption {
	return func(s *Service) {
		s.alwaysEnable = always
	}
}

// CreateRunParams contains friendly inputs for creating a debug run.
type CreateRunParams struct {
	ChatID              uuid.UUID
	RootChatID          uuid.UUID
	ParentChatID        uuid.UUID
	ModelConfigID       uuid.UUID
	TriggerMessageID    int64
	HistoryTipMessageID int64
	Kind                RunKind
	Status              Status
	Provider            string
	Model               string
	Summary             any
}

// UpdateRunParams contains inputs for updating a debug run.
// Zero-valued fields are treated as "keep the existing value" by the
// COALESCE-based SQL query.  Once a field is set it cannot be cleared
// back to NULL; this is intentional for the write-once-finalize
// lifecycle of debug rows.
type UpdateRunParams struct {
	ID         uuid.UUID
	ChatID     uuid.UUID
	Status     Status
	Summary    any
	FinishedAt time.Time
}

// CreateStepParams contains friendly inputs for creating a debug step.
type CreateStepParams struct {
	RunID               uuid.UUID
	ChatID              uuid.UUID
	Operation           Operation
	Status              Status
	HistoryTipMessageID int64
	NormalizedRequest   any
}

// UpdateStepParams contains optional inputs for updating a debug step.
// Most payload fields are typed as any and serialized through nullJSON
// because their shape varies by provider.  The Attempts field uses a
// concrete slice for compile-time safety where the schema is stable.
// Zero-valued fields are treated as "keep the existing value" by the
// COALESCE-based SQL query. Once set, fields cannot be cleared back
// to NULL.  This is intentional for the write-once-finalize lifecycle
// of debug rows.
type UpdateStepParams struct {
	ID                 uuid.UUID
	ChatID             uuid.UUID
	Status             Status
	AssistantMessageID int64
	NormalizedResponse any
	Usage              any
	Attempts           []Attempt
	Error              any
	Metadata           any
	FinishedAt         time.Time
}

// NewService constructs a chat debug persistence service.
func NewService(db database.Store, log slog.Logger, ps pubsub.Pubsub, opts ...ServiceOption) *Service {
	if db == nil {
		panic("chatdebug: nil database.Store")
	}

	s := &Service{
		db:     db,
		log:    log,
		pubsub: ps,
		clock:  quartz.NewReal(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func chatdContext(ctx context.Context) context.Context {
	//nolint:gocritic // AsChatd provides narrowly-scoped daemon access for
	// chat debug persistence reads and writes.
	return dbauthz.AsChatd(ctx)
}

// IsEnabled returns whether debug logging is enabled for the given chat.
func (s *Service) IsEnabled(
	ctx context.Context,
	chatID uuid.UUID,
	ownerID uuid.UUID,
) bool {
	if s == nil {
		return false
	}
	if s.alwaysEnable {
		return true
	}
	if s.db == nil {
		return false
	}

	authCtx := chatdContext(ctx)

	allowUsers, err := s.db.GetChatDebugLoggingAllowUsers(authCtx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false
		}
		s.log.Warn(ctx, "failed to load runtime admin chat debug logging setting",
			slog.Error(err),
		)
		return false
	}
	if !allowUsers {
		return false
	}

	if ownerID == uuid.Nil {
		s.log.Warn(ctx, "missing chat owner for debug logging enablement check",
			slog.F("chat_id", chatID),
		)
		return false
	}

	enabled, err := s.db.GetUserChatDebugLoggingEnabled(authCtx, ownerID)
	if err == nil {
		return enabled
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}

	s.log.Warn(ctx, "failed to load user chat debug logging setting",
		slog.Error(err),
		slog.F("chat_id", chatID),
		slog.F("owner_id", ownerID),
	)
	return false
}

// CreateRun inserts a new debug run and emits a run update event.
func (s *Service) CreateRun(
	ctx context.Context,
	params CreateRunParams,
) (database.ChatDebugRun, error) {
	now := s.clock.Now()
	run, err := s.db.InsertChatDebugRun(chatdContext(ctx),
		database.InsertChatDebugRunParams{
			ChatID:              params.ChatID,
			RootChatID:          nullUUID(params.RootChatID),
			ParentChatID:        nullUUID(params.ParentChatID),
			ModelConfigID:       nullUUID(params.ModelConfigID),
			TriggerMessageID:    nullInt64(params.TriggerMessageID),
			HistoryTipMessageID: nullInt64(params.HistoryTipMessageID),
			Kind:                string(params.Kind),
			Status:              string(params.Status),
			Provider:            nullString(params.Provider),
			Model:               nullString(params.Model),
			Summary:             s.nullJSON(ctx, params.Summary),
			StartedAt:           sql.NullTime{Time: now, Valid: true},
			UpdatedAt:           sql.NullTime{Time: now, Valid: true},
			FinishedAt:          sql.NullTime{},
		})
	if err != nil {
		return database.ChatDebugRun{}, err
	}

	s.publishEvent(ctx, run.ChatID, EventKindRunUpdate, run.ID, uuid.Nil)
	return run, nil
}

// createRunTimeout limits how long best-effort debug persistence can delay
// the call being recorded.
const createRunTimeout = 5 * time.Second

// BeginRun creates a run independently of ctx's cancellation, bounded by
// createRunTimeout. Insert failures are logged and reported as false.
func (s *Service) BeginRun(ctx context.Context, params CreateRunParams) (RunContext, bool) {
	createCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), createRunTimeout)
	defer cancel()
	run, err := s.CreateRun(createCtx, params)
	if err != nil {
		s.log.Warn(ctx, "failed to create debug run",
			slog.F("chat_id", params.ChatID),
			slog.F("run_kind", params.Kind),
			slog.F("provider", params.Provider),
			slog.F("model", params.Model),
			slog.Error(err),
		)
		return RunContext{}, false
	}
	return RunContext{
		RunID:               run.ID,
		ChatID:              params.ChatID,
		RootChatID:          params.RootChatID,
		ParentChatID:        params.ParentChatID,
		ModelConfigID:       params.ModelConfigID,
		TriggerMessageID:    params.TriggerMessageID,
		HistoryTipMessageID: params.HistoryTipMessageID,
		Provider:            params.Provider,
		Model:               params.Model,
	}, true
}

// StartRun returns ctx carrying the run and a finalizer for the call's error.
// Finalization is detached from ctx's cancellation. If creation fails, it
// returns the original ctx and a no-op finalizer.
func (s *Service) StartRun(
	ctx context.Context,
	params CreateRunParams,
	seedSummary map[string]any,
	finalizeTimeout time.Duration,
) (context.Context, func(error)) {
	params.Summary = seedSummary
	runContext, ok := s.BeginRun(ctx, params)
	if !ok {
		return ctx, func(error) {}
	}
	return ContextWithRun(ctx, &runContext), func(runErr error) {
		if err := s.FinalizeRun(ctx, FinalizeRunParams{
			RunID:       runContext.RunID,
			ChatID:      runContext.ChatID,
			Status:      ClassifyError(runErr),
			SeedSummary: seedSummary,
			Timeout:     finalizeTimeout,
		}); err != nil {
			s.log.Warn(ctx, "failed to finalize debug run",
				slog.F("chat_id", runContext.ChatID),
				slog.F("run_kind", params.Kind),
				slog.F("run_id", runContext.RunID),
				slog.Error(err),
			)
		}
	}
}

// UpdateRun updates an existing debug run and emits a run update event.
// When a terminal status is set without an explicit FinishedAt, the
// service auto-fills the timestamp so the row is immediately visible
// to the InsertChatDebugStep atomic guard (finished_at IS NULL).
// UpdateChatDebugRun itself enforces finished_at as write-once: once
// the column is populated, repeated auto-fills or explicit refreshes
// never overwrite the original completion timestamp, so calling this
// more than once on an already-finalized run is idempotent.
func (s *Service) UpdateRun(
	ctx context.Context,
	params UpdateRunParams,
) (database.ChatDebugRun, error) {
	if params.Status.IsTerminal() && params.FinishedAt.IsZero() {
		params.FinishedAt = s.clock.Now()
	}
	run, err := s.db.UpdateChatDebugRun(chatdContext(ctx),
		database.UpdateChatDebugRunParams{
			RootChatID:          uuid.NullUUID{},
			ParentChatID:        uuid.NullUUID{},
			ModelConfigID:       uuid.NullUUID{},
			TriggerMessageID:    sql.NullInt64{},
			HistoryTipMessageID: sql.NullInt64{},
			Status:              nullString(string(params.Status)),
			Provider:            sql.NullString{},
			Model:               sql.NullString{},
			Summary:             s.nullJSON(ctx, params.Summary),
			FinishedAt:          nullTime(params.FinishedAt),
			Now:                 s.clock.Now(),
			ID:                  params.ID,
			ChatID:              params.ChatID,
		})
	if err != nil {
		return database.ChatDebugRun{}, err
	}

	s.publishEvent(ctx, run.ChatID, EventKindRunUpdate, run.ID, uuid.Nil)
	return run, nil
}

// errRunFinalized is returned by CreateStep when the parent run has
// already reached a terminal state (finished_at IS NOT NULL). This
// prevents delayed retries from appending in-progress steps to runs
// that were already finalized.
var errRunFinalized = xerrors.New("parent run is already finalized")

// errRunNotFound is returned by CreateStep when the parent run cannot
// be located (missing run_id or chat_id mismatch). This surfaces
// caller-side data bugs instead of conflating them with the legitimate
// "already finalized" terminal case.
var errRunNotFound = xerrors.New("parent run not found")

// CreateStep inserts a new debug step and emits a step update event.
// It returns errRunFinalized if the parent run has already finished,
// or errRunNotFound if the run_id/chat_id pair does not match an
// existing run. The finalization guard is enforced atomically by the
// INSERT's CTE, which issues an UPDATE on the parent run (taking a
// row lock). This prevents a concurrent finalization from setting
// finished_at between the check and the INSERT.
func (s *Service) CreateStep(
	ctx context.Context,
	params CreateStepParams,
) (database.ChatDebugStep, error) {
	now := s.clock.Now()
	insert := database.InsertChatDebugStepParams{
		RunID:               params.RunID,
		StepNumber:          0, // Zero lets the database number the step.
		Operation:           string(params.Operation),
		Status:              string(params.Status),
		HistoryTipMessageID: nullInt64(params.HistoryTipMessageID),
		AssistantMessageID:  sql.NullInt64{},
		NormalizedRequest:   s.nullJSON(ctx, params.NormalizedRequest),
		NormalizedResponse:  pqtype.NullRawMessage{},
		Usage:               pqtype.NullRawMessage{},
		Attempts:            pqtype.NullRawMessage{},
		Error:               pqtype.NullRawMessage{},
		Metadata:            pqtype.NullRawMessage{},
		StartedAt:           sql.NullTime{Time: now, Valid: true},
		UpdatedAt:           sql.NullTime{Time: now, Valid: true},
		FinishedAt:          sql.NullTime{},
		ChatID:              params.ChatID,
	}

	// The insert numbers the step from its statement snapshot, which it
	// takes before it waits on the run row. Holding the run's step lock
	// from an earlier statement makes that snapshot include every step
	// that the previous holder committed.
	var step database.ChatDebugStep
	err := s.db.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(chatdContext(ctx), database.LockIDChatDebugRunSteps(params.RunID)); err != nil {
			return xerrors.Errorf("lock debug run steps: %w", err)
		}
		var err error
		step, err = tx.InsertChatDebugStep(chatdContext(ctx), insert)
		return err
	}, nil)
	// The INSERT's locked_run CTE filters on id, chat_id, and
	// finished_at IS NULL, so sql.ErrNoRows can mean "run not
	// found", "chat_id mismatch", or "already finalized." Look
	// the run up to disambiguate instead of conflating
	// caller-side data bugs with the legitimate terminal case.
	if errors.Is(err, sql.ErrNoRows) {
		return database.ChatDebugStep{}, s.classifyMissingRun(ctx, params)
	}
	if err != nil {
		return database.ChatDebugStep{}, err
	}
	// The INSERT CTE atomically bumps the parent run's
	// updated_at, so no separate touch call is needed.
	s.publishEvent(ctx, step.ChatID, EventKindStepUpdate, step.RunID, step.ID)
	return step, nil
}

// classifyMissingRun disambiguates the sql.ErrNoRows returned by
// InsertChatDebugStep's locked_run CTE. The CTE filters on id,
// chat_id, and finished_at IS NULL, so empty RETURNING rows can mean
// the run is absent, belongs to a different chat, or has already been
// finalized. GetChatDebugRunByID is keyed only by id, which is
// sufficient to tell these cases apart.
func (s *Service) classifyMissingRun(
	ctx context.Context,
	params CreateStepParams,
) error {
	run, err := s.db.GetChatDebugRunByID(chatdContext(ctx), params.RunID)
	if errors.Is(err, sql.ErrNoRows) {
		return errRunNotFound
	}
	if err != nil {
		return xerrors.Errorf("look up parent run after failed step insert: %w", err)
	}
	if run.ChatID != params.ChatID {
		return errRunNotFound
	}
	if run.FinishedAt.Valid {
		return errRunFinalized
	}
	// The run matches the caller's (run_id, chat_id) and is still
	// open, yet the INSERT returned no rows. This is unexpected
	// under write-once-finalize semantics and likely indicates a
	// concurrent delete or unrelated defect; surface it instead of
	// silently masking it as a terminal case.
	return xerrors.Errorf(
		"InsertChatDebugStep returned no rows but run is still active (run_id=%s)",
		params.RunID,
	)
}

// UpdateStep updates an existing debug step and emits a step update event.
// When a terminal status is set without an explicit FinishedAt, the
// service auto-fills the timestamp so terminal rows do not keep
// finished_at = NULL.
func (s *Service) UpdateStep(
	ctx context.Context,
	params UpdateStepParams,
) (database.ChatDebugStep, error) {
	if params.Status.IsTerminal() && params.FinishedAt.IsZero() {
		params.FinishedAt = s.clock.Now()
	}
	step, err := s.db.UpdateChatDebugStep(chatdContext(ctx),
		database.UpdateChatDebugStepParams{
			Status:              nullString(string(params.Status)),
			HistoryTipMessageID: sql.NullInt64{},
			AssistantMessageID:  nullInt64(params.AssistantMessageID),
			NormalizedRequest:   pqtype.NullRawMessage{},
			NormalizedResponse:  s.nullJSON(ctx, params.NormalizedResponse),
			Usage:               s.nullJSON(ctx, params.Usage),
			Attempts:            s.nullJSON(ctx, params.Attempts),
			Error:               s.nullJSON(ctx, params.Error),
			Metadata:            s.nullJSON(ctx, params.Metadata),
			FinishedAt:          nullTime(params.FinishedAt),
			Now:                 s.clock.Now(),
			ID:                  params.ID,
			ChatID:              params.ChatID,
		})
	if err != nil {
		return database.ChatDebugStep{}, err
	}

	s.publishEvent(ctx, step.ChatID, EventKindStepUpdate, step.RunID, step.ID)
	return step, nil
}

// DeleteByChatID deletes debug data for a chat and emits a delete event.
// The startedBefore bound scopes deletion to runs created before that
// instant so that retried cleanup does not remove runs created by a
// replacement turn that raced ahead of the retry window (for example,
// an unarchive that fires between the initial archive-cleanup attempt
// and its retry).
func (s *Service) DeleteByChatID(
	ctx context.Context,
	chatID uuid.UUID,
	startedBefore time.Time,
) (int64, error) {
	deleted, err := s.db.DeleteChatDebugDataByChatID(
		chatdContext(ctx),
		database.DeleteChatDebugDataByChatIDParams{
			ChatID:        chatID,
			StartedBefore: startedBefore,
		},
	)
	if err != nil {
		return 0, err
	}

	s.publishEvent(ctx, chatID, EventKindDelete, uuid.Nil, uuid.Nil)
	return deleted, nil
}

// DeleteAfterMessageID deletes debug data newer than the given message.
// The startedBefore bound scopes deletion to runs created before that
// instant so that retried cleanup does not remove runs created by a
// replacement turn that raced ahead of the retry window.
func (s *Service) DeleteAfterMessageID(
	ctx context.Context,
	chatID uuid.UUID,
	messageID int64,
	startedBefore time.Time,
) (int64, error) {
	deleted, err := s.db.DeleteChatDebugDataAfterMessageID(
		chatdContext(ctx),
		database.DeleteChatDebugDataAfterMessageIDParams{
			ChatID:        chatID,
			MessageID:     messageID,
			StartedBefore: startedBefore,
		},
	)
	if err != nil {
		return 0, err
	}

	s.publishEvent(ctx, chatID, EventKindDelete, uuid.Nil, uuid.Nil)
	return deleted, nil
}

// FinalizeRunParams bundles the arguments for FinalizeRun.
type FinalizeRunParams struct {
	RunID       uuid.UUID
	ChatID      uuid.UUID
	Status      Status
	SeedSummary map[string]any
	// Timeout for the aggregate + update calls. Zero defaults to 5s.
	Timeout time.Duration
}

// FinalizeRun aggregates the run summary and updates the run status.
// It detaches from the parent context's cancellation so finalization
// succeeds even when the request context is already done. Errors are
// returned but are always safe to ignore; callers that treat debug
// instrumentation as best-effort can discard them.
func (s *Service) FinalizeRun(ctx context.Context, p FinalizeRunParams) error {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	finalizeCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx), timeout,
	)
	defer cancel()

	finalSummary := p.SeedSummary
	if aggregated, aggErr := s.AggregateRunSummary(
		finalizeCtx,
		p.RunID,
		p.SeedSummary,
	); aggErr != nil {
		// Non-fatal: proceed with the seed summary.
		s.log.Warn(ctx, "failed to aggregate debug run summary",
			slog.F("chat_id", p.ChatID),
			slog.F("run_id", p.RunID),
			slog.Error(aggErr),
		)
	} else {
		finalSummary = aggregated
	}

	if _, err := s.UpdateRun(finalizeCtx, UpdateRunParams{
		ID:         p.RunID,
		ChatID:     p.ChatID,
		Status:     p.Status,
		Summary:    finalSummary,
		FinishedAt: s.clock.Now(),
	}); err != nil {
		return xerrors.Errorf("update debug run: %w", err)
	}
	return nil
}

// ClassifyError maps a run error to the appropriate debug status.
// nil → StatusCompleted, context.Canceled → StatusInterrupted,
// everything else → StatusError.
func ClassifyError(err error) Status {
	switch {
	case err == nil:
		return StatusCompleted
	case errors.Is(err, context.Canceled):
		return StatusInterrupted
	default:
		return StatusError
	}
}

func nullUUID(id uuid.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil}
}

func nullInt64(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: v != 0}
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func nullTime(value time.Time) sql.NullTime {
	return sql.NullTime{Time: value, Valid: !value.IsZero()}
}

// jsonClear is a sentinel value that tells nullJSON to emit a valid
// JSON null (JSONB 'null') instead of SQL NULL.  COALESCE treats SQL
// NULL as "keep existing" but replaces with a non-NULL JSONB value,
// so passing jsonClear explicitly overwrites a previously set field.
type jsonClear struct{}

// nullJSON marshals value to a NullRawMessage. When value is nil
// (including typed nils such as `var p *T = nil` whose interface
// representation carries a type but no value) or marshals to JSON
// "null", the result is {Valid: false}. Typed nils fall through the
// `value == nil` guard but produce `[]byte("null")` from
// json.Marshal, which the `bytes.Equal(data, []byte("null"))` check
// catches identically. This is intentional for the write-once-finalize
// pattern: combined with the COALESCE-based UPDATE queries, passing
// nil (typed or untyped) preserves the existing column value. Fields
// accumulate monotonically (request -> response -> usage -> error) and
// never need to be cleared during normal operation. The jsonClear
// sentinel exists for the sole exception (error retry clearing).
func (s *Service) nullJSON(ctx context.Context, value any) pqtype.NullRawMessage {
	if value == nil {
		return pqtype.NullRawMessage{}
	}
	// Sentinel: emit a valid JSONB null so COALESCE replaces
	// any previously stored value.
	if _, ok := value.(jsonClear); ok {
		return pqtype.NullRawMessage{
			RawMessage: json.RawMessage("null"),
			Valid:      true,
		}
	}

	data, err := json.Marshal(value)
	if err != nil {
		s.log.Warn(ctx, "failed to marshal chat debug JSON",
			slog.Error(err),
			slog.F("value_type", fmt.Sprintf("%T", value)),
		)
		return pqtype.NullRawMessage{}
	}
	if bytes.Equal(data, []byte("null")) {
		return pqtype.NullRawMessage{}
	}

	return pqtype.NullRawMessage{RawMessage: data, Valid: true}
}

func (s *Service) publishEvent(
	ctx context.Context,
	chatID uuid.UUID,
	kind EventKind,
	runID uuid.UUID,
	stepID uuid.UUID,
) {
	if s.pubsub == nil {
		s.log.Debug(ctx,
			"chat debug pubsub unavailable; skipping event",
			slog.F("kind", kind),
			slog.F("chat_id", chatID),
		)
		return
	}

	event := DebugEvent{
		Kind:   kind,
		ChatID: chatID,
		RunID:  runID,
		StepID: stepID,
	}
	data, err := json.Marshal(event)
	if err != nil {
		s.log.Warn(ctx, "failed to marshal chat debug event",
			slog.Error(err),
			slog.F("kind", kind),
			slog.F("chat_id", chatID),
		)
		return
	}

	channel := PubsubChannel(chatID)
	if err := s.pubsub.Publish(channel, data); err != nil {
		s.log.Warn(ctx, "failed to publish chat debug event",
			slog.Error(err),
			slog.F("channel", channel),
			slog.F("kind", kind),
			slog.F("chat_id", chatID),
		)
	}
}
