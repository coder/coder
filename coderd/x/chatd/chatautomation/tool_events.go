package chatautomation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

// firstPollFloor and firstPollCeiling bound when the poll loop first polls a
// new subscription, after the poll that subscribe makes.
const (
	firstPollFloor   = 30 * time.Second
	firstPollCeiling = time.Hour
)

// ErrMCPAuthRequired means that the chat owner has no usable credentials for
// an MCP server and must connect to it again.
var ErrMCPAuthRequired = xerrors.New("the chat owner must connect to the MCP server again")

var (
	errNoEvents         = xerrors.New("MCP events are not available in this chat")
	errUnknownMCPServer = xerrors.New("this chat has no MCP server with that slug that supports events")
	errUnknownEvent     = xerrors.New("the MCP server offers no event with that name in a delivery mode that this deployment receives; run list_event_types")
)

// EventSource reaches the MCP servers of the tool's chat as the chat owner.
type EventSource interface {
	// Servers returns the MCP server configs that the chat uses.
	Servers(ctx context.Context) ([]database.MCPServerConfig, error)
	// Open opens an events session to cfg. The caller closes it.
	Open(ctx context.Context, cfg database.MCPServerConfig) (*mcpclient.EventsSession, error)
	// Unsubscribe ends a webhook subscription, best effort.
	Unsubscribe(ctx context.Context, owner uuid.UUID, sub database.ChatAutomationMCPSubscription)
}

type toolEventServer struct {
	MCPServer string                `json:"mcp_server"`
	Events    []mcpclient.EventType `json:"events,omitempty"`
	Error     string                `json:"error,omitempty"`
}

// listEventTypes lists the event types of the chat's MCP servers that support
// events, or of the one named by slug.
func listEventTypes(ctx context.Context, options ToolOptions, slug string) (map[string][]toolEventServer, error) {
	servers, err := eventServers(ctx, options, slug)
	if err != nil {
		return nil, err
	}
	out := []toolEventServer{}
	for _, cfg := range servers {
		entry := toolEventServer{MCPServer: cfg.Slug}
		err := withEvents(ctx, options, cfg, func(session *mcpclient.EventsSession) (err error) {
			entry.Events, err = session.List(ctx)
			return err
		})
		switch {
		case errors.Is(err, mcpclient.ErrEventsUnsupported):
			continue
		case errors.Is(err, ErrMCPAuthRequired):
			entry.Error = "The chat owner must connect to this MCP server first."
		case err != nil:
			options.Logger.Warn(ctx, "list MCP event types", slog.F("server_slug", cfg.Slug), slog.Error(err))
			entry.Error = "The MCP server did not answer."
		}
		out = append(out, entry)
	}
	return map[string][]toolEventServer{"mcp_servers": out}, nil
}

// subscribe subscribes the tool's chat to an event in the first mode of
// deliveryOrder that the event type offers. A webhook subscribe that the
// server cannot deliver to falls back to the next mode.
func subscribe(ctx context.Context, options ToolOptions, args toolArgs, now time.Time) (toolAutomation, error) {
	rejected := validateNameAndPrompt(args.Name, args.Prompt)
	if args.MCPServer == "" {
		rejected.add("mcp_server", "Required.")
	}
	arguments, ok := args.Arguments.(map[string]any)
	if args.Arguments != nil && !ok {
		rejected.add("arguments", "Must be a JSON object.")
	}
	if len(rejected) > 0 {
		return toolAutomation{}, &InvalidRequestError{Validations: rejected}
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return toolAutomation{}, xerrors.Errorf("encode arguments: %w", err)
	}
	servers, err := eventServers(ctx, options, args.MCPServer)
	if err != nil {
		return toolAutomation{}, err
	}
	cfg := servers[0]
	var (
		created database.ChatAutomation
		sub     database.ChatAutomationMCPSubscription
	)
	err = withEvents(ctx, options, cfg, func(session *mcpclient.EventsSession) error {
		types, err := session.List(ctx)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(types, func(t mcpclient.EventType) bool { return t.Name == args.Event })
		if i < 0 {
			return errUnknownEvent
		}
		for _, mode := range deliveryModes(types[i].Delivery, options.AccessURL) {
			if mode == mcpclient.DeliveryWebhook {
				created, sub, err = subscribeWebhook(ctx, options, session, cfg, args, encoded)
				var eventsErr *mcpclient.EventsError
				if errors.As(err, &eventsErr) && eventsErr.Code == codeCallbackEndpointError {
					options.Logger.Warn(ctx, "mcp server cannot deliver to the webhook url, polling instead", slog.F("server_slug", cfg.Slug), slog.Error(err))
					continue
				}
				return refusal(err)
			}
			created, sub, err = subscribePoll(ctx, options, session, cfg, args, arguments, encoded, now)
			return refusal(err)
		}
		return errUnknownEvent
	})
	switch {
	case errors.Is(err, mcpclient.ErrEventsUnsupported):
		return toolAutomation{}, errUnknownMCPServer
	case err != nil:
		return toolAutomation{}, err
	}
	options.Audit(ctx, database.AuditActionCreate, database.ChatAutomation{}, created)
	refs := db2sdk.ChatAutomationRefs{Subscriptions: map[uuid.UUID]database.ChatAutomationMCPSubscription{created.ID: sub}}
	return toToolAutomation(created, refs, now), nil
}

// subscribePoll polls once from now, which makes the server check the
// arguments, and then creates the automation and its poll subscription with
// the returned cursor in one transaction.
func subscribePoll(ctx context.Context, options ToolOptions, session *mcpclient.EventsSession, cfg database.MCPServerConfig, args toolArgs, arguments map[string]any, encoded json.RawMessage, now time.Time) (database.ChatAutomation, database.ChatAutomationMCPSubscription, error) {
	poll, err := session.Poll(ctx, mcpclient.PollParams{Name: args.Event, Arguments: arguments})
	if err != nil {
		return database.ChatAutomation{}, database.ChatAutomationMCPSubscription{}, err
	}
	var sub database.ChatAutomationMCPSubscription
	created, err := createEventAutomation(ctx, options.DB, options.Chat, args.Name, args.Prompt, func(tx database.Store, created database.ChatAutomation) error {
		var cursor sql.NullString
		if poll.Cursor != nil {
			cursor = sql.NullString{String: *poll.Cursor, Valid: true}
		}
		sub, err = tx.InsertChatAutomationMCPSubscription(ctx, database.InsertChatAutomationMCPSubscriptionParams{
			AutomationID:      created.ID,
			MCPServerConfigID: cfg.ID,
			EventName:         args.Event,
			Arguments:         encoded,
			Delivery:          database.ChatAutomationEventDeliveryPoll,
			State:             database.ChatAutomationSubscriptionStateActive,
			Cursor:            cursor,
			NextPollAt:        sql.NullTime{Time: now.Add(min(max(time.Duration(poll.NextPollMs)*time.Millisecond, firstPollFloor), firstPollCeiling)), Valid: true},
			RenewAt:           sql.NullTime{},
		})
		if err != nil {
			return xerrors.Errorf("insert subscription: %w", err)
		}
		return nil
	})
	return created, sub, err
}

// refusal turns an events error into a validation error that helps the agent;
// the server checks the arguments.
func refusal(err error) error {
	var eventsErr *mcpclient.EventsError
	if !errors.As(err, &eventsErr) {
		return err
	}
	field := "event"
	if eventsErr.Code == jsonrpc.CodeInvalidParams {
		field = "arguments"
	}
	return &InvalidRequestError{Validations: []codersdk.ValidationError{{
		Field: field, Detail: fmt.Sprintf("The MCP server refused the subscription: %d %.300s", eventsErr.Code, eventsErr.Message),
	}}}
}

// eventServers returns the chat's MCP servers that an events session can
// reach, only the one named by slug when it is set.
func eventServers(ctx context.Context, options ToolOptions, slug string) ([]database.MCPServerConfig, error) {
	if options.Events == nil {
		return nil, errNoEvents
	}
	configs, err := options.Events.Servers(ctx)
	if err != nil {
		return nil, xerrors.Errorf("get chat MCP servers: %w", err)
	}
	configs = slices.DeleteFunc(configs, func(cfg database.MCPServerConfig) bool {
		// Events sessions refuse user_oidc servers.
		return cfg.AuthType == "user_oidc" || (slug != "" && cfg.Slug != slug)
	})
	if slug != "" && len(configs) == 0 {
		return nil, errUnknownMCPServer
	}
	return configs, nil
}

// withEvents runs fn with an events session to cfg. It returns
// mcpclient.ErrEventsUnsupported for a server without the extension.
func withEvents(ctx context.Context, options ToolOptions, cfg database.MCPServerConfig, fn func(*mcpclient.EventsSession) error) error {
	session, err := options.Events.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer session.Close()
	if !session.Supported() {
		return mcpclient.ErrEventsUnsupported
	}
	return fn(session)
}
