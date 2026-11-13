package chatd

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// automationsExperimentEnabled reports whether the chat-automations experiment
// is on for userID. A deployment without an evaluator has it off, and the
// evaluator fails closed.
func (p *Server) automationsExperimentEnabled(ctx context.Context, userID uuid.UUID) bool {
	return p.experimentEvaluator != nil &&
		p.experimentEvaluator.Enabled(ctx, userID, codersdk.ExperimentChatAutomations)
}

// manageAutomationsAvailable reports whether a turn of chat offers the
// manage_automations tool. The RFC says "The tool is available only in
// non-archived root chats outside plan and explore modes", and "The tool is
// enabled by default while the experiment is on." The experiment is read on
// each turn, so turning it off removes the tool without a write to the chat.
// The cheap column checks run first, so the rules are read only for chats that
// have the switch on.
func (p *Server) manageAutomationsAvailable(ctx context.Context, chat database.Chat) bool {
	return chat.ManageAutomationsEnabled &&
		!chat.ParentChatID.Valid &&
		!chat.Archived &&
		(!chat.PlanMode.Valid || chat.PlanMode.ChatPlanMode != database.ChatPlanModePlan) &&
		!isExploreSubagentMode(chat.Mode) &&
		p.automationsExperimentEnabled(ctx, chat.OwnerID)
}
