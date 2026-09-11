package chatd

import (
	"cmp"
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
	goalResumeKickOpenTag  = "<goal-resumed>"
	goalResumeKickCloseTag = "</goal-resumed>"
)

func goalTaggedPayload(goalID uuid.UUID, openTag, closeTag string) (string, error) {
	payload, err := json.Marshal(struct {
		GoalID string `json:"goal_id"`
	}{
		GoalID: goalID.String(),
	})
	if err != nil {
		return "", xerrors.Errorf("marshal goal message payload: %w", err)
	}
	return openTag + "\n" + string(payload) + "\n" + closeTag, nil
}

func goalResumeKickText(goalID uuid.UUID) (string, error) {
	tagged, err := goalTaggedPayload(goalID, goalResumeKickOpenTag, goalResumeKickCloseTag)
	if err != nil {
		return "", err
	}
	return tagged + "\n\n" +
		"The user resumed the chat goal.\n" +
		"Continue working toward the objective.\n" +
		"Call complete_goal with this goal_id when the objective is verifiably done.", nil
}

func hiddenGoalUserMessage(text string, modelConfigID uuid.UUID, createdBy uuid.UUID) (chatstate.Message, error) {
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	if err != nil {
		return chatstate.Message{}, xerrors.Errorf("marshal hidden goal message: %w", err)
	}
	return chatstate.Message{
		Role:           database.ChatMessageRoleUser,
		Content:        content,
		Visibility:     database.ChatMessageVisibilityModel,
		ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: modelConfigID != uuid.Nil},
		CreatedBy:      uuid.NullUUID{UUID: createdBy, Valid: createdBy != uuid.Nil},
		ContentVersion: chatprompt.CurrentContentVersion,
	}, nil
}

// goalResumeKickMessage builds the hidden user message that starts a
// turn when a paused goal is resumed on an idle chat. It is a real turn
// boundary: step counting and stop-after scoping reset at the kick.
func goalResumeKickMessage(goalID uuid.UUID, modelConfigID uuid.UUID, createdBy uuid.UUID) (chatstate.Message, error) {
	text, err := goalResumeKickText(goalID)
	if err != nil {
		return chatstate.Message{}, err
	}
	return hiddenGoalUserMessage(text, modelConfigID, createdBy)
}

// appendHiddenGoalMessages merges model-only goal messages (resume
// kicks) from promptRows into messages. Generation decisions need them
// because they open the turn the decision loop is driving.
func appendHiddenGoalMessages(messages []database.ChatMessage, promptRows []database.ChatMessage) ([]database.ChatMessage, error) {
	seen := make(map[int64]struct{}, len(messages))
	for _, msg := range messages {
		seen[msg.ID] = struct{}{}
	}
	for _, msg := range promptRows {
		if _, ok := seen[msg.ID]; ok {
			continue
		}
		hidden, err := isHiddenGoalMessage(msg)
		if err != nil {
			return nil, err
		}
		if hidden {
			messages = append(messages, msg)
			seen[msg.ID] = struct{}{}
		}
	}
	slices.SortFunc(messages, func(a, b database.ChatMessage) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return messages, nil
}

func isHiddenGoalMessage(msg database.ChatMessage) (bool, error) {
	_, kick, err := parseGoalResumeKickMessage(msg)
	return kick, err
}

func isGoalTurnBoundaryMessageBestEffort(msg database.ChatMessage) bool {
	_, resume, err := parseGoalResumeKickMessage(msg)
	return err == nil && resume
}

func parseGoalResumeKickMessage(msg database.ChatMessage) (uuid.UUID, bool, error) {
	return parseGoalTaggedMessage(msg, goalResumeKickOpenTag, goalResumeKickCloseTag)
}

func parseGoalTaggedMessage(msg database.ChatMessage, openTag, closeTag string) (uuid.UUID, bool, error) {
	if msg.Role != database.ChatMessageRoleUser || msg.Visibility != database.ChatMessageVisibilityModel {
		return uuid.Nil, false, nil
	}
	parts, err := chatprompt.ParseContent(msg)
	if err != nil {
		return uuid.Nil, false, xerrors.Errorf("parse goal tagged message candidate: %w", err)
	}
	text := textFromParts(parts)
	remainder, ok := strings.CutPrefix(text, openTag+"\n")
	if !ok {
		return uuid.Nil, false, nil
	}
	payload, _, ok := strings.Cut(remainder, "\n"+closeTag)
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
