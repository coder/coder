package chatautomation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// ToolName is the name of the agent tool that manages the heartbeats of its
// chat.
const ToolName = "manage_automations"

const (
	toolActionCreate  = "create"
	toolActionList    = "list"
	toolActionDisable = "disable"
	toolActionDelete  = "delete"
)

var (
	// errNotInThisChat answers for an automation that does not exist, one
	// that the chat owner cannot read and one that targets another chat
	// alike, so the tool never reveals another chat's automation.
	errNotInThisChat = xerrors.New("no automation with this ID targets this chat")
	errUnknownAction = xerrors.New("unknown action: the actions are create, list, disable and delete")
)

// AuditFunc records a change that the tool committed. before is zero for a
// create and after is zero for a delete.
type AuditFunc func(ctx context.Context, action database.AuditAction, before, after database.ChatAutomation)

// ToolOptions configures the tool of one chat.
type ToolOptions struct {
	DB database.Store
	// Chat holds the tool. The tool acts as its owner and reaches only the
	// automations that target it.
	Chat   database.Chat
	Audit  AuditFunc
	Logger slog.Logger
	Clock  quartz.Clock
}

type toolArgs struct {
	Action           string `json:"action" enum:"create,list,disable,delete" description:"The action to run."`
	AutomationID     string `json:"automation_id,omitempty" description:"The heartbeat's ID. Required for disable and delete."`
	Name             string `json:"name,omitempty" description:"A short name for the heartbeat. Required for create."`
	Prompt           string `json:"prompt,omitempty" description:"The message that each run sends to this chat. Required for create."`
	ScheduleCron     string `json:"schedule_cron,omitempty" description:"A five-field cron expression (minute, hour, day of month, month, day of week). Runs at most every 5 minutes. Required for create."`
	ScheduleTimeZone string `json:"schedule_time_zone,omitempty" description:"The IANA time zone of schedule_cron, such as UTC or Europe/Berlin. Required for create."`
}

// toolAutomation is an automation in a tool result.
type toolAutomation struct {
	ID               uuid.UUID                       `json:"id"`
	Name             string                          `json:"name"`
	Enabled          bool                            `json:"enabled"`
	Prompt           string                          `json:"prompt"`
	ScheduleCron     string                          `json:"schedule_cron"`
	ScheduleTimeZone string                          `json:"schedule_time_zone"`
	NextRunTimes     []time.Time                     `json:"next_run_times"`
	LastRun          *codersdk.ChatAutomationLastRun `json:"last_run,omitempty"`
}

// Tool returns the manage_automations tool of a chat. Each call loads the
// chat owner's permissions and acts with them, so the tool has no permission
// rules of its own beyond keeping to its chat.
func Tool(options ToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ToolName,
		"Manage the heartbeats of this chat. A heartbeat sends a saved prompt to this chat on a schedule, as the chat owner, "+
			"and is skipped while the chat is busy. Each run's message names the heartbeat's ID. "+
			"create starts a heartbeat. list shows the heartbeats of this chat. disable stops one and delete removes one, by automation_id.",
		func(ctx context.Context, args toolArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			result, err := runTool(ctx, options, args)
			if err != nil {
				if text, ok := toolErrorText(err); ok {
					return fantasy.NewTextErrorResponse(text), nil
				}
				options.Logger.Error(ctx, "manage_automations failed", slog.F("action", args.Action), slog.F("chat_id", options.Chat.ID), slog.Error(err))
				return fantasy.NewTextErrorResponse("The heartbeats are unavailable. Try again later."), nil
			}
			data, err := json.Marshal(result)
			if err != nil {
				return fantasy.NewTextErrorResponse("The heartbeats are unavailable. Try again later."), nil
			}
			return fantasy.NewTextResponse(string(data)), nil
		},
	)
}

func runTool(ctx context.Context, options ToolOptions, args toolArgs) (any, error) {
	owner, _, err := httpmw.UserRBACSubject(ctx, options.DB, options.Chat.OwnerID, rbac.ScopeAll)
	if err != nil {
		return nil, xerrors.Errorf("load chat owner: %w", err)
	}
	ctx = dbauthz.As(ctx, owner)
	now := dbtime.Time(options.Clock.Now())
	switch args.Action {
	case toolActionCreate:
		return createHeartbeat(ctx, options, args, now)
	case toolActionList:
		return listHeartbeats(ctx, options, now)
	case toolActionDisable:
		automation, err := heartbeatOfChat(ctx, options, args.AutomationID)
		if err != nil {
			return nil, err
		}
		before, after, err := Update(ctx, options.DB, options.Chat.OrganizationID, automation.ID, codersdk.UpdateChatAutomationRequest{Enabled: new(false)}, now)
		if err != nil {
			return nil, err
		}
		if before.Enabled {
			options.Audit(ctx, database.AuditActionWrite, before, after)
		}
		return toToolAutomation(after, now), nil
	case toolActionDelete:
		automation, err := heartbeatOfChat(ctx, options, args.AutomationID)
		if err != nil {
			return nil, err
		}
		deleted, err := Delete(ctx, options.DB, options.Chat.OrganizationID, automation.ID)
		if err != nil {
			return nil, err
		}
		options.Audit(ctx, database.AuditActionDelete, deleted, database.ChatAutomation{})
		return map[string]uuid.UUID{"deleted_automation_id": deleted.ID}, nil
	}
	return nil, errUnknownAction
}

// createHeartbeat creates an enabled schedule that targets the tool's chat
// and names it as its creator.
func createHeartbeat(ctx context.Context, options ToolOptions, args toolArgs, now time.Time) (toolAutomation, error) {
	chatID := options.Chat.ID
	created, err := createAutomation(ctx, options.DB, options.Chat.OrganizationID, uuid.NullUUID{UUID: chatID, Valid: true}, codersdk.CreateChatAutomationRequest{
		Name:             args.Name,
		TargetMode:       codersdk.ChatAutomationTargetModeExistingChat,
		TargetChatID:     &chatID,
		ScheduleCron:     args.ScheduleCron,
		ScheduleTimeZone: args.ScheduleTimeZone,
		Prompt:           args.Prompt,
	}, now)
	if err != nil {
		return toolAutomation{}, err
	}
	options.Audit(ctx, database.AuditActionCreate, database.ChatAutomation{}, created)
	return toToolAutomation(created, now), nil
}

// listHeartbeats lists the chat owner's automations that target the tool's
// chat. An owner has at most MaxAutomationsPerOwner, so the filter runs here.
func listHeartbeats(ctx context.Context, options ToolOptions, now time.Time) (map[string][]toolAutomation, error) {
	rows, err := options.DB.GetChatAutomationsByOrganizationID(ctx, database.GetChatAutomationsByOrganizationIDParams{
		OrganizationID: options.Chat.OrganizationID,
		OwnerID:        options.Chat.OwnerID,
	})
	if err != nil {
		return nil, xerrors.Errorf("list chat automations: %w", err)
	}
	heartbeats := []toolAutomation{}
	for _, row := range rows {
		if row.TargetChatID == (uuid.NullUUID{UUID: options.Chat.ID, Valid: true}) {
			heartbeats = append(heartbeats, toToolAutomation(row, now))
		}
	}
	return map[string][]toolAutomation{"heartbeats": heartbeats}, nil
}

// heartbeatOfChat reads an automation as the chat owner and returns it only
// when it targets the tool's chat. Disable and delete read it again under the
// automation lock; only a chat delete can change the target, and only to
// NULL.
func heartbeatOfChat(ctx context.Context, options ToolOptions, rawID string) (database.ChatAutomation, error) {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return database.ChatAutomation{}, errNotInThisChat
	}
	automation, err := options.DB.GetChatAutomationByID(ctx, id)
	if httpapi.Is404Error(err) {
		return database.ChatAutomation{}, errNotInThisChat
	}
	if err != nil {
		return database.ChatAutomation{}, xerrors.Errorf("get chat automation: %w", err)
	}
	if automation.TargetChatID != (uuid.NullUUID{UUID: options.Chat.ID, Valid: true}) {
		return database.ChatAutomation{}, errNotInThisChat
	}
	return automation, nil
}

func toToolAutomation(a database.ChatAutomation, now time.Time) toolAutomation {
	sdk := db2sdk.ChatAutomation(a, db2sdk.ChatAutomationRefs{}, now)
	return toolAutomation{
		ID:               a.ID,
		Name:             a.Name,
		Enabled:          a.Enabled,
		Prompt:           a.Prompt,
		ScheduleCron:     a.ScheduleCron.String,
		ScheduleTimeZone: a.ScheduleTimeZone.String,
		NextRunTimes:     sdk.NextRunTimes,
		LastRun:          sdk.LastRun,
	}
}

// toolErrorText is the text that the model reads for an error that the chat
// can act on. No text quotes the prompt.
func toolErrorText(err error) (string, bool) {
	var invalid *InvalidRequestError
	switch {
	case errors.Is(err, errUnknownAction), errors.Is(err, errNotInThisChat):
		return err.Error(), true
	case errors.As(err, &invalid):
		details := make([]string, len(invalid.Validations))
		for i, v := range invalid.Validations {
			details[i] = v.Field + ": " + v.Detail
		}
		return "The heartbeat is invalid. " + strings.Join(details, " "), true
	case errors.Is(err, ErrLimitReached):
		return fmt.Sprintf("The chat owner has %d automations, the most allowed. Delete one to create another.", MaxAutomationsPerOwner), true
	case errors.Is(err, ErrCreateForbidden), errors.Is(err, ErrTargetChatForbidden),
		errors.Is(err, ErrUpdateForbidden), errors.Is(err, ErrDeleteForbidden):
		return "The chat owner is not allowed to do this.", true
	}
	return "", false
}
