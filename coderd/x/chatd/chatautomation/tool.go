package chatautomation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
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

// ToolName is the name of the agent tool that manages the heartbeats and
// event subscriptions of its chat.
const ToolName = "manage_automations"

const (
	toolActionCreate  = "create"
	toolActionList    = "list"
	toolActionDisable = "disable"
	toolActionDelete  = "delete"
	// toolActionUnsubscribe deletes, like toolActionDelete.
	toolActionUnsubscribe    = "unsubscribe"
	toolActionListEventTypes = "list_event_types"
	toolActionSubscribe      = "subscribe"
)

var (
	// errNotInThisChat answers for an automation that does not exist, one
	// that the chat owner cannot read and one that targets another chat
	// alike, so the tool never reveals another chat's automation.
	errNotInThisChat = xerrors.New("no automation with this ID targets this chat")
	errUnknownAction = xerrors.New("unknown action: the actions are create, list, disable, delete, list_event_types, subscribe and unsubscribe")
)

// AuditFunc records a change that the tool committed. before is zero for a
// create and after is zero for a delete.
type AuditFunc func(ctx context.Context, action database.AuditAction, before, after database.ChatAutomation)

// ToolOptions configures the tool of one chat.
type ToolOptions struct {
	DB database.Store
	// Chat holds the tool. The tool acts as its owner and reaches only the
	// automations that target it.
	Chat  database.Chat
	Audit AuditFunc
	// Events reaches the chat's MCP servers. The event actions are
	// unavailable without it.
	Events EventSource
	// AccessURL is where MCP servers send webhook deliveries. Without a
	// public https one, subscriptions poll.
	AccessURL *url.URL
	Logger    slog.Logger
	Clock     quartz.Clock
}

func (o ToolOptions) unsubscriber() Unsubscriber {
	if o.Events == nil {
		return nil
	}
	return o.Events.Unsubscribe
}

type toolArgs struct {
	Action           string `json:"action" enum:"create,list,disable,delete,list_event_types,subscribe,unsubscribe" description:"The action to run."`
	AutomationID     string `json:"automation_id,omitempty" description:"The automation's ID. Required for disable, delete and unsubscribe."`
	Name             string `json:"name,omitempty" description:"A short name for the automation. Required for create and subscribe."`
	Prompt           string `json:"prompt,omitempty" description:"The message that each run sends to this chat. Required for create and subscribe."`
	ScheduleCron     string `json:"schedule_cron,omitempty" description:"A five-field cron expression (minute, hour, day of month, month, day of week). Runs at most every 5 minutes. Required for create."`
	ScheduleTimeZone string `json:"schedule_time_zone,omitempty" description:"The IANA time zone of schedule_cron, such as UTC or Europe/Berlin. Required for create."`
	MCPServer        string `json:"mcp_server,omitempty" description:"The slug of one of this chat's MCP servers. Required for subscribe. list_event_types lists every server without it."`
	Event            string `json:"event,omitempty" description:"An event name from list_event_types. Required for subscribe."`
	Arguments        any    `json:"arguments,omitempty" description:"The event's arguments: a JSON object that matches its input_schema."`
}

// toolAutomation is an automation in a tool result.
type toolAutomation struct {
	ID               uuid.UUID                       `json:"id"`
	Name             string                          `json:"name"`
	Enabled          bool                            `json:"enabled"`
	Prompt           string                          `json:"prompt"`
	ScheduleCron     string                          `json:"schedule_cron,omitempty"`
	ScheduleTimeZone string                          `json:"schedule_time_zone,omitempty"`
	NextRunTimes     []time.Time                     `json:"next_run_times,omitempty"`
	Event            *codersdk.ChatAutomationEvent   `json:"event,omitempty"`
	LastRun          *codersdk.ChatAutomationLastRun `json:"last_run,omitempty"`
}

// Tool returns the manage_automations tool of a chat. Each call loads the
// chat owner's permissions and acts with them, so the tool has no permission
// rules of its own beyond keeping to its chat.
func Tool(options ToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ToolName,
		"Manage the automations of this chat. A heartbeat sends a saved prompt to this chat on a schedule and is skipped while the chat is busy. "+
			"An event subscription sends the saved prompt with the events that an MCP server reports; events wait while the chat is busy. "+
			"Both run as the chat owner, and each run's message names the automation's ID. "+
			"create starts a heartbeat. list_event_types lists the events of this chat's MCP servers, and subscribe subscribes to one. "+
			"list shows the automations of this chat. disable stops one, and delete or unsubscribe removes one, by automation_id.",
		func(ctx context.Context, args toolArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			result, err := runTool(ctx, options, args)
			if err != nil {
				if text, ok := toolErrorText(err); ok {
					return fantasy.NewTextErrorResponse(text), nil
				}
				options.Logger.Error(ctx, "manage_automations failed", slog.F("action", args.Action), slog.F("chat_id", options.Chat.ID), slog.Error(err))
				return fantasy.NewTextErrorResponse("The automations are unavailable. Try again later."), nil
			}
			data, err := json.Marshal(result)
			if err != nil {
				return fantasy.NewTextErrorResponse("The automations are unavailable. Try again later."), nil
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
		return listAutomations(ctx, options, now)
	case toolActionListEventTypes:
		return listEventTypes(ctx, options, args.MCPServer)
	case toolActionSubscribe:
		return subscribe(ctx, options, args, now)
	case toolActionDisable:
		automation, err := automationOfChat(ctx, options, args.AutomationID)
		if err != nil {
			return nil, err
		}
		before, after, err := Update(ctx, options.DB, options.Chat.OrganizationID, automation.ID, codersdk.UpdateChatAutomationRequest{Enabled: new(false)}, now, options.unsubscriber())
		if err != nil {
			return nil, err
		}
		if before.Enabled {
			options.Audit(ctx, database.AuditActionWrite, before, after)
		}
		return toToolAutomation(after, db2sdk.ChatAutomationRefs{}, now), nil
	case toolActionDelete, toolActionUnsubscribe:
		automation, err := automationOfChat(ctx, options, args.AutomationID)
		if err != nil {
			return nil, err
		}
		deleted, err := Delete(ctx, options.DB, options.Chat.OrganizationID, automation.ID, options.unsubscriber())
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
	return toToolAutomation(created, db2sdk.ChatAutomationRefs{}, now), nil
}

// listAutomations lists the chat owner's automations that target the tool's
// chat. An owner has at most MaxAutomationsPerOwner, so the filter runs here.
func listAutomations(ctx context.Context, options ToolOptions, now time.Time) (map[string][]toolAutomation, error) {
	rows, err := options.DB.GetChatAutomationsByOrganizationID(ctx, database.GetChatAutomationsByOrganizationIDParams{
		OrganizationID: options.Chat.OrganizationID,
		OwnerID:        options.Chat.OwnerID,
	})
	if err != nil {
		return nil, xerrors.Errorf("list chat automations: %w", err)
	}
	var ofChat, events []uuid.UUID
	for _, row := range rows {
		if row.TargetChatID == (uuid.NullUUID{UUID: options.Chat.ID, Valid: true}) {
			ofChat = append(ofChat, row.ID)
			if row.Kind == database.ChatAutomationKindMcpEvent {
				events = append(events, row.ID)
			}
		}
	}
	refs := db2sdk.ChatAutomationRefs{Subscriptions: map[uuid.UUID]database.ChatAutomationMCPSubscription{}}
	if len(events) > 0 {
		//nolint:gocritic // The chat owner read these automations above.
		subs, err := options.DB.GetChatAutomationMCPSubscriptionsByAutomationIDs(dbauthz.AsSystemRestricted(ctx), events)
		if err != nil {
			return nil, xerrors.Errorf("list subscriptions: %w", err)
		}
		for _, sub := range subs {
			refs.Subscriptions[sub.AutomationID] = sub
		}
	}
	automations := []toolAutomation{}
	for _, row := range rows {
		if slices.Contains(ofChat, row.ID) {
			automations = append(automations, toToolAutomation(row, refs, now))
		}
	}
	return map[string][]toolAutomation{"automations": automations}, nil
}

// automationOfChat reads an automation as the chat owner and returns it only
// when it targets the tool's chat. Disable and delete read it again under the
// automation lock; only a chat delete can change the target, and only to
// NULL.
func automationOfChat(ctx context.Context, options ToolOptions, rawID string) (database.ChatAutomation, error) {
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

func toToolAutomation(a database.ChatAutomation, refs db2sdk.ChatAutomationRefs, now time.Time) toolAutomation {
	sdk := db2sdk.ChatAutomation(a, refs, now)
	return toolAutomation{
		ID:               a.ID,
		Name:             a.Name,
		Enabled:          a.Enabled,
		Prompt:           a.Prompt,
		ScheduleCron:     a.ScheduleCron.String,
		ScheduleTimeZone: a.ScheduleTimeZone.String,
		NextRunTimes:     sdk.NextRunTimes,
		Event:            sdk.Event,
		LastRun:          sdk.LastRun,
	}
}

// toolErrorText is the text that the model reads for an error that the chat
// can act on. No text quotes the prompt.
func toolErrorText(err error) (string, bool) {
	var invalid *InvalidRequestError
	switch {
	case errors.Is(err, errUnknownAction), errors.Is(err, errNotInThisChat),
		errors.Is(err, errNoEvents), errors.Is(err, errUnknownMCPServer), errors.Is(err, errUnknownEvent):
		return err.Error(), true
	case errors.Is(err, ErrMCPAuthRequired):
		return "The chat owner must connect to this MCP server first.", true
	case errors.As(err, &invalid):
		details := make([]string, len(invalid.Validations))
		for i, v := range invalid.Validations {
			details[i] = v.Field + ": " + v.Detail
		}
		return "The automation is invalid. " + strings.Join(details, " "), true
	case errors.Is(err, ErrLimitReached):
		return fmt.Sprintf("The chat owner has %d automations, the most allowed. Delete one to create another.", MaxAutomationsPerOwner), true
	case errors.Is(err, ErrCreateForbidden), errors.Is(err, ErrTargetChatForbidden),
		errors.Is(err, ErrUpdateForbidden), errors.Is(err, ErrDeleteForbidden):
		return "The chat owner is not allowed to do this.", true
	}
	return "", false
}
