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
	// goalCompletionReminder tags identify hidden mid-turn reminder
	// messages written by an older revision of the goal loop. The
	// builder is gone; the parser stays so historical rows keep their
	// mid-turn (non-boundary) role in step counting and stop-after
	// scoping.
	goalCompletionReminderOpenTag  = "<goal-completion-required>"
	goalCompletionReminderCloseTag = "</goal-completion-required>"

	goalResumeKickOpenTag  = "<goal-resumed>"
	goalResumeKickCloseTag = "</goal-resumed>"

	goalContinuationOpenTag  = "<goal-continuation>"
	goalContinuationCloseTag = "</goal-continuation>"
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
		"Call complete_goal with this goal_id when the objective is verifiably done.\n" +
		"If you cannot proceed without the user, call block_goal with this goal_id and the reason.", nil
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

// goalContinuationText steers an auto-continuation turn. It mirrors the
// resume kick but adds a completion audit so long-running loops do not
// drift into claiming success without evidence.
func goalContinuationText(goalID uuid.UUID) (string, error) {
	tagged, err := goalTaggedPayload(goalID, goalContinuationOpenTag, goalContinuationCloseTag)
	if err != nil {
		return "", err
	}
	return tagged + "\n\n" +
		"The chat goal is still active, so work continues automatically.\n" +
		"Continue working toward the objective. Completion is unproven until verified against evidence from the workspace or conversation.\n" +
		"If the objective is verifiably done, call complete_goal with this goal_id and a concise summary.\n" +
		"If you cannot proceed without the user, or you are stuck on the same obstacle repeatedly, call block_goal with this goal_id and the reason.", nil
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

// goalContinuationMessage builds the hidden user message that starts an
// auto-continuation turn when a goal turn finishes with the goal still
// active. Like the resume kick it is a real turn boundary.
func goalContinuationMessage(goalID uuid.UUID, modelConfigID uuid.UUID) (chatstate.Message, error) {
	text, err := goalContinuationText(goalID)
	if err != nil {
		return chatstate.Message{}, err
	}
	return hiddenGoalUserMessage(text, modelConfigID, uuid.Nil)
}

// appendHiddenGoalMessages merges model-only goal messages (legacy
// completion reminders, resume kicks, and continuation kicks) from
// promptRows into messages. Generation decisions need them because they
// open or extend the turn the decision loop is driving.
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
	if _, reminder, err := parseGoalCompletionReminderMessage(msg); err != nil || reminder {
		return reminder, err
	}
	if _, kick, err := parseGoalResumeKickMessage(msg); err != nil || kick {
		return kick, err
	}
	_, continuation, err := parseGoalContinuationMessage(msg)
	return continuation, err
}

func isGoalTurnBoundaryMessageBestEffort(msg database.ChatMessage) bool {
	if _, resume, err := parseGoalResumeKickMessage(msg); err == nil && resume {
		return true
	}
	_, continuation, err := parseGoalContinuationMessage(msg)
	return err == nil && continuation
}

func parseGoalCompletionReminderMessage(msg database.ChatMessage) (uuid.UUID, bool, error) {
	return parseGoalTaggedMessage(msg, goalCompletionReminderOpenTag, goalCompletionReminderCloseTag)
}

func parseGoalResumeKickMessage(msg database.ChatMessage) (uuid.UUID, bool, error) {
	return parseGoalTaggedMessage(msg, goalResumeKickOpenTag, goalResumeKickCloseTag)
}

func parseGoalContinuationMessage(msg database.ChatMessage) (uuid.UUID, bool, error) {
	return parseGoalTaggedMessage(msg, goalContinuationOpenTag, goalContinuationCloseTag)
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
