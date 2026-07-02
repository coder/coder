package chatd

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

const (
	goalCompletionReminderOpenTag  = "<goal-completion-required>"
	goalCompletionReminderCloseTag = "</goal-completion-required>"
)

func goalCompletionReminderText(goalID uuid.UUID) (string, error) {
	payload, err := json.Marshal(struct {
		GoalID string `json:"goal_id"`
	}{
		GoalID: goalID.String(),
	})
	if err != nil {
		return "", xerrors.Errorf("marshal goal completion reminder payload: %w", err)
	}
	return goalCompletionReminderOpenTag + "\n" +
		string(payload) + "\n" +
		goalCompletionReminderCloseTag + "\n\n" +
		"Your previous response ended while this chat goal is still active.\n" +
		"Do not finish the turn with the goal active.\n" +
		"If the objective is satisfied, call complete_goal now with this goal_id and a concise summary.\n" +
		"If the objective is not satisfied, continue working toward it. Ask the user only if blocked.", nil
}

func goalCompletionReminderMessage(goalID uuid.UUID, modelConfigID uuid.UUID, apiKeyID string) (chatstate.Message, error) {
	text, err := goalCompletionReminderText(goalID)
	if err != nil {
		return chatstate.Message{}, err
	}
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	if err != nil {
		return chatstate.Message{}, xerrors.Errorf("marshal goal completion reminder: %w", err)
	}
	return chatstate.Message{
		Role:           database.ChatMessageRoleUser,
		Content:        content,
		Visibility:     database.ChatMessageVisibilityModel,
		ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: modelConfigID != uuid.Nil},
		ContentVersion: chatprompt.CurrentContentVersion,
		APIKeyID:       sql.NullString{String: apiKeyID, Valid: apiKeyID != ""},
	}, nil
}

func appendGoalCompletionReminderMessages(messages []database.ChatMessage, promptRows []database.ChatMessage) ([]database.ChatMessage, error) {
	seen := make(map[int64]struct{}, len(messages))
	for _, msg := range messages {
		seen[msg.ID] = struct{}{}
	}
	for _, msg := range promptRows {
		if _, ok := seen[msg.ID]; ok {
			continue
		}
		_, reminder, err := parseGoalCompletionReminderMessage(msg)
		if err != nil {
			return nil, err
		}
		if reminder {
			messages = append(messages, msg)
			seen[msg.ID] = struct{}{}
		}
	}
	slices.SortFunc(messages, func(a, b database.ChatMessage) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return messages, nil
}

func goalCompletionReminderCountForTurn(messages []database.ChatMessage, goalID uuid.UUID) (int, error) {
	count := 0
	for _, msg := range messages {
		if msg.Deleted || msg.Role != database.ChatMessageRoleUser {
			continue
		}
		reminderGoalID, reminder, err := parseGoalCompletionReminderMessage(msg)
		if err != nil {
			return 0, err
		}
		if reminder {
			if goalID == uuid.Nil || reminderGoalID == goalID {
				count++
			}
			continue
		}
		if !msg.Compressed {
			count = 0
		}
	}
	return count, nil
}

func isGoalCompletionReminderMessage(msg database.ChatMessage) (bool, error) {
	_, reminder, err := parseGoalCompletionReminderMessage(msg)
	return reminder, err
}

func isGoalCompletionReminderMessageBestEffort(msg database.ChatMessage) bool {
	_, reminder, err := parseGoalCompletionReminderMessage(msg)
	return err == nil && reminder
}

func parseGoalCompletionReminderMessage(msg database.ChatMessage) (uuid.UUID, bool, error) {
	if msg.Role != database.ChatMessageRoleUser || msg.Visibility != database.ChatMessageVisibilityModel {
		return uuid.Nil, false, nil
	}
	parts, err := chatprompt.ParseContent(msg)
	if err != nil {
		return uuid.Nil, false, xerrors.Errorf("parse goal completion reminder candidate: %w", err)
	}
	text := textFromParts(parts)
	remainder, ok := strings.CutPrefix(text, goalCompletionReminderOpenTag+"\n")
	if !ok {
		return uuid.Nil, false, nil
	}
	payload, _, ok := strings.Cut(remainder, "\n"+goalCompletionReminderCloseTag)
	if !ok {
		return uuid.Nil, false, nil
	}
	var data struct {
		GoalID string `json:"goal_id"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &data); err != nil {
		return uuid.Nil, false, nil
	}
	goalID, err := uuid.Parse(data.GoalID)
	if err != nil {
		return uuid.Nil, false, nil
	}
	return goalID, true, nil
}
