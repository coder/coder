package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

const manageAutomationsToolName = "manage_automations"

// manageAutomationsNextRunCount matches the next runs the automation API
// returns.
const manageAutomationsNextRunCount = 5

const manageAutomationsDescription = "Manage the chat owner's automations in this chat's organization. " +
	"Automations are webhook or schedule triggers that send a prompt to an existing chat or start a new chat. " +
	"Actions: list returns the automations without their prompts; get returns one automation with its prompt; " +
	"disable stops an automation from running until its owner enables it again; " +
	"delete removes an automation permanently. get, disable, and delete require automation_id. " +
	"When the current turn was started by an automation, only automations that target this chat " +
	"or that created this chat are visible, and delete only removes the automation that started the turn."

// manageAutomationsLaterActions return a clear error instead of an
// unknown-action error until a later version supports them.
var manageAutomationsLaterActions = []string{"create", "update", "enable", "run_now"}

var errManageAutomationsNotFound = xerrors.New("automation not found")

type manageAutomationsArgs struct {
	Action       string `json:"action" enum:"list,get,disable,delete" description:"The action to perform."`
	AutomationID string `json:"automation_id,omitempty" description:"Automation UUID. Required for get, disable, and delete."`
}

// automationTurnTrigger identifies the automation input that reached the
// current turn. An invalid AutomationID means a human reached it.
type automationTurnTrigger struct {
	AutomationID uuid.NullUUID
	InputID      uuid.NullUUID
}

// automationTurnTriggerFromHistory returns the automation input that
// reached the current turn. messages is the chat's full, ordered history:
// the prompt window is not enough because compaction replays pending user
// rows without their automation_id. The turn is the latest user prompt,
// the contiguous user rows before it back to the previous assistant or
// tool row, and any user rows after it. The latest of these rows that
// carries an automation_id is the trigger, so a human message sent right
// after an automation message, with no response in between, also counts
// as reached. That errs on the restrictive side.
func automationTurnTriggerFromHistory(messages []database.ChatMessage) automationTurnTrigger {
	start := lastUserPromptIndex(messages)
	if start == -1 {
		return automationTurnTrigger{}
	}
	for start > 0 {
		row := messages[start-1]
		if !row.Deleted && !row.Compressed && row.Role != database.ChatMessageRoleUser {
			break
		}
		start--
	}
	var trigger automationTurnTrigger
	for _, row := range messages[start:] {
		if row.Deleted || row.Role != database.ChatMessageRoleUser || !row.AutomationID.Valid {
			continue
		}
		trigger = automationTurnTrigger{AutomationID: row.AutomationID, InputID: row.InputID}
	}
	return trigger
}

// manageAutomationsAllowed reports whether chat may use the
// manage_automations tool: a root chat outside plan mode and explore mode
// that is not archived, has the switch on, and whose owner has the
// chat-automations experiment. experimentEnabled is called last, only
// when every other rule holds, because it may read the database.
func manageAutomationsAllowed(chat database.Chat, experimentEnabled func() bool) bool {
	if chat.ParentChatID.Valid || chat.Archived || !chat.ManageAutomationsEnabled || isExploreSubagentMode(chat.Mode) {
		return false
	}
	if chat.PlanMode.Valid && chat.PlanMode.ChatPlanMode == database.ChatPlanModePlan {
		return false
	}
	return experimentEnabled()
}

// manageAutomationsTool returns the manage_automations tool for chatID.
// Offering the tool is not enough to use it: every call reloads the chat
// and checks every rule again, so turning the switch or the experiment off
// takes effect at the next call.
func (p *Server) manageAutomationsTool(chatID uuid.UUID) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		manageAutomationsToolName,
		manageAutomationsDescription,
		func(ctx context.Context, args manageAutomationsArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			result, err := p.runManageAutomations(ctx, chatID, args)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return toolJSONResponse(result), nil
		},
	)
}

func (p *Server) runManageAutomations(ctx context.Context, chatID uuid.UUID, args manageAutomationsArgs) (map[string]any, error) {
	action := strings.TrimSpace(args.Action)
	switch action {
	case "list", "get", "disable", "delete":
	default:
		if slices.Contains(manageAutomationsLaterActions, action) {
			return nil, xerrors.Errorf("action %q is not available in this version", action)
		}
		return nil, xerrors.Errorf("unknown action %q: use list, get, disable, or delete", action)
	}

	// The model sees only fixed messages, so unexpected failures are
	// logged with their cause first.
	logger := p.logger.With(slog.F("chat_id", chatID), slog.F("action", action))

	//nolint:gocritic // The tool reloads its own chat; the owner's permissions apply below.
	chatdCtx := dbauthz.AsChatd(ctx)
	chat, err := p.db.GetChatByID(chatdCtx, chatID)
	if err != nil {
		logger.Warn(ctx, "manage_automations failed to load chat", slog.Error(err))
		return nil, xerrors.New("failed to load this chat")
	}
	if !manageAutomationsAllowed(chat, func() bool {
		return AutomationsEnabled(ctx, p.experimentEvaluator, chat.OwnerID)
	}) {
		return nil, xerrors.New("manage_automations is not available for this chat")
	}

	// Owner and organization always come from the chat, and every read
	// and write runs as the owner.
	owner, err := automationOwnerSubject(ctx, p.db, chat.OwnerID)
	if err != nil {
		// An inactive owner is an expected refusal; anything else is a
		// server fault.
		if errors.Is(err, ErrAutomationOwnerInactive) {
			logger.Debug(ctx, "manage_automations refused for inactive chat owner", slog.Error(err))
		} else {
			logger.Warn(ctx, "manage_automations failed to load chat owner", slog.Error(err))
		}
		return nil, xerrors.New("the chat owner cannot manage automations")
	}
	ownerCtx := dbauthz.As(ctx, owner)

	history, err := p.db.GetChatMessagesByChatID(chatdCtx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	if err != nil {
		logger.Warn(ctx, "manage_automations failed to load chat history", slog.Error(err))
		return nil, xerrors.New("failed to load chat history")
	}
	trigger := automationTurnTriggerFromHistory(history)

	if action == "list" {
		rows, err := p.db.GetChatAutomationsByOrganizationIDAndOwnerID(ownerCtx, database.GetChatAutomationsByOrganizationIDAndOwnerIDParams{
			OrganizationID: chat.OrganizationID,
			OwnerID:        chat.OwnerID,
		})
		if err != nil {
			logger.Warn(ctx, "manage_automations failed to list automations", slog.Error(err))
			return nil, xerrors.New("failed to list automations")
		}
		automations := make([]codersdk.ChatAutomation, 0, len(rows))
		for _, row := range rows {
			if manageAutomationsVisible(chat, trigger, row) {
				// The result stays in the chat, which may be shared, so list
				// leaves prompts out; get returns one when asked.
				view := p.manageAutomationsView(row)
				view.Prompt = ""
				automations = append(automations, view)
			}
		}
		return map[string]any{"automations": automations}, nil
	}

	id, err := uuid.Parse(strings.TrimSpace(args.AutomationID))
	if err != nil {
		return nil, xerrors.New("automation_id must be a valid UUID")
	}
	logger = logger.With(slog.F("automation_id", id))
	row, err := p.db.GetChatAutomationByID(ownerCtx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
			return nil, errManageAutomationsNotFound
		}
		logger.Warn(ctx, "manage_automations failed to load automation", slog.Error(err))
		return nil, xerrors.New("failed to load automation")
	}
	if !manageAutomationsVisible(chat, trigger, row) {
		return nil, errManageAutomationsNotFound
	}

	switch action {
	case "get":
		return map[string]any{"automation": p.manageAutomationsView(row)}, nil
	case "disable":
		disabled := false
		updated, err := p.UpdateAutomation(ownerCtx, chat.OwnerID, row.ID, codersdk.UpdateChatAutomationRequest{Enabled: &disabled})
		if err != nil {
			return nil, manageAutomationsError(ctx, logger, err)
		}
		p.auditManageAutomations(ctx, chat, trigger, database.AuditActionWrite, row, updated)
		return map[string]any{"automation": p.manageAutomationsView(updated)}, nil
	default: // delete
		// Input from one automation must not remove another one for good;
		// disabling it stays possible and is undone by enabling it.
		if trigger.AutomationID.Valid && trigger.AutomationID.UUID != row.ID {
			return nil, xerrors.New("in a turn an automation started, delete only removes that automation; use disable for others")
		}
		if err := p.DeleteAutomation(ownerCtx, row.ID); err != nil {
			return nil, manageAutomationsError(ctx, logger, err)
		}
		p.auditManageAutomations(ctx, chat, trigger, database.AuditActionDelete, row, database.ChatAutomation{})
		return map[string]any{"deleted": true, "automation_id": row.ID.String()}, nil
	}
}

// manageAutomationsVisible reports whether the tool may see row. Rows of
// other owners or organizations are never visible, even when the owner
// could read them as an administrator. In a turn an automation reached,
// only automations that target this chat or that created it are visible.
func manageAutomationsVisible(chat database.Chat, trigger automationTurnTrigger, row database.ChatAutomation) bool {
	if row.OwnerID != chat.OwnerID || row.OrganizationID != chat.OrganizationID {
		return false
	}
	if !trigger.AutomationID.Valid {
		return true
	}
	targetsChat := row.TargetMode == database.ChatAutomationTargetModeExistingChat &&
		row.TargetChatID.Valid && row.TargetChatID.UUID == chat.ID
	createdChat := chat.AutomationID.Valid && chat.AutomationID.UUID == row.ID
	return targetsChat || createdChat
}

// manageAutomationsView converts row to the API shape, which carries no
// webhook secret or secret hash.
func (p *Server) manageAutomationsView(row database.ChatAutomation) codersdk.ChatAutomation {
	return db2sdk.ChatAutomation(row, AutomationNextRuns(row, p.clock.Now(), manageAutomationsNextRunCount))
}

// manageAutomationsError maps service errors to tool errors. Expected
// refusals are returned as is; any other error is logged with logger
// before it is replaced with a fixed message.
func manageAutomationsError(ctx context.Context, logger slog.Logger, err error) error {
	var validationErr *AutomationValidationError
	switch {
	case errors.Is(err, ErrAutomationNotFound), dbauthz.IsNotAuthorizedError(err):
		return errManageAutomationsNotFound
	case errors.Is(err, ErrAutomationOwnerOnly):
		return xerrors.New("only the automation owner can change it")
	case errors.As(err, &validationErr):
		return validationErr
	default:
		logger.Warn(ctx, "manage_automations action failed", slog.Error(err))
		return xerrors.New("failed to update automation")
	}
}

// auditManageAutomations records a tool change to an automation. There is
// no HTTP request, so the entry is attributed to the chat owner, whose
// permissions the change ran with, and names the calling chat and, in a
// turn an automation reached, the triggering automation and input.
func (p *Server) auditManageAutomations(ctx context.Context, chat database.Chat, trigger automationTurnTrigger, action database.AuditAction, oldRow, newRow database.ChatAutomation) {
	if p.chatWorker == nil || p.chatWorker.opts.Auditor == nil {
		return
	}
	auditor := p.chatWorker.opts.Auditor.Load()
	if auditor == nil {
		return
	}
	fields := map[string]string{"chat_id": chat.ID.String()}
	if trigger.AutomationID.Valid {
		fields["automation_id"] = trigger.AutomationID.UUID.String()
	}
	if trigger.InputID.Valid {
		fields["input_id"] = trigger.InputID.UUID.String()
	}
	// Marshaling a map of strings cannot fail.
	raw, _ := json.Marshal(fields)
	status := http.StatusOK
	if action == database.AuditActionDelete {
		status = http.StatusNoContent
	}
	audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.ChatAutomation]{
		Audit:            *auditor,
		Log:              p.logger.With(slog.F("chat_id", chat.ID), slog.F("tool", manageAutomationsToolName)),
		UserID:           chat.OwnerID,
		OrganizationID:   chat.OrganizationID,
		Action:           action,
		Old:              oldRow,
		New:              newRow,
		Status:           status,
		AdditionalFields: raw,
	})
}
