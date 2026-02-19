package chatd

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

var ErrSubagentNotDescendant = xerrors.New("target chat is not a descendant of current chat")

const defaultSubagentAwaitTimeout = 5 * time.Minute
const subagentAwaitPollInterval = 200 * time.Millisecond
const subagentReportToolCallIDPrefix = "subagent_report_"
const defaultFallbackSubagentReport = "Sub-agent completed without explicit report."

const (
	subagentEventRequest  = "request"
	subagentEventResponse = "response"

	subagentResponseMarkerRole   = "__subagent_response_marker"
	subagentReportOnlyMarkerRole = "__subagent_report_only_marker"
)

type subagentServiceStore interface {
	GetChatByID(ctx context.Context, id uuid.UUID) (database.Chat, error)
	GetChatMessagesByChatID(ctx context.Context, chatID uuid.UUID) ([]database.ChatMessage, error)
	GetLatestPendingSubagentRequestIDByChatID(ctx context.Context, chatID uuid.UUID) (uuid.NullUUID, error)
	GetSubagentRequestDurationByChatIDAndRequestID(
		ctx context.Context,
		arg database.GetSubagentRequestDurationByChatIDAndRequestIDParams,
	) (int64, error)
	GetSubagentResponseMessageByChatIDAndRequestID(
		ctx context.Context,
		arg database.GetSubagentResponseMessageByChatIDAndRequestIDParams,
	) (database.ChatMessage, error)
	InsertChat(ctx context.Context, arg database.InsertChatParams) (database.Chat, error)
	InsertChatMessage(ctx context.Context, arg database.InsertChatMessageParams) (database.ChatMessage, error)
	ListChildChatsByParentID(ctx context.Context, parentChatID uuid.UUID) ([]database.Chat, error)
	UpdateChatStatus(ctx context.Context, arg database.UpdateChatStatusParams) (database.Chat, error)
}

type subagentServiceInterrupter interface {
	InterruptChat(chatID uuid.UUID) bool
}

type SubagentAwaitResult struct {
	RequestID  uuid.UUID
	Report     string
	DurationMS int64
}

type subagentRequestKey struct {
	chatID    uuid.UUID
	requestID uuid.UUID
}

// SubagentService handles delegated subagent request/response correlation and
// in-memory waiting for subagent responses.
type SubagentService struct {
	db subagentServiceStore

	interrupter subagentServiceInterrupter

	waitersMu sync.Mutex
	waiters   map[subagentRequestKey][]chan SubagentAwaitResult
	results   map[subagentRequestKey]SubagentAwaitResult
}

func newSubagentService(db subagentServiceStore, interrupter subagentServiceInterrupter) *SubagentService {
	return &SubagentService{
		db:          db,
		interrupter: interrupter,
		waiters:     make(map[subagentRequestKey][]chan SubagentAwaitResult),
		results:     make(map[subagentRequestKey]SubagentAwaitResult),
	}
}

func (s *SubagentService) CreateChildSubagentChat(
	ctx context.Context,
	parent database.Chat,
	prompt string,
	title string,
	background bool,
) (database.Chat, uuid.UUID, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return database.Chat{}, uuid.Nil, xerrors.New("prompt is required")
	}

	title = strings.TrimSpace(title)
	if title == "" {
		title = fallbackChatTitle(prompt)
	}

	rootChatID := parent.ID
	if parent.RootChatID.Valid {
		rootChatID = parent.RootChatID.UUID
	}

	child, err := s.db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:          parent.OwnerID,
		WorkspaceID:      parent.WorkspaceID,
		WorkspaceAgentID: parent.WorkspaceAgentID,
		ParentChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  rootChatID,
			Valid: true,
		},
		Title:       title,
		ModelConfig: parent.ModelConfig,
	})
	if err != nil {
		return database.Chat{}, uuid.Nil, xerrors.Errorf("insert child chat: %w", err)
	}

	requestID := uuid.New()
	if err := s.insertRequestMessage(ctx, child.ID, prompt, requestID); err != nil {
		return database.Chat{}, uuid.Nil, err
	}

	// Child subagents are always enqueued asynchronously in phase-1, regardless
	// of whether the parent awaits in the same tool call.
	_ = background

	child, err = s.requeueChatIfNeeded(ctx, child)
	if err != nil {
		return database.Chat{}, uuid.Nil, err
	}

	s.clearCachedResult(child.ID, requestID)
	return child, requestID, nil
}

func (s *SubagentService) SendSubagentMessage(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
	message string,
) (database.Chat, uuid.UUID, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return database.Chat{}, uuid.Nil, xerrors.New("message is required")
	}

	isDescendant, err := s.isDescendant(ctx, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, uuid.Nil, err
	}
	if !isDescendant {
		return database.Chat{}, uuid.Nil, ErrSubagentNotDescendant
	}

	targetChat, err := s.db.GetChatByID(ctx, targetChatID)
	if err != nil {
		return database.Chat{}, uuid.Nil, xerrors.Errorf("get target chat: %w", err)
	}

	requestID := uuid.New()
	if err := s.insertRequestMessage(ctx, targetChatID, message, requestID); err != nil {
		return database.Chat{}, uuid.Nil, err
	}

	targetChat, err = s.requeueChatIfNeeded(ctx, targetChat)
	if err != nil {
		return database.Chat{}, uuid.Nil, err
	}

	s.clearCachedResult(targetChatID, requestID)
	return targetChat, requestID, nil
}

func (s *SubagentService) insertRequestMessage(
	ctx context.Context,
	chatID uuid.UUID,
	message string,
	requestID uuid.UUID,
) error {
	userContent, err := marshalContentBlocks([]fantasy.Content{
		fantasy.TextContent{Text: message},
	})
	if err != nil {
		return xerrors.Errorf("marshal subagent request message: %w", err)
	}

	_, err = s.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:  chatID,
		Role:    string(fantasy.MessageRoleUser),
		Content: userContent,
		Hidden:  false,
		SubagentRequestID: uuid.NullUUID{
			UUID:  requestID,
			Valid: true,
		},
		SubagentEvent: sql.NullString{
			String: subagentEventRequest,
			Valid:  true,
		},
	})
	if err != nil {
		return xerrors.Errorf("insert subagent request message: %w", err)
	}

	return nil
}

func (s *SubagentService) requeueChatIfNeeded(ctx context.Context, chat database.Chat) (database.Chat, error) {
	if chat.Status != database.ChatStatusWaiting && chat.Status != database.ChatStatusCompleted {
		return chat, nil
	}

	updatedChat, err := s.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:        chat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("requeue subagent chat: %w", err)
	}
	return updatedChat, nil
}

func (s *SubagentService) LatestPendingRequestID(
	ctx context.Context,
	chatID uuid.UUID,
) (uuid.UUID, bool, error) {
	requestID, err := s.db.GetLatestPendingSubagentRequestIDByChatID(ctx, chatID)
	if xerrors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, xerrors.Errorf("get latest pending subagent request: %w", err)
	}
	if !requestID.Valid || requestID.UUID == uuid.Nil {
		return uuid.Nil, false, nil
	}
	return requestID.UUID, true, nil
}

func (s *SubagentService) ShouldRunReportOnlyPass(
	ctx context.Context,
	chatID uuid.UUID,
	requestID uuid.UUID,
) (bool, error) {
	messages, err := s.db.GetChatMessagesByChatID(ctx, chatID)
	if err != nil {
		return false, xerrors.Errorf("get chat messages: %w", err)
	}

	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if !message.SubagentRequestID.Valid || message.SubagentRequestID.UUID != requestID {
			continue
		}
		if message.SubagentEvent.Valid && message.SubagentEvent.String == subagentEventRequest {
			return false, nil
		}
		return true, nil
	}

	return false, nil
}

func (s *SubagentService) MarkReportOnlyPassRequested(
	ctx context.Context,
	chatID uuid.UUID,
	requestID uuid.UUID,
) error {
	content, err := marshalContentBlocks([]fantasy.Content{
		fantasy.TextContent{Text: "report-only pass requested"},
	})
	if err != nil {
		return xerrors.Errorf("marshal report-only marker: %w", err)
	}

	_, err = s.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:  chatID,
		Role:    subagentReportOnlyMarkerRole,
		Content: content,
		Hidden:  true,
		SubagentRequestID: uuid.NullUUID{
			UUID:  requestID,
			Valid: true,
		},
	})
	if err != nil {
		return xerrors.Errorf("insert report-only marker: %w", err)
	}
	return nil
}

func (s *SubagentService) MarkSubagentReported(
	ctx context.Context,
	chatID uuid.UUID,
	report string,
	explicitRequestID uuid.NullUUID,
) (SubagentAwaitResult, error) {
	report = strings.TrimSpace(report)

	chat, err := s.db.GetChatByID(ctx, chatID)
	if err != nil {
		return SubagentAwaitResult{}, xerrors.Errorf("get chat: %w", err)
	}

	requestID, err := s.resolveReportRequestID(ctx, chatID, explicitRequestID)
	if err != nil {
		return SubagentAwaitResult{}, err
	}

	responseContent, err := marshalContentBlocks([]fantasy.Content{
		fantasy.TextContent{Text: report},
	})
	if err != nil {
		return SubagentAwaitResult{}, xerrors.Errorf("marshal subagent response marker: %w", err)
	}

	_, err = s.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:  chatID,
		Role:    subagentResponseMarkerRole,
		Content: responseContent,
		Hidden:  true,
		SubagentRequestID: uuid.NullUUID{
			UUID:  requestID,
			Valid: true,
		},
		SubagentEvent: sql.NullString{
			String: subagentEventResponse,
			Valid:  true,
		},
	})
	if err != nil {
		return SubagentAwaitResult{}, xerrors.Errorf("insert subagent response marker: %w", err)
	}

	if chat.ParentChatID.Valid {
		if err := s.insertParentSubagentReportMessage(
			ctx,
			chat.ParentChatID.UUID,
			chatID,
			requestID,
			report,
		); err != nil {
			return SubagentAwaitResult{}, err
		}
	}

	result, ok, err := s.responseForRequest(ctx, chatID, requestID)
	if err != nil {
		return SubagentAwaitResult{}, err
	}
	if !ok {
		result = SubagentAwaitResult{
			RequestID:  requestID,
			Report:     report,
			DurationMS: 0,
		}
	}

	s.resolveRequestWaiters(subagentRequestKey{chatID: chatID, requestID: requestID}, result)

	if chat.ParentChatID.Valid {
		if err := s.requeueParentChat(ctx, chat.ParentChatID.UUID); err != nil {
			return SubagentAwaitResult{}, err
		}
	}

	return result, nil
}

func (s *SubagentService) resolveReportRequestID(
	ctx context.Context,
	chatID uuid.UUID,
	explicitRequestID uuid.NullUUID,
) (uuid.UUID, error) {
	if explicitRequestID.Valid && explicitRequestID.UUID != uuid.Nil {
		return explicitRequestID.UUID, nil
	}

	requestID, err := s.db.GetLatestPendingSubagentRequestIDByChatID(ctx, chatID)
	if xerrors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, xerrors.New("no pending subagent request found")
	}
	if err != nil {
		return uuid.Nil, xerrors.Errorf("get latest pending subagent request: %w", err)
	}
	if !requestID.Valid || requestID.UUID == uuid.Nil {
		return uuid.Nil, xerrors.New("no pending subagent request found")
	}
	return requestID.UUID, nil
}

func (s *SubagentService) SynthesizeFallbackSubagentReport(
	ctx context.Context,
	chatID uuid.UUID,
	requestID uuid.UUID,
) string {
	messages, err := s.db.GetChatMessagesByChatID(ctx, chatID)
	if err != nil {
		return defaultFallbackSubagentReport
	}

	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Role != string(fantasy.MessageRoleAssistant) {
			continue
		}
		if requestID != uuid.Nil {
			if !message.SubagentRequestID.Valid || message.SubagentRequestID.UUID != requestID {
				continue
			}
		}

		content, parseErr := parseContentBlocks(message.Role, message.Content)
		if parseErr != nil {
			continue
		}
		report := strings.TrimSpace(contentBlocksToText(content))
		if report != "" {
			return report
		}
	}

	return defaultFallbackSubagentReport
}

func (s *SubagentService) AwaitSubagentReport(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
	requestID uuid.UUID,
	timeout time.Duration,
) (SubagentAwaitResult, error) {
	isDescendant, err := s.isDescendant(ctx, parentChatID, targetChatID)
	if err != nil {
		return SubagentAwaitResult{}, err
	}
	if !isDescendant {
		return SubagentAwaitResult{}, ErrSubagentNotDescendant
	}

	key := subagentRequestKey{chatID: targetChatID, requestID: requestID}
	if result, ok := s.cachedResult(targetChatID, requestID); ok {
		return result, nil
	}

	if result, ok, err := s.responseForRequest(ctx, targetChatID, requestID); err != nil {
		return SubagentAwaitResult{}, err
	} else if ok {
		s.cacheResult(targetChatID, requestID, result)
		return result, nil
	}

	waiter := make(chan SubagentAwaitResult, 1)
	if result, ok := s.registerWaiter(key, waiter); ok {
		return result, nil
	}
	defer s.unregisterWaiter(key, waiter)

	if timeout <= 0 {
		timeout = defaultSubagentAwaitTimeout
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(subagentAwaitPollInterval)
	defer ticker.Stop()

	for {
		select {
		case result := <-waiter:
			return result, nil
		case <-ticker.C:
			result, ok, lookupErr := s.responseForRequest(ctx, targetChatID, requestID)
			if lookupErr != nil {
				return SubagentAwaitResult{}, lookupErr
			}
			if ok {
				s.resolveRequestWaiters(key, result)
				return result, nil
			}
		case <-deadline.C:
			return SubagentAwaitResult{}, xerrors.New("timed out waiting for delegated subagent report")
		case <-ctx.Done():
			return SubagentAwaitResult{}, ctx.Err()
		}
	}
}

func (s *SubagentService) responseForRequest(
	ctx context.Context,
	chatID uuid.UUID,
	requestID uuid.UUID,
) (SubagentAwaitResult, bool, error) {
	message, err := s.db.GetSubagentResponseMessageByChatIDAndRequestID(ctx,
		database.GetSubagentResponseMessageByChatIDAndRequestIDParams{
			ChatID:            chatID,
			SubagentRequestID: requestID,
		},
	)
	if xerrors.Is(err, sql.ErrNoRows) {
		return SubagentAwaitResult{}, false, nil
	}
	if err != nil {
		return SubagentAwaitResult{}, false, xerrors.Errorf("get subagent response marker: %w", err)
	}

	duration, err := s.db.GetSubagentRequestDurationByChatIDAndRequestID(ctx,
		database.GetSubagentRequestDurationByChatIDAndRequestIDParams{
			ChatID:            chatID,
			SubagentRequestID: requestID,
		},
	)
	if err != nil {
		return SubagentAwaitResult{}, false, xerrors.Errorf("get subagent request duration: %w", err)
	}

	report := ""
	content, parseErr := parseContentBlocks(message.Role, message.Content)
	if parseErr == nil {
		report = strings.TrimSpace(contentBlocksToText(content))
	}

	return SubagentAwaitResult{
		RequestID:  requestID,
		Report:     report,
		DurationMS: duration,
	}, true, nil
}

func (s *SubagentService) TerminateSubagentSubtree(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
) error {
	isDescendant, err := s.isDescendant(ctx, parentChatID, targetChatID)
	if err != nil {
		return err
	}
	if !isDescendant {
		return ErrSubagentNotDescendant
	}

	subtree, err := s.subtree(ctx, targetChatID)
	if err != nil {
		return err
	}

	for _, chat := range subtree {
		if chat.Status == database.ChatStatusRunning && s.interrupter != nil {
			s.interrupter.InterruptChat(chat.ID)
		}

		if chat.Status == database.ChatStatusPending {
			_, err := s.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
				ID:        chat.ID,
				Status:    database.ChatStatusWaiting,
				WorkerID:  uuid.NullUUID{},
				StartedAt: sql.NullTime{},
			})
			if err != nil {
				return xerrors.Errorf("set pending chat waiting for termination: %w", err)
			}
		}

		for {
			requestID, hasPending, requestErr := s.LatestPendingRequestID(ctx, chat.ID)
			if requestErr != nil {
				return requestErr
			}
			if !hasPending {
				break
			}

			_, reportErr := s.MarkSubagentReported(ctx, chat.ID, "terminated", uuid.NullUUID{
				UUID:  requestID,
				Valid: true,
			})
			if reportErr != nil {
				return reportErr
			}
		}
	}

	return nil
}

func (s *SubagentService) HasActiveDescendants(ctx context.Context, chatID uuid.UUID) (bool, error) {
	descendants, err := s.listDescendants(ctx, chatID)
	if err != nil {
		return false, err
	}
	for _, descendant := range descendants {
		_, hasPending, requestErr := s.LatestPendingRequestID(ctx, descendant.ID)
		if requestErr != nil {
			return false, requestErr
		}
		if hasPending {
			return true, nil
		}
	}
	return false, nil
}

func (s *SubagentService) isDescendant(
	ctx context.Context,
	ancestorChatID uuid.UUID,
	targetChatID uuid.UUID,
) (bool, error) {
	if ancestorChatID == targetChatID {
		return false, nil
	}

	descendants, err := s.listDescendants(ctx, ancestorChatID)
	if err != nil {
		return false, err
	}
	for _, descendant := range descendants {
		if descendant.ID == targetChatID {
			return true, nil
		}
	}
	return false, nil
}

func (s *SubagentService) subtree(ctx context.Context, rootChatID uuid.UUID) ([]database.Chat, error) {
	rootChat, err := s.db.GetChatByID(ctx, rootChatID)
	if err != nil {
		return nil, xerrors.Errorf("get subtree root chat: %w", err)
	}

	descendants, err := s.listDescendants(ctx, rootChatID)
	if err != nil {
		return nil, err
	}

	out := make([]database.Chat, 0, len(descendants)+1)
	out = append(out, rootChat)
	out = append(out, descendants...)
	return out, nil
}

func (s *SubagentService) listDescendants(ctx context.Context, chatID uuid.UUID) ([]database.Chat, error) {
	queue := []uuid.UUID{chatID}
	visited := map[uuid.UUID]struct{}{
		chatID: {},
	}

	out := make([]database.Chat, 0)
	for len(queue) > 0 {
		parentChatID := queue[0]
		queue = queue[1:]

		children, err := s.db.ListChildChatsByParentID(ctx, parentChatID)
		if err != nil {
			return nil, xerrors.Errorf("list child chats for %s: %w", parentChatID, err)
		}

		for _, child := range children {
			if _, ok := visited[child.ID]; ok {
				continue
			}
			visited[child.ID] = struct{}{}
			out = append(out, child)
			queue = append(queue, child.ID)
		}
	}

	return out, nil
}

func (s *SubagentService) cachedResult(chatID uuid.UUID, requestID uuid.UUID) (SubagentAwaitResult, bool) {
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()

	result, ok := s.results[subagentRequestKey{chatID: chatID, requestID: requestID}]
	return result, ok
}

func (s *SubagentService) cacheResult(chatID uuid.UUID, requestID uuid.UUID, result SubagentAwaitResult) {
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()

	s.results[subagentRequestKey{chatID: chatID, requestID: requestID}] = result
}

func (s *SubagentService) clearCachedResult(chatID uuid.UUID, requestID uuid.UUID) {
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()

	delete(s.results, subagentRequestKey{chatID: chatID, requestID: requestID})
}

func (s *SubagentService) resolveRequestWaiters(
	key subagentRequestKey,
	result SubagentAwaitResult,
) {
	s.waitersMu.Lock()
	s.results[key] = result
	waiters := s.waiters[key]
	delete(s.waiters, key)
	s.waitersMu.Unlock()

	for _, waiter := range waiters {
		select {
		case waiter <- result:
		default:
		}
		close(waiter)
	}
}

func (s *SubagentService) insertParentSubagentReportMessage(
	ctx context.Context,
	parentChatID uuid.UUID,
	childChatID uuid.UUID,
	requestID uuid.UUID,
	report string,
) error {
	toolCallID := subagentReportToolCallID(requestID)
	toolCallContent, err := marshalContentBlocks([]fantasy.Content{
		fantasy.ToolCallContent{
			ToolCallID: toolCallID,
			ToolName:   toolSubagentReport,
			Input:      "{}",
		},
	})
	if err != nil {
		return xerrors.Errorf("marshal parent subagent report tool call: %w", err)
	}

	_, err = s.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:  parentChatID,
		Role:    string(fantasy.MessageRoleAssistant),
		Content: toolCallContent,
		ToolCallID: sql.NullString{
			String: toolCallID,
			Valid:  true,
		},
		// Keep existing visible report card behavior by only surfacing the
		// tool result message to clients.
		Hidden: true,
	})
	if err != nil {
		return xerrors.Errorf("insert parent subagent report tool call message: %w", err)
	}

	content, err := marshalToolResults([]ToolResultBlock{{
		ToolCallID: toolCallID,
		ToolName:   toolSubagentReport,
		Result: map[string]any{
			"chat_id":     childChatID.String(),
			"request_id":  requestID.String(),
			"report":      report,
			"status":      "reported",
			"duration_ms": nil,
		},
	}})
	if err != nil {
		return xerrors.Errorf("marshal parent subagent report tool result: %w", err)
	}

	_, err = s.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:  parentChatID,
		Role:    string(fantasy.MessageRoleTool),
		Content: content,
		ToolCallID: sql.NullString{
			String: toolCallID,
			Valid:  true,
		},
		Hidden: false,
	})
	if err != nil {
		return xerrors.Errorf("insert parent subagent report tool message: %w", err)
	}

	return nil
}

func subagentReportToolCallID(requestID uuid.UUID) string {
	return subagentReportToolCallIDPrefix + requestID.String()
}

func (s *SubagentService) requeueParentChat(ctx context.Context, parentChatID uuid.UUID) error {
	parentChat, err := s.db.GetChatByID(ctx, parentChatID)
	if err != nil {
		return xerrors.Errorf("get parent chat: %w", err)
	}
	if parentChat.Status != database.ChatStatusWaiting && parentChat.Status != database.ChatStatusCompleted {
		return nil
	}

	_, err = s.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:        parentChat.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	if err != nil {
		return xerrors.Errorf("requeue parent chat: %w", err)
	}

	return nil
}

func (s *SubagentService) registerWaiter(
	key subagentRequestKey,
	waiter chan SubagentAwaitResult,
) (SubagentAwaitResult, bool) {
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()

	if result, ok := s.results[key]; ok {
		return result, true
	}

	s.waiters[key] = append(s.waiters[key], waiter)
	return SubagentAwaitResult{}, false
}

func (s *SubagentService) unregisterWaiter(
	key subagentRequestKey,
	waiter chan SubagentAwaitResult,
) {
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()

	waiters := s.waiters[key]
	if len(waiters) == 0 {
		return
	}

	filtered := make([]chan SubagentAwaitResult, 0, len(waiters))
	for _, current := range waiters {
		if current == waiter {
			continue
		}
		filtered = append(filtered, current)
	}

	if len(filtered) == 0 {
		delete(s.waiters, key)
		return
	}
	s.waiters[key] = filtered
}
