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
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

// firstPollDelay is when the poll loop first polls a new subscription. The
// subscribe call already polled once to validate the arguments.
const firstPollDelay = 30 * time.Second

// ErrMCPAuthRequired means that the chat owner has no usable credentials for
// an MCP server and must connect to it again.
var ErrMCPAuthRequired = xerrors.New("the chat owner must connect to the MCP server again")

var (
	errNoEvents         = xerrors.New("MCP events are not available in this chat")
	errUnknownMCPServer = xerrors.New("this chat has no MCP server with that slug that supports events")
	errUnknownEvent     = xerrors.New("the MCP server offers no event with that name for poll delivery; run list_event_types")
)

// EventSource reaches the MCP servers of the tool's chat as the chat owner.
type EventSource interface {
	// Servers returns the MCP server configs that the chat uses.
	Servers(ctx context.Context) ([]database.MCPServerConfig, error)
	// Open opens an events session to cfg. The caller closes it.
	Open(ctx context.Context, cfg database.MCPServerConfig) (*mcpclient.EventsSession, error)
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

// subscribe subscribes the tool's chat to an event. It polls once from now,
// which makes the server check the arguments, and then creates the
// automation and its subscription with the returned cursor in one
// transaction.
func subscribe(ctx context.Context, options ToolOptions, args toolArgs, now time.Time) (toolAutomation, error) {
	rejected := validateNameAndPrompt(args.Name, args.Prompt)
	if args.MCPServer == "" {
		rejected.add("mcp_server", "Required.")
	}
	if args.Event == "" {
		rejected.add("event", "Required.")
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
	servers, err := eventServers(ctx, options, args.MCPServer)
	if err != nil {
		return toolAutomation{}, err
	}
	cfg := servers[0]
	var poll mcpclient.PollResult
	err = withEvents(ctx, options, cfg, func(session *mcpclient.EventsSession) error {
		types, err := session.List(ctx)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(types, func(t mcpclient.EventType) bool {
			return t.Name == args.Event && slices.Contains(t.Delivery, mcpclient.DeliveryPoll)
		}) {
			return errUnknownEvent
		}
		poll, err = session.Poll(ctx, mcpclient.PollParams{Name: args.Event, Arguments: arguments})
		// The server checks the arguments; its answer helps the agent fix them.
		var eventsErr *mcpclient.EventsError
		if errors.As(err, &eventsErr) {
			return &InvalidRequestError{Validations: []codersdk.ValidationError{{
				Field: "arguments", Detail: fmt.Sprintf("The MCP server rejected them: %d %.300s", eventsErr.Code, eventsErr.Message),
			}}}
		}
		return err
	})
	switch {
	case errors.Is(err, mcpclient.ErrEventsUnsupported):
		return toolAutomation{}, errUnknownMCPServer
	case err != nil:
		return toolAutomation{}, err
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return toolAutomation{}, xerrors.Errorf("encode arguments: %w", err)
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
			Cursor:            cursor,
			NextPollAt:        sql.NullTime{Time: now.Add(firstPollDelay), Valid: true},
		})
		if err != nil {
			return xerrors.Errorf("insert subscription: %w", err)
		}
		return nil
	})
	if err != nil {
		return toolAutomation{}, err
	}
	options.Audit(ctx, database.AuditActionCreate, database.ChatAutomation{}, created)
	refs := db2sdk.ChatAutomationRefs{Subscriptions: map[uuid.UUID]database.ChatAutomationMCPSubscription{created.ID: sub}}
	return toToolAutomation(created, refs, now), nil
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
