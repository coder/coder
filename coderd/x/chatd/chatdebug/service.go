package chatdebug

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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

// DefaultStaleThreshold is the fallback stale timeout for debug rows
// when no caller-provided value is supplied.
const DefaultStaleThreshold = 5 * time.Minute

// Service persists chat debug rows and fans out lightweight change events.
type Service struct {
	db           database.Store
	log          slog.Logger
	pubsub       pubsub.Pubsub
	clock        quartz.Clock
	alwaysEnable bool
	// staleAfterNanos stores the stale threshold as nanoseconds in an
	// atomic.Int64 so SetStaleAfter and FinalizeStale can be called
	// from concurrent goroutines without a data race.
	staleAfterNanos atomic.Int64

	// thresholdMu protects thresholdChanged.
	thresholdMu sync.Mutex
	// thresholdChanged is closed by SetStaleAfter to wake heartbeat
	// goroutines so they can re-read the (possibly shorter) interval
	// immediately instead of waiting for the old ticker to fire.
	thresholdChanged chan struct{}
}

// ServiceOption configures optional Service behavior.
type ServiceOption func(*Service)

// WithStaleThreshold overrides the default stale-row finalization
// threshold. Callers that already have a configurable in-flight chat
// timeout (e.g. chatd's InFlightChatStaleAfter) should pass it here
// so the two sweeps stay in sync.
func WithStaleThreshold(d time.Duration) ServiceOption {
	return func(s *Service) {
		if d > 0 {
			s.staleAfterNanos.Store(d.Nanoseconds())
		}
	}
}

// WithAlwaysEnable forces debug logging on for every chat regardless
// of the runtime admin and user opt-in settings. This is used for the
// deployment-level serpent flag.
func WithAlwaysEnable(always bool) ServiceOption {
	return func(s *Service) {
		s.alwaysEnable = always
	}
}

// WithClock overrides the default real clock. Tests inject
// quartz.NewMock(t) to control time-dependent behavior such as
// heartbeat tickers and FinalizeStale timestamps.
func WithClock(c quartz.Clock) ServiceOption {
	return func(s *Service) {
		if c != nil {
			s.clock = c
		}
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
	StepNumber          int32
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
		db:               db,
		log:              log,
		pubsub:           ps,
		clock:            quartz.NewReal(),
		thresholdChanged: make(chan struct{}),
	}
	s.staleAfterNanos.Store(DefaultStaleThreshold.Nanoseconds())
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SetStaleAfter overrides the in-flight stale threshold used when
// finalizing abandoned debug rows. Zero or negative durations are
// ignored, leaving the current threshold (initial or previously
// overridden) unchanged. Active heartbeat goroutines are woken so
// they can re-read the (possibly shorter) interval immediately.
func (s *Service) SetStaleAfter(staleAfter time.Duration) {
	if s == nil || staleAfter <= 0 {
		return
	}
	s.staleAfterNanos.Store(staleAfter.Nanoseconds())

	// Wake all heartbeat goroutines by closing the current channel
	// and replacing it with a fresh one for the next update.
	s.thresholdMu.Lock()
	close(s.thresholdChanged)
	s.thresholdChanged = make(chan struct{})
	s.thresholdMu.Unlock()
}

// thresholdChan returns the current threshold-change notification
// channel. Heartbeat goroutines select on this to detect runtime
// stale-threshold updates.
func (s *Service) thresholdChan() <-chan struct{} {
	s.thresholdMu.Lock()
	defer s.thresholdMu.Unlock()
	return s.thresholdChanged
}

// staleThreshold returns the current stale timeout.
func (s *Service) staleThreshold() time.Duration {
	ns := s.staleAfterNanos.Load()
	d := time.Duration(ns)
	if d <= 0 {
		return DefaultStaleThreshold
	}
	return d
}

// heartbeatInterval returns a safe ticker interval for stream heartbeats.
// It is half the stale threshold so at least one touch lands before the
// stale sweep considers the row abandoned.  The result is clamped to a
// minimum of 1 ms to prevent panics from time.NewTicker(0) with
// pathologically small thresholds, while still staying well below any
// practical stale timeout.
func (s *Service) heartbeatInterval() time.Duration {
	return max(s.staleThreshold()/2, time.Millisecond)
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
// that FinalizeStale already marked as interrupted.
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
// row lock). This prevents concurrent FinalizeStale from setting
// finished_at between the check and the INSERT.
func (s *Service) CreateStep(
	ctx context.Context,
	params CreateStepParams,
) (database.ChatDebugStep, error) {
	now := s.clock.Now()
	insert := database.InsertChatDebugStepParams{
		RunID:               params.RunID,
		StepNumber:          params.StepNumber,
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

	// Cap retry attempts to prevent infinite loops under
	// pathological concurrency. Each iteration performs two DB
	// round-trips (insert + list), so 10 retries is generous.
	const maxCreateStepRetries = 10

	for range maxCreateStepRetries {
		if err := ctx.Err(); err != nil {
			return database.ChatDebugStep{}, err
		}

		step, err := s.db.InsertChatDebugStep(chatdContext(ctx), insert)
		if err == nil {
			// The INSERT CTE atomically bumps the parent run's
			// updated_at, so no separate touch call is needed.
			s.publishEvent(ctx, step.ChatID, EventKindStepUpdate, step.RunID, step.ID)
			return step, nil
		}
		// The INSERT's locked_run CTE filters on id, chat_id, and
		// finished_at IS NULL, so sql.ErrNoRows can mean "run not
		// found", "chat_id mismatch", or "already finalized." Look
		// the run up to disambiguate instead of conflating
		// caller-side data bugs with the legitimate terminal case.
		if errors.Is(err, sql.ErrNoRows) {
			return database.ChatDebugStep{}, s.classifyMissingRun(ctx, params)
		}
		if !database.IsUniqueViolation(err, database.UniqueIndexChatDebugStepsRunStep) {
			return database.ChatDebugStep{}, err
		}

		steps, listErr := s.db.GetChatDebugStepsByRunID(chatdContext(ctx), params.RunID)
		if listErr != nil {
			return database.ChatDebugStep{}, listErr
		}
		nextStepNumber := insert.StepNumber + 1
		for _, existing := range steps {
			if existing.StepNumber >= nextStepNumber {
				nextStepNumber = existing.StepNumber + 1
			}
		}
		insert.StepNumber = nextStepNumber
	}

	return database.ChatDebugStep{}, xerrors.Errorf(
		"failed to create debug step after %d attempts (run_id=%s)",
		maxCreateStepRetries, params.RunID,
	)
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
// service auto-fills the timestamp so the stale sweep does not leave
// terminal rows with finished_at = NULL.
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

// TouchStep bumps the step's and its parent run's updated_at timestamps
// without changing any other fields. This prevents long-running operations
// (e.g. streaming) from being prematurely swept by FinalizeStale, which
// first marks runs stale by chat_debug_runs.updated_at and then cascades
// to steps whose run_id was just finalized.
func (s *Service) TouchStep(
	ctx context.Context,
	stepID uuid.UUID,
	runID uuid.UUID,
	chatID uuid.UUID,
) error {
	// Atomically bump both the step and its parent run so
	// FinalizeStale cannot interleave between the two touches.
	return s.db.TouchChatDebugStepAndRun(chatdContext(ctx),
		database.TouchChatDebugStepAndRunParams{
			Now:    s.clock.Now(),
			StepID: stepID,
			RunID:  runID,
			ChatID: chatID,
		})
}

// DeleteByChatID deletes all debug data for a chat and emits a delete event.
func (s *Service) DeleteByChatID(
	ctx context.Context,
	chatID uuid.UUID,
) (int64, error) {
	deleted, err := s.db.DeleteChatDebugDataByChatID(chatdContext(ctx), chatID)
	if err != nil {
		return 0, err
	}

	s.publishEvent(ctx, chatID, EventKindDelete, uuid.Nil, uuid.Nil)
	return deleted, nil
}

// DeleteAfterMessageID deletes debug data newer than the given message.
func (s *Service) DeleteAfterMessageID(
	ctx context.Context,
	chatID uuid.UUID,
	messageID int64,
) (int64, error) {
	deleted, err := s.db.DeleteChatDebugDataAfterMessageID(
		chatdContext(ctx),
		database.DeleteChatDebugDataAfterMessageIDParams{
			ChatID:    chatID,
			MessageID: messageID,
		},
	)
	if err != nil {
		return 0, err
	}

	s.publishEvent(ctx, chatID, EventKindDelete, uuid.Nil, uuid.Nil)
	return deleted, nil
}

// FinalizeStale finalizes stale in-flight debug rows and emits a broadcast.
func (s *Service) FinalizeStale(
	ctx context.Context,
) (database.FinalizeStaleChatDebugRowsRow, error) {
	now := s.clock.Now()
	result, err := s.db.FinalizeStaleChatDebugRows(
		chatdContext(ctx),
		database.FinalizeStaleChatDebugRowsParams{
			Now:           now,
			UpdatedBefore: now.Add(-s.staleThreshold()),
		},
	)
	if err != nil {
		return database.FinalizeStaleChatDebugRowsRow{}, err
	}

	if result.RunsFinalized > 0 || result.StepsFinalized > 0 {
		s.publishEvent(ctx, uuid.Nil, EventKindFinalize, uuid.Nil, uuid.Nil)
	}
	return result, nil
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
