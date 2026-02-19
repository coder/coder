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

var ErrTaskNotDescendant = xerrors.New("target chat is not a descendant of current chat")

const defaultTaskAwaitTimeout = 5 * time.Minute
const taskReportToolCallIDPrefix = "task_report_"
const defaultFallbackTaskReport = "Task completed without explicit report."

type taskServiceStore interface {
	GetChatByID(ctx context.Context, id uuid.UUID) (database.Chat, error)
	GetChatMessagesByChatID(ctx context.Context, chatID uuid.UUID) ([]database.ChatMessage, error)
	InsertChat(ctx context.Context, arg database.InsertChatParams) (database.Chat, error)
	InsertChatMessage(ctx context.Context, arg database.InsertChatMessageParams) (database.ChatMessage, error)
	ListChildChatsByParentID(ctx context.Context, parentChatID uuid.UUID) ([]database.Chat, error)
	UpdateChatStatus(ctx context.Context, arg database.UpdateChatStatusParams) (database.Chat, error)
	UpdateChatTaskReport(ctx context.Context, arg database.UpdateChatTaskReportParams) (database.Chat, error)
	UpdateChatTaskStatus(ctx context.Context, arg database.UpdateChatTaskStatusParams) (database.Chat, error)
}

type taskServiceInterrupter interface {
	InterruptChat(chatID uuid.UUID) bool
}

// TaskService handles delegated chat task lifecycle transitions and
// in-memory report waiting.
type TaskService struct {
	db taskServiceStore

	interrupter taskServiceInterrupter

	waitersMu sync.Mutex
	waiters   map[uuid.UUID][]chan string
	reports   map[uuid.UUID]string
}

func newTaskService(db taskServiceStore, interrupter taskServiceInterrupter) *TaskService {
	return &TaskService{
		db:          db,
		interrupter: interrupter,
		waiters:     make(map[uuid.UUID][]chan string),
		reports:     make(map[uuid.UUID]string),
	}
}

func (s *TaskService) CreateChildTaskChat(
	ctx context.Context,
	parent database.Chat,
	prompt string,
	title string,
	background bool,
) (database.Chat, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return database.Chat{}, xerrors.New("prompt is required")
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
		TaskStatus: database.NullChatTaskStatus{
			ChatTaskStatus: database.ChatTaskStatusQueued,
			Valid:          true,
		},
		Title:       title,
		ModelConfig: parent.ModelConfig,
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("insert child chat: %w", err)
	}

	userContent, err := marshalContentBlocks([]fantasy.Content{
		fantasy.TextContent{Text: prompt},
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("marshal child chat user message: %w", err)
	}

	_, err = s.db.InsertChatMessage(ctx, database.InsertChatMessageParams{
		ChatID:  child.ID,
		Role:    string(fantasy.MessageRoleUser),
		Content: userContent,
		Hidden:  false,
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("insert child chat user message: %w", err)
	}

	// Child tasks are always enqueued asynchronously in phase-1, regardless of
	// whether the parent awaits in the same tool call.
	_ = background

	_, err = s.db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:        child.ID,
		Status:    database.ChatStatusPending,
		WorkerID:  uuid.NullUUID{},
		StartedAt: sql.NullTime{},
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("set child chat pending: %w", err)
	}

	return child, nil
}

func (s *TaskService) SetTaskRunning(ctx context.Context, chatID uuid.UUID) error {
	_, err := s.db.UpdateChatTaskStatus(ctx, database.UpdateChatTaskStatusParams{
		TaskStatus: database.ChatTaskStatusRunning,
		ID:         chatID,
	})
	if err != nil {
		return xerrors.Errorf("set task running: %w", err)
	}
	return nil
}

func (s *TaskService) SetTaskAwaitingReport(ctx context.Context, chatID uuid.UUID) error {
	chat, err := s.db.GetChatByID(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("get chat: %w", err)
	}
	if chat.TaskStatus == database.ChatTaskStatusReported {
		return nil
	}

	_, err = s.db.UpdateChatTaskStatus(ctx, database.UpdateChatTaskStatusParams{
		TaskStatus: database.ChatTaskStatusAwaitingReport,
		ID:         chatID,
	})
	if err != nil {
		return xerrors.Errorf("set task awaiting report: %w", err)
	}
	return nil
}

func (s *TaskService) MarkTaskReported(ctx context.Context, chatID uuid.UUID, report string) error {
	report = strings.TrimSpace(report)

	chat, err := s.db.GetChatByID(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("get chat: %w", err)
	}

	_, err = s.db.UpdateChatTaskReport(ctx, database.UpdateChatTaskReportParams{
		TaskReport: sql.NullString{
			String: report,
			Valid:  true,
		},
		ID: chatID,
	})
	if err != nil {
		return xerrors.Errorf("set task report: %w", err)
	}

	_, err = s.db.UpdateChatTaskStatus(ctx, database.UpdateChatTaskStatusParams{
		TaskStatus: database.ChatTaskStatusReported,
		ID:         chatID,
	})
	if err != nil {
		return xerrors.Errorf("set task reported: %w", err)
	}

	if chat.ParentChatID.Valid {
		if err := s.insertParentTaskReportMessage(ctx, chat.ParentChatID.UUID, chatID, report); err != nil {
			return err
		}
	}

	s.resolveReportWaiters(chatID, report)

	if chat.ParentChatID.Valid {
		if err := s.requeueParentChat(ctx, chat.ParentChatID.UUID); err != nil {
			return err
		}
	}

	return nil
}

func (s *TaskService) SynthesizeFallbackTaskReport(ctx context.Context, chatID uuid.UUID) string {
	messages, err := s.db.GetChatMessagesByChatID(ctx, chatID)
	if err != nil {
		return defaultFallbackTaskReport
	}

	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Role != string(fantasy.MessageRoleAssistant) {
			continue
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

	return defaultFallbackTaskReport
}

func (s *TaskService) AwaitTaskReport(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
	timeout time.Duration,
) (string, error) {
	isDescendant, err := s.isDescendant(ctx, parentChatID, targetChatID)
	if err != nil {
		return "", err
	}
	if !isDescendant {
		return "", ErrTaskNotDescendant
	}

	if report, ok := s.cachedReport(targetChatID); ok {
		return report, nil
	}

	targetChat, err := s.db.GetChatByID(ctx, targetChatID)
	if err != nil {
		return "", xerrors.Errorf("get target chat: %w", err)
	}
	if targetChat.TaskReport.Valid {
		report := targetChat.TaskReport.String
		s.cacheReport(targetChatID, report)
		return report, nil
	}
	if targetChat.TaskStatus == database.ChatTaskStatusReported {
		return "", nil
	}

	waiter := make(chan string, 1)
	if report, ok := s.registerWaiter(targetChatID, waiter); ok {
		return report, nil
	}
	defer s.unregisterWaiter(targetChatID, waiter)

	if timeout <= 0 {
		timeout = defaultTaskAwaitTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case report := <-waiter:
		return report, nil
	case <-timer.C:
		return "", xerrors.New("timed out waiting for delegated task report")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *TaskService) TerminateTaskSubtree(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
) error {
	isDescendant, err := s.isDescendant(ctx, parentChatID, targetChatID)
	if err != nil {
		return err
	}
	if !isDescendant {
		return ErrTaskNotDescendant
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

		if err := s.MarkTaskReported(ctx, chat.ID, "terminated"); err != nil {
			return err
		}
	}

	return nil
}

func (s *TaskService) HasActiveDescendants(ctx context.Context, chatID uuid.UUID) (bool, error) {
	descendants, err := s.listDescendants(ctx, chatID)
	if err != nil {
		return false, err
	}
	for _, chat := range descendants {
		if chat.TaskStatus != database.ChatTaskStatusReported {
			return true, nil
		}
	}
	return false, nil
}

func (s *TaskService) isDescendant(
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

func (s *TaskService) subtree(ctx context.Context, rootChatID uuid.UUID) ([]database.Chat, error) {
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

func (s *TaskService) listDescendants(ctx context.Context, chatID uuid.UUID) ([]database.Chat, error) {
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

func (s *TaskService) cachedReport(chatID uuid.UUID) (string, bool) {
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()

	report, ok := s.reports[chatID]
	return report, ok
}

func (s *TaskService) cacheReport(chatID uuid.UUID, report string) {
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()

	s.reports[chatID] = report
}

func (s *TaskService) resolveReportWaiters(chatID uuid.UUID, report string) {
	s.waitersMu.Lock()
	s.reports[chatID] = report
	waiters := s.waiters[chatID]
	delete(s.waiters, chatID)
	s.waitersMu.Unlock()

	for _, waiter := range waiters {
		select {
		case waiter <- report:
		default:
		}
		close(waiter)
	}
}

func (s *TaskService) insertParentTaskReportMessage(
	ctx context.Context,
	parentChatID uuid.UUID,
	childChatID uuid.UUID,
	report string,
) error {
	toolCallID := taskReportToolCallID(childChatID)
	content, err := marshalToolResults([]ToolResultBlock{{
		ToolCallID: toolCallID,
		ToolName:   toolAgentReport,
		Result: map[string]any{
			"chat_id": childChatID.String(),
			"report":  report,
			"status":  string(database.ChatTaskStatusReported),
		},
	}})
	if err != nil {
		return xerrors.Errorf("marshal parent task report tool result: %w", err)
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
		return xerrors.Errorf("insert parent task report tool message: %w", err)
	}

	return nil
}

func taskReportToolCallID(childChatID uuid.UUID) string {
	return taskReportToolCallIDPrefix + childChatID.String()
}

func (s *TaskService) requeueParentChat(ctx context.Context, parentChatID uuid.UUID) error {
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

func (s *TaskService) registerWaiter(chatID uuid.UUID, waiter chan string) (string, bool) {
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()

	if report, ok := s.reports[chatID]; ok {
		return report, true
	}

	s.waiters[chatID] = append(s.waiters[chatID], waiter)
	return "", false
}

func (s *TaskService) unregisterWaiter(chatID uuid.UUID, waiter chan string) {
	s.waitersMu.Lock()
	defer s.waitersMu.Unlock()

	waiters := s.waiters[chatID]
	if len(waiters) == 0 {
		return
	}

	filtered := make([]chan string, 0, len(waiters))
	for _, current := range waiters {
		if current == waiter {
			continue
		}
		filtered = append(filtered, current)
	}

	if len(filtered) == 0 {
		delete(s.waiters, chatID)
		return
	}
	s.waiters[chatID] = filtered
}
