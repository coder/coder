package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/codersdk"
)

const manageAutomationsToolName = "manage_automations"

// manageAutomationsNextRunCount matches the next runs the automation API
// returns.
const manageAutomationsNextRunCount = 5

const manageAutomationsDescription = "Manage the chat owner's automations in this chat's organization. " +
	"Automations are webhook or schedule triggers that send a prompt to an existing chat or start a new chat. " +
	"Actions: list returns the automations without their prompts; get returns one automation with its prompt; create adds an enabled automation; " +
	"update changes only the fields you pass; enable turns an automation on again; " +
	"disable stops an automation from running until it is enabled again; delete removes an automation permanently; " +
	"run_now sends the prompt of an enabled schedule automation now. " +
	"Every action except list and create requires automation_id. kind, target_mode, and webhook_use cannot change after create. " +
	"A heartbeat is a schedule automation with target_mode existing_chat on this chat: create it with kind schedule, " +
	"target_mode existing_chat, a prompt, schedule_cron, and schedule_time_zone, and omit target_chat_id. " +
	"An existing_chat automation can only target this chat. A new_chat automation uses this chat's model config " +
	"unless new_chat_model_config_id names a model config without provider tools such as web search. " +
	"update, enable, and run_now work only on automations that follow these rules. " +
	"Webhook secrets are never returned: the owner rotates the secret in the automations UI to get one. " +
	"When the current turn was started by an automation, create, update, enable, and run_now are refused; " +
	"only automations that target this chat or that created this chat are visible, and delete only removes the automation that started the turn."

var manageAutomationsActions = []string{"list", "get", "create", "update", "enable", "disable", "delete", "run_now"}

// manageAutomationsWriteActions are refused in a turn an automation
// reached, because they can add or widen what automations do.
var manageAutomationsWriteActions = []string{"create", "update", "enable", "run_now"}

var errManageAutomationsNotFound = xerrors.New("automation not found")

// manageAutomationsSecretNotShown replaces a webhook secret the tool does
// not return.
//
//nolint:gosec // Explains that a secret is withheld; it is not a credential.
const manageAutomationsSecretNotShown = "The webhook secret is not shown. The owner can rotate the secret in the automations UI to get a new one."

type manageAutomationsArgs struct {
	Action       string `json:"action" enum:"list,get,create,update,enable,disable,delete,run_now" description:"The action to perform."`
	AutomationID string `json:"automation_id,omitempty" description:"Automation UUID. Required for every action except list and create."`
	// The fields below apply to create and update only. Pointers tell an
	// omitted field from an empty one, so update changes only the fields
	// that are set.
	Name                 *string `json:"name,omitempty" description:"create (required) and update: the automation name."`
	Kind                 *string `json:"kind,omitempty" enum:"webhook,schedule" description:"create only (required): webhook or schedule."`
	TargetMode           *string `json:"target_mode,omitempty" enum:"existing_chat,new_chat" description:"create only (required): send to this chat (existing_chat) or start a new chat per run (new_chat)."`
	TargetChatID         *string `json:"target_chat_id,omitempty" description:"existing_chat only: must be this chat's ID. Defaults to this chat on create."`
	NewChatModelConfigID *string `json:"new_chat_model_config_id,omitempty" description:"new_chat only: model config UUID for new chats. Defaults to this chat's model config on create."`
	ReasoningEffort      *string `json:"reasoning_effort,omitempty" description:"new_chat only: reasoning effort for new chats. On update, an empty value clears it."`
	WhenBusy             *string `json:"when_busy,omitempty" enum:"queue,skip" description:"existing_chat only: queue or skip a run while the chat is busy. Schedules default to skip, webhooks to queue."`
	WebhookUse           *string `json:"webhook_use,omitempty" enum:"single,multi" description:"create only, webhook only: single-use or multi-use secret. Defaults to multi."`
	Prompt               *string `json:"prompt,omitempty" description:"create (required) and update: the prompt each run sends."`
	ScheduleCron         *string `json:"schedule_cron,omitempty" description:"schedule only, required on create: a standard five-field cron expression."`
	ScheduleTimeZone     *string `json:"schedule_time_zone,omitempty" description:"schedule only, required on create: an IANA time zone such as UTC or Europe/Berlin."`
}

// setFields returns the names of the create and update fields that are
// set, in schema order.
func (a manageAutomationsArgs) setFields() []string {
	var set []string
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"name", a.Name},
		{"kind", a.Kind},
		{"target_mode", a.TargetMode},
		{"target_chat_id", a.TargetChatID},
		{"new_chat_model_config_id", a.NewChatModelConfigID},
		{"reasoning_effort", a.ReasoningEffort},
		{"when_busy", a.WhenBusy},
		{"webhook_use", a.WebhookUse},
		{"prompt", a.Prompt},
		{"schedule_cron", a.ScheduleCron},
		{"schedule_time_zone", a.ScheduleTimeZone},
	} {
		if field.value != nil {
			set = append(set, field.name)
		}
	}
	return set
}

// checkFields rejects fields that do not apply to action, so the tool
// never silently drops part of a request.
func (a manageAutomationsArgs) checkFields(action string) error {
	set := a.setFields()
	switch action {
	case "create":
		if strings.TrimSpace(a.AutomationID) != "" {
			return xerrors.New("create does not take automation_id")
		}
	case "update":
		for _, fixed := range []string{"kind", "target_mode", "webhook_use"} {
			if slices.Contains(set, fixed) {
				return xerrors.Errorf("%s cannot be changed after the automation is created", fixed)
			}
		}
		if len(set) == 0 {
			return xerrors.New("update needs at least one field to change")
		}
	default:
		if len(set) > 0 {
			return xerrors.Errorf("%s does not take %s", action, strings.Join(set, ", "))
		}
	}
	return nil
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
	if !slices.Contains(manageAutomationsActions, action) {
		return nil, xerrors.Errorf("unknown action %q: use one of %s", action, strings.Join(manageAutomationsActions, ", "))
	}
	if err := args.checkFields(action); err != nil {
		return nil, err
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
	if trigger.AutomationID.Valid && slices.Contains(manageAutomationsWriteActions, action) {
		return nil, xerrors.Errorf("%s is not available in a turn that an automation started", action)
	}

	switch action {
	case "list":
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
	case "create":
		return p.manageAutomationsCreate(ctx, ownerCtx, chat, trigger, args)
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
	case "update":
		req, err := args.updateRequest()
		if err != nil {
			return nil, err
		}
		return p.manageAutomationsUpdate(ctx, ownerCtx, chat, trigger, action, row, req)
	case "enable":
		enabled := true
		return p.manageAutomationsUpdate(ctx, ownerCtx, chat, trigger, action, row, codersdk.UpdateChatAutomationRequest{Enabled: &enabled})
	case "run_now":
		return p.manageAutomationsRun(ctx, ownerCtx, chat, row)
	case "disable":
		disabled := false
		updated, err := p.UpdateAutomation(ownerCtx, chat.OwnerID, row.ID, codersdk.UpdateChatAutomationRequest{Enabled: &disabled}, nil)
		if err != nil {
			return nil, p.manageAutomationsError(ctx, logger, action, err)
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
			return nil, p.manageAutomationsError(ctx, logger, action, err)
		}
		p.auditManageAutomations(ctx, chat, trigger, database.AuditActionDelete, row, database.ChatAutomation{})
		return map[string]any{"deleted": true, "automation_id": row.ID.String()}, nil
	}
}

// manageAutomationsCreate creates an automation owned by the chat owner in
// the chat's organization and records the calling chat as its creator.
// Omitted targets default to this chat and this chat's model config.
func (p *Server) manageAutomationsCreate(ctx, ownerCtx context.Context, chat database.Chat, trigger automationTurnTrigger, args manageAutomationsArgs) (map[string]any, error) {
	targetChatID, err := parseManageAutomationsUUID("target_chat_id", args.TargetChatID)
	if err != nil {
		return nil, err
	}
	modelConfigID, err := parseManageAutomationsUUID("new_chat_model_config_id", args.NewChatModelConfigID)
	if err != nil {
		return nil, err
	}
	req := codersdk.CreateChatAutomationRequest{
		Name:                 ptr.NilToEmpty(args.Name),
		Kind:                 codersdk.ChatAutomationKind(ptr.NilToEmpty(args.Kind)),
		TargetMode:           codersdk.ChatAutomationTargetMode(ptr.NilToEmpty(args.TargetMode)),
		TargetChatID:         targetChatID,
		NewChatModelConfigID: modelConfigID,
		ReasoningEffort:      args.ReasoningEffort,
		Prompt:               ptr.NilToEmpty(args.Prompt),
		ScheduleCron:         args.ScheduleCron,
		ScheduleTimeZone:     args.ScheduleTimeZone,
	}
	if args.WhenBusy != nil {
		req.WhenBusy = ptr.Ref(codersdk.ChatAutomationWhenBusy(*args.WhenBusy))
	}
	if args.WebhookUse != nil {
		req.WebhookUse = ptr.Ref(codersdk.ChatAutomationWebhookUse(*args.WebhookUse))
	}
	switch req.TargetMode {
	case codersdk.ChatAutomationTargetModeExistingChat:
		if req.TargetChatID == nil {
			req.TargetChatID = &chat.ID
		}
	case codersdk.ChatAutomationTargetModeNewChat:
		if req.NewChatModelConfigID == nil {
			req.NewChatModelConfigID = &chat.LastModelConfigID
		}
	}
	target := automationTarget{mode: database.ChatAutomationTargetMode(req.TargetMode)}.
		with(req.TargetChatID, req.NewChatModelConfigID)
	if err := p.manageAutomationsContained(ownerCtx, p.db, chat, target); err != nil {
		return nil, err
	}

	// The secret is dropped: tool results stay in the chat, which shared
	// readers, the model provider, and compaction can see.
	row, _, err := p.CreateAutomation(ownerCtx, CreateAutomationParams{
		OrganizationID:  chat.OrganizationID,
		OwnerID:         chat.OwnerID,
		CreatedByChatID: uuid.NullUUID{UUID: chat.ID, Valid: true},
		Request:         req,
	})
	if err != nil {
		logger := p.logger.With(slog.F("chat_id", chat.ID), slog.F("action", "create"))
		return nil, p.manageAutomationsError(ctx, logger, "create", err)
	}
	p.auditManageAutomations(ctx, chat, trigger, database.AuditActionCreate, database.ChatAutomation{}, row)
	result := map[string]any{"automation": p.manageAutomationsView(row)}
	if row.Kind == database.ChatAutomationKindWebhook {
		result["webhook_secret_note"] = manageAutomationsSecretNotShown
	}
	return result, nil
}

// updateRequest converts the update fields of a to an update request.
// checkFields has already rejected the fields that cannot change.
func (a manageAutomationsArgs) updateRequest() (codersdk.UpdateChatAutomationRequest, error) {
	targetChatID, err := parseManageAutomationsUUID("target_chat_id", a.TargetChatID)
	if err != nil {
		return codersdk.UpdateChatAutomationRequest{}, err
	}
	modelConfigID, err := parseManageAutomationsUUID("new_chat_model_config_id", a.NewChatModelConfigID)
	if err != nil {
		return codersdk.UpdateChatAutomationRequest{}, err
	}
	req := codersdk.UpdateChatAutomationRequest{
		Name:                 a.Name,
		Prompt:               a.Prompt,
		ScheduleCron:         a.ScheduleCron,
		ScheduleTimeZone:     a.ScheduleTimeZone,
		ReasoningEffort:      a.ReasoningEffort,
		TargetChatID:         targetChatID,
		NewChatModelConfigID: modelConfigID,
	}
	if a.WhenBusy != nil {
		req.WhenBusy = ptr.Ref(codersdk.ChatAutomationWhenBusy(*a.WhenBusy))
	}
	return req, nil
}

// manageAutomationsUpdate applies req to row. The stored row and the row
// as it would be after the update must both be contained, so the tool can
// neither change an automation that already reaches beyond this chat nor
// widen one. The service checks both again on the locked row, so a
// concurrent change by the owner cannot slip past the checks here.
func (p *Server) manageAutomationsUpdate(ctx, ownerCtx context.Context, chat database.Chat, trigger automationTurnTrigger, action string, row database.ChatAutomation, req codersdk.UpdateChatAutomationRequest) (map[string]any, error) {
	before := automationTargetOf(row)
	if err := p.manageAutomationsContained(ownerCtx, p.db, chat, before); err != nil {
		return nil, err
	}
	after := before.with(req.TargetChatID, req.NewChatModelConfigID)
	if err := p.manageAutomationsContained(ownerCtx, p.db, chat, after); err != nil {
		return nil, err
	}
	updated, err := p.UpdateAutomation(ownerCtx, chat.OwnerID, row.ID, req, p.manageAutomationsGuard(ownerCtx, chat))
	if err != nil {
		logger := p.logger.With(slog.F("chat_id", chat.ID), slog.F("action", action), slog.F("automation_id", row.ID))
		return nil, p.manageAutomationsError(ctx, logger, action, err)
	}
	p.auditManageAutomations(ctx, chat, trigger, database.AuditActionWrite, row, updated)
	return map[string]any{"automation": p.manageAutomationsView(updated)}, nil
}

// manageAutomationsRun publishes the prompt of a contained schedule
// automation now. Admission checks containment again on the locked row
// whose input it accepts. Like the run endpoint, it audits only a chat it
// creates, and that entry also names the calling chat.
func (p *Server) manageAutomationsRun(ctx, ownerCtx context.Context, chat database.Chat, row database.ChatAutomation) (map[string]any, error) {
	if err := p.manageAutomationsContained(ownerCtx, p.db, chat, automationTargetOf(row)); err != nil {
		return nil, err
	}
	result, err := p.RunAutomation(ownerCtx, chat.OwnerID, row.ID, p.manageAutomationsGuard(ownerCtx, chat))
	if err != nil {
		logger := p.logger.With(slog.F("chat_id", chat.ID), slog.F("action", "run_now"), slog.F("automation_id", row.ID))
		return nil, p.manageAutomationsError(ctx, logger, "run", err)
	}
	if row.TargetMode == database.ChatAutomationTargetModeNewChat {
		logger := p.logger.With(slog.F("chat_id", chat.ID), slog.F("automation_id", row.ID), slog.F("tool", manageAutomationsToolName))
		p.auditAutomationCreatedChat(ctx, logger, row, result, map[string]string{"created_by_chat_id": chat.ID.String()})
	}
	return map[string]any{"input_id": result.InputID.String(), "chat_id": result.ChatID.String()}, nil
}

// automationTarget is where an automation sends its runs.
type automationTarget struct {
	mode          database.ChatAutomationTargetMode
	chatID        uuid.NullUUID
	modelConfigID uuid.NullUUID
}

func automationTargetOf(row database.ChatAutomation) automationTarget {
	return automationTarget{mode: row.TargetMode, chatID: row.TargetChatID, modelConfigID: row.NewChatModelConfigID}
}

// with returns t with the target fields that are set replaced.
func (t automationTarget) with(chatID, modelConfigID *uuid.UUID) automationTarget {
	if chatID != nil {
		t.chatID = uuid.NullUUID{UUID: *chatID, Valid: true}
	}
	if modelConfigID != nil {
		t.modelConfigID = uuid.NullUUID{UUID: *modelConfigID, Valid: true}
	}
	return t
}

// manageAutomationsContainmentError marks a containment refusal that a
// service guard returned, so the tool reports it unchanged.
type manageAutomationsContainmentError struct {
	err error
}

func (e *manageAutomationsContainmentError) Error() string { return e.err.Error() }

func (e *manageAutomationsContainmentError) Unwrap() error { return e.err }

// manageAutomationsGuard returns the service guard that requires the
// locked automation row to be contained.
func (p *Server) manageAutomationsGuard(ownerCtx context.Context, chat database.Chat) AutomationGuard {
	return func(store database.Store, row database.ChatAutomation) error {
		if err := p.manageAutomationsContained(ownerCtx, store, chat, automationTargetOf(row)); err != nil {
			return &manageAutomationsContainmentError{err: err}
		}
		return nil
	}
}

// manageAutomationsContained reports whether the tool may send runs to
// target: an existing_chat target must be the calling chat, and a new_chat
// target must not get more tools than the calling chat has. Only the model
// config's provider tools can differ, so the config must be the calling
// chat's or have no provider tools. The model config is read through store.
func (*Server) manageAutomationsContained(ownerCtx context.Context, store database.Store, chat database.Chat, target automationTarget) error {
	switch target.mode {
	case database.ChatAutomationTargetModeExistingChat:
		if !target.chatID.Valid || target.chatID.UUID != chat.ID {
			return xerrors.New("an existing_chat automation managed by this tool must target this chat")
		}
		return nil
	case database.ChatAutomationTargetModeNewChat:
		if !target.modelConfigID.Valid {
			return automationFieldError("new_chat_model_config_id", "is required for new_chat automations")
		}
		if target.modelConfigID.UUID == chat.LastModelConfigID {
			return nil
		}
		config, err := store.GetChatModelConfigByID(ownerCtx, target.modelConfigID.UUID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
				return automationFieldError("new_chat_model_config_id", "model config not found")
			}
			return xerrors.New("failed to load model config")
		}
		callConfig, err := parseModelConfigOptions(config.Options)
		if err != nil || len(buildProviderTools(callConfig.ProviderOptions)) > 0 {
			return xerrors.New("a new_chat automation managed by this tool must use this chat's model config or a model config without provider tools such as web search")
		}
		return nil
	default:
		return automationFieldError("target_mode", fmt.Sprintf("must be one of %q", database.AllChatAutomationTargetModeValues()))
	}
}

func parseManageAutomationsUUID(field string, value *string) (*uuid.UUID, error) {
	if value == nil {
		return nil, nil //nolint:nilnil // A nil ID means the field was omitted.
	}
	id, err := uuid.Parse(strings.TrimSpace(*value))
	if err != nil {
		return nil, xerrors.Errorf("%s must be a valid UUID", field)
	}
	return &id, nil
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
	now := p.clock.Now()
	view := db2sdk.ChatAutomation(row, AutomationNextRuns(row, now, manageAutomationsNextRunCount))
	view.ScheduleIntervalBelowMinimum = AutomationScheduleBelowMinimum(row, now, p.chatLimits.MinAutomationScheduleInterval)
	return view
}

// manageAutomationsError maps service errors to tool errors that reveal
// no more than the management API does. verb names the failed operation.
// Any other error is logged with logger, which carries the chat and
// automation fields, before it is replaced with a fixed message.
func (*Server) manageAutomationsError(ctx context.Context, logger slog.Logger, verb string, err error) error {
	var containmentErr *manageAutomationsContainmentError
	var validationErr *AutomationValidationError
	var denied *chathooks.UserPromptDeniedError
	switch {
	case errors.As(err, &containmentErr):
		return containmentErr.err
	case errors.Is(err, ErrAutomationNotFound), dbauthz.IsNotAuthorizedError(err):
		return errManageAutomationsNotFound
	case errors.Is(err, ErrAutomationOwnerOnly):
		return xerrors.New("only the automation owner can change it")
	case errors.As(err, &validationErr):
		return validationErr
	case errors.As(err, &denied):
		return denied
	case errors.Is(err, ErrAutomationLimitReached):
		return err
	case errors.Is(err, ErrAutomationDisabled):
		return xerrors.New("the automation is disabled: enable it before running it")
	case errors.Is(err, ErrAutomationChatBusy):
		return xerrors.New("the target chat is busy, and the automation skips runs while it is busy")
	}
	for _, refusal := range []error{
		ErrAutomationQueueShareFull,
		ErrAutomationTargetUnavailable,
		ErrAutomationModelUnavailable,
		ErrAutomationOwnerInactive,
		ErrAutomationForbidden,
		ErrAutomationsExperimentDisabled,
	} {
		if errors.Is(err, refusal) {
			return refusal
		}
	}
	logger.Warn(ctx, "manage_automations action failed", slog.Error(err))
	return xerrors.Errorf("failed to %s automation", verb)
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
	switch action {
	case database.AuditActionCreate:
		status = http.StatusCreated
	case database.AuditActionDelete:
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
