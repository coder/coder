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

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// StaleThreshold matches chatd's in-flight stale timeout for debug rows.
const StaleThreshold = 5 * time.Minute

// Service persists chat debug rows and fans out lightweight change events.
type Service struct {
	db         database.Store
	log        slog.Logger
	pubsub     pubsub.Pubsub
	staleAfter time.Duration
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

// UpdateRunParams contains optional inputs for updating a debug run.
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
type UpdateStepParams struct {
	ID                 uuid.UUID
	ChatID             uuid.UUID
	Status             Status
	AssistantMessageID int64
	NormalizedResponse any
	Usage              any
	Attempts           any
	Error              any
	Metadata           any
	FinishedAt         time.Time
}

// NewService constructs a chat debug persistence service.
func NewService(db database.Store, log slog.Logger, ps pubsub.Pubsub) *Service {
	if db == nil {
		panic("chatdebug: nil database.Store")
	}

	return &Service{
		db:         db,
		log:        log,
		pubsub:     ps,
		staleAfter: StaleThreshold,
	}
}

// SetStaleAfter overrides the in-flight stale threshold used when
// finalizing abandoned debug rows. Zero or negative durations keep the
// default threshold.
func (s *Service) SetStaleAfter(staleAfter time.Duration) {
	if s == nil || staleAfter <= 0 {
		return
	}
	s.staleAfter = staleAfter
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
	if s == nil || s.db == nil {
		return false
	}

	authCtx := chatdContext(ctx)

	chat, err := s.db.GetChatByID(authCtx, chatID)
	if err != nil {
		s.log.Warn(ctx, "failed to load chat debug logging override",
			slog.Error(err),
			slog.F("chat_id", chatID),
		)
		return false
	}
	if chat.DebugLogsEnabledOverride.Valid {
		return chat.DebugLogsEnabledOverride.Bool
	}

	effectiveOwnerID := chat.OwnerID
	if effectiveOwnerID == uuid.Nil {
		effectiveOwnerID = ownerID
	}
	if effectiveOwnerID != uuid.Nil {
		enabled, err := s.db.GetUserChatDebugLoggingEnabled(authCtx, effectiveOwnerID)
		if err == nil {
			return enabled
		}
		if !errors.Is(err, sql.ErrNoRows) {
			s.log.Warn(ctx, "failed to load user chat debug logging setting",
				slog.Error(err),
				slog.F("owner_id", effectiveOwnerID),
			)
			return false
		}
	}

	enabled, err := s.db.GetChatDebugLoggingEnabled(authCtx)
	if err == nil {
		return enabled
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}

	s.log.Warn(ctx, "failed to load deployment chat debug logging setting",
		slog.Error(err),
	)
	return false
}

// CreateRun inserts a new debug run and emits a run update event.
func (s *Service) CreateRun(
	ctx context.Context,
	params CreateRunParams,
) (database.ChatDebugRun, error) {
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
			Summary:             s.nullJSON(params.Summary),
			StartedAt:           sql.NullTime{},
			UpdatedAt:           sql.NullTime{},
			FinishedAt:          sql.NullTime{},
		})
	if err != nil {
		return database.ChatDebugRun{}, err
	}

	s.publishEvent(run.ChatID, EventKindRunUpdate, run.ID, uuid.Nil)
	return run, nil
}

// UpdateRun updates an existing debug run and emits a run update event.
func (s *Service) UpdateRun(
	ctx context.Context,
	params UpdateRunParams,
) (database.ChatDebugRun, error) {
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
			Summary:             s.nullJSON(params.Summary),
			FinishedAt:          nullTime(params.FinishedAt),
			ID:                  params.ID,
			ChatID:              params.ChatID,
		})
	if err != nil {
		return database.ChatDebugRun{}, err
	}

	s.publishEvent(run.ChatID, EventKindRunUpdate, run.ID, uuid.Nil)
	return run, nil
}

// CreateStep inserts a new debug step and emits a step update event.
func (s *Service) CreateStep(
	ctx context.Context,
	params CreateStepParams,
) (database.ChatDebugStep, error) {
	insert := database.InsertChatDebugStepParams{
		RunID:               params.RunID,
		StepNumber:          params.StepNumber,
		Operation:           string(params.Operation),
		Status:              string(params.Status),
		HistoryTipMessageID: nullInt64(params.HistoryTipMessageID),
		AssistantMessageID:  sql.NullInt64{},
		NormalizedRequest:   s.nullJSON(params.NormalizedRequest),
		NormalizedResponse:  pqtype.NullRawMessage{},
		Usage:               pqtype.NullRawMessage{},
		Attempts:            pqtype.NullRawMessage{},
		Error:               pqtype.NullRawMessage{},
		Metadata:            pqtype.NullRawMessage{},
		StartedAt:           sql.NullTime{},
		UpdatedAt:           sql.NullTime{},
		FinishedAt:          sql.NullTime{},
		ChatID:              params.ChatID,
	}

	for {
		step, err := s.db.InsertChatDebugStep(chatdContext(ctx), insert)
		if err == nil {
			// Touch the parent run's updated_at so the stale-
			// finalization sweep does not prematurely interrupt
			// long-running runs that are still producing steps.
			if _, touchErr := s.db.UpdateChatDebugRun(chatdContext(ctx), database.UpdateChatDebugRunParams{
				RootChatID:          uuid.NullUUID{},
				ParentChatID:        uuid.NullUUID{},
				ModelConfigID:       uuid.NullUUID{},
				TriggerMessageID:    sql.NullInt64{},
				HistoryTipMessageID: sql.NullInt64{},
				Status:              sql.NullString{},
				Provider:            sql.NullString{},
				Model:               sql.NullString{},
				Summary:             pqtype.NullRawMessage{},
				FinishedAt:          sql.NullTime{},
				ID:                  params.RunID,
				ChatID:              params.ChatID,
			}); touchErr != nil {
				s.log.Warn(ctx, "failed to touch parent run updated_at",
					slog.F("run_id", params.RunID),
					slog.Error(touchErr),
				)
			}
			s.publishEvent(step.ChatID, EventKindStepUpdate, step.RunID, step.ID)
			return step, nil
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
}

// UpdateStep updates an existing debug step and emits a step update event.
func (s *Service) UpdateStep(
	ctx context.Context,
	params UpdateStepParams,
) (database.ChatDebugStep, error) {
	step, err := s.db.UpdateChatDebugStep(chatdContext(ctx),
		database.UpdateChatDebugStepParams{
			Operation:           sql.NullString{},
			Status:              nullString(string(params.Status)),
			HistoryTipMessageID: sql.NullInt64{},
			AssistantMessageID:  nullInt64(params.AssistantMessageID),
			NormalizedRequest:   pqtype.NullRawMessage{},
			NormalizedResponse:  s.nullJSON(params.NormalizedResponse),
			Usage:               s.nullJSON(params.Usage),
			Attempts:            s.nullJSON(params.Attempts),
			Error:               s.nullJSON(params.Error),
			Metadata:            s.nullJSON(params.Metadata),
			FinishedAt:          nullTime(params.FinishedAt),
			ID:                  params.ID,
			ChatID:              params.ChatID,
		})
	if err != nil {
		return database.ChatDebugStep{}, err
	}

	s.publishEvent(step.ChatID, EventKindStepUpdate, step.RunID, step.ID)
	return step, nil
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

	s.publishEvent(chatID, EventKindDelete, uuid.Nil, uuid.Nil)
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

	s.publishEvent(chatID, EventKindDelete, uuid.Nil, uuid.Nil)
	return deleted, nil
}

// FinalizeStale finalizes stale in-flight debug rows and emits a broadcast.
func (s *Service) FinalizeStale(
	ctx context.Context,
) (database.FinalizeStaleChatDebugRowsRow, error) {
	staleAfter := s.staleAfter
	if staleAfter <= 0 {
		staleAfter = StaleThreshold
	}

	result, err := s.db.FinalizeStaleChatDebugRows(
		chatdContext(ctx),
		time.Now().Add(-staleAfter),
	)
	if err != nil {
		return database.FinalizeStaleChatDebugRowsRow{}, err
	}

	if result.RunsFinalized > 0 || result.StepsFinalized > 0 {
		s.publishEvent(uuid.Nil, EventKindFinalize, uuid.Nil, uuid.Nil)
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

func (s *Service) nullJSON(value any) pqtype.NullRawMessage {
	if value == nil {
		return pqtype.NullRawMessage{}
	}

	data, err := json.Marshal(value)
	if err != nil {
		s.log.Warn(context.Background(), "failed to marshal chat debug JSON",
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
	chatID uuid.UUID,
	kind EventKind,
	runID uuid.UUID,
	stepID uuid.UUID,
) {
	if s.pubsub == nil {
		s.log.Debug(context.Background(),
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
		s.log.Warn(context.Background(), "failed to marshal chat debug event",
			slog.Error(err),
			slog.F("kind", kind),
			slog.F("chat_id", chatID),
		)
		return
	}

	channel := PubsubChannel(chatID)
	if err := s.pubsub.Publish(channel, data); err != nil {
		s.log.Warn(context.Background(), "failed to publish chat debug event",
			slog.Error(err),
			slog.F("channel", channel),
			slog.F("kind", kind),
			slog.F("chat_id", chatID),
		)
	}
}
