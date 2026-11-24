package mcpclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/net/http/httpguts"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
)

// EventsExtensionID identifies the MCP Events extension (SEP-3415).
const EventsExtensionID = "io.modelcontextprotocol/events"

const (
	// eventsMinProtocolVersion is the first revision that defines the
	// extension.
	eventsMinProtocolVersion = "2026-07-28"
	// maxEventListPages stops a server that always returns a next cursor.
	maxEventListPages = 100
	// maxSafeHeaderInteger bounds integer x-mcp-header values (the
	// JavaScript safe-integer range).
	maxSafeHeaderInteger = 1<<53 - 1
	// codeHeaderMismatch is the transport's HeaderMismatch error.
	codeHeaderMismatch = -32020
	// maxEventsMessageBytes bounds each message that an SSE stream of an
	// events session carries.
	maxEventsMessageBytes = 1 << 20
)

// DeliveryMode is a way a server delivers an event type.
type DeliveryMode string

const (
	DeliveryPoll    DeliveryMode = "poll"
	DeliveryPush    DeliveryMode = "push"
	DeliveryWebhook DeliveryMode = "webhook"
)

// ErrEventsUnsupported is returned, without a request, by calls on a
// session whose server did not advertise the extension.
var ErrEventsUnsupported = xerrors.New("MCP server does not support the events extension")

// EventsError is a JSON-RPC error answer to an events/* request, such
// as Forbidden (-32024) or HeaderMismatch (-32020).
type EventsError struct {
	Code    int64
	Message string
	Data    json.RawMessage
}

func (e *EventsError) Error() string {
	return fmt.Sprintf("MCP events error %d: %s", e.Code, e.Message)
}

// EventType is one entry of an events/list result.
type EventType struct {
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	Delivery      []DeliveryMode  `json:"delivery"`
	InputSchema   json.RawMessage `json:"inputSchema,omitempty"`
	PayloadSchema json.RawMessage `json:"payloadSchema,omitempty"`
	Meta          mcp.Meta        `json:"_meta,omitempty"`
}

// EventOccurrence is one delivered event. Cursor comes with push and
// webhook deliveries only; a poll result carries it at the top level.
type EventOccurrence struct {
	EventID   string          `json:"eventId"`
	Name      string          `json:"name"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
	Cursor    *string         `json:"cursor,omitempty"`
	Meta      mcp.Meta        `json:"_meta,omitempty"`
}

// PollParams are the params of events/poll. A nil Cursor is sent as
// null, which starts from now.
type PollParams struct {
	mcp.ParamsBase
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Cursor    *string        `json:"cursor"`
	MaxAgeMs  *int64         `json:"maxAgeMs,omitempty"`
	MaxEvents *int64         `json:"maxEvents,omitempty"`
}

// PollResult is the result of events/poll. A nil Cursor means the event
// type does not support replay.
type PollResult struct {
	mcp.ResultBase
	Events     []EventOccurrence `json:"events"`
	Cursor     *string           `json:"cursor"`
	Truncated  bool              `json:"truncated"`
	HasMore    bool              `json:"hasMore"`
	NextPollMs int64             `json:"nextPollMs"`
}

// SubscribeParams are the params of events/subscribe. Subscribe sets
// Delivery.Mode. A nil Cursor is sent as null; a nil TTLMs asks for the
// server's default lifetime.
type SubscribeParams struct {
	mcp.ParamsBase
	Name      string          `json:"name"`
	Arguments map[string]any  `json:"arguments"`
	Delivery  WebhookDelivery `json:"delivery"`
	Cursor    *string         `json:"cursor"`
	MaxAgeMs  *int64          `json:"maxAgeMs,omitempty"`
	TTLMs     *int64          `json:"ttlMs,omitempty"`
}

// WebhookDelivery is the delivery object of events/subscribe. Secret is
// a Standard Webhooks whsec_ secret that the client generates.
type WebhookDelivery struct {
	Mode   DeliveryMode `json:"mode,omitempty"`
	URL    string       `json:"url"`
	Secret string       `json:"secret,omitempty"`
}

// SubscribeResult is the result of events/subscribe. The server sends
// ID in X-MCP-Subscription-Id on each delivery.
type SubscribeResult struct {
	mcp.ResultBase
	ID            string     `json:"id"`
	RefreshBefore *time.Time `json:"refreshBefore"`
	Cursor        *string    `json:"cursor"`
	Truncated     bool       `json:"truncated"`
	// DeliveryStatus may come with a refresh of an existing subscription.
	DeliveryStatus *WebhookDeliveryStatus `json:"deliveryStatus,omitempty"`
}

// WebhookDeliveryStatus says how the deliveries of a webhook subscription
// fare. LastError is a category such as "timeout" or "http_4xx".
type WebhookDeliveryStatus struct {
	LastError *string `json:"lastError"`
}

type listEventsParams struct {
	mcp.ParamsBase
	Cursor string `json:"cursor,omitempty"`
}

type listEventsResult struct {
	mcp.ResultBase
	Events     []EventType `json:"events"`
	NextCursor string      `json:"nextCursor,omitempty"`
}

type unsubscribeParams struct {
	mcp.ParamsBase
	Name      string          `json:"name"`
	Arguments map[string]any  `json:"arguments"`
	Delivery  WebhookDelivery `json:"delivery"`
}

type unsubscribeResult struct{ mcp.ResultBase }

// paramHeader binds the argument at path to the header Mcp-Param-{header}.
type paramHeader struct {
	path   []string
	header string
}

// EventsSession is one MCP session for events/* requests. The caller
// owns it, must Close it, and must not use it from two goroutines.
type EventsSession struct {
	session *mcp.ClientSession
	logger  slog.Logger
	// paramHeaders holds each listed event type's x-mcp-header bindings.
	// It is nil until a List succeeds.
	paramHeaders map[string][]paramHeader
}

// ConnectEvents opens an events session to an org-configured MCP server,
// with the owner's OAuth2 token when the server uses one, and with
// coderHeaders when the server forwards Coder identity headers. It
// refuses user_oidc servers. A server without the extension connects
// too; Supported tells them apart.
func ConnectEvents(
	ctx context.Context,
	logger slog.Logger,
	cfg database.MCPServerConfig,
	tok *database.MCPServerUserToken,
	coderHeaders map[string]string,
	hc *http.Client,
) (*EventsSession, error) {
	srv, err := ServerFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	if srv.UserAuth == UserAuthOIDC {
		return nil, xerrors.New("event sessions do not support user_oidc auth")
	}
	tokens := map[uuid.UUID]database.MCPServerUserToken{}
	if tok != nil {
		// Keyed by the token's own config, so a token for another server
		// is never sent here.
		tokens[tok.MCPServerConfigID] = *tok
	}
	if hc == nil || hc.Transport == nil {
		hc = NewHTTPClient(hc)
	}
	eventsHTTPClient := *hc
	eventsHTTPClient.Transport = &eventsHeaderRoundTripper{base: hc.Transport}
	headers := buildAuthHeaders(ctx, logger, srv, tokens, uuid.Nil, nil, coderHeaders)
	tr, err := createTransport(srv, headers, &eventsHTTPClient, maxEventsMessageBytes)
	if err != nil {
		return nil, xerrors.Errorf("create transport: %w", err)
	}

	caps := &mcp.ClientCapabilities{}
	caps.AddExtension(EventsExtensionID, map[string]any{})
	client := mcp.NewClient(
		&mcp.Implementation{Name: "coder", Version: buildinfo.Version()},
		&mcp.ClientOptions{Capabilities: caps},
	)
	if err := errors.Join(
		mcp.AddSendingCustomMethod[*listEventsParams, *listEventsResult](client, "events/list"),
		mcp.AddSendingCustomMethod[*PollParams, *PollResult](client, "events/poll"),
		mcp.AddSendingCustomMethod[*SubscribeParams, *SubscribeResult](client, "events/subscribe"),
		mcp.AddSendingCustomMethod[*unsubscribeParams, *unsubscribeResult](client, "events/unsubscribe"),
	); err != nil {
		return nil, xerrors.Errorf("register events methods: %w", err)
	}

	// As in connectOne, enforce the budget outside the SDK: Connect can
	// block past its context on a server that stops answering.
	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	type connectResult struct {
		session *mcp.ClientSession
		err     error
	}
	resCh := make(chan connectResult, 1)
	go func() {
		session, err := client.Connect(connectCtx, tr, nil)
		resCh <- connectResult{session: session, err: err}
	}()
	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, xerrors.Errorf("connect: %w", res.err)
		}
		return &EventsSession{session: res.session, logger: logger}, nil
	case <-connectCtx.Done():
		go func() {
			if late := <-resCh; late.session != nil {
				_ = late.session.Close()
			}
		}()
		return nil, xerrors.Errorf("connect: %w", connectCtx.Err())
	}
}

// Close ends the session.
func (s *EventsSession) Close() error {
	return s.session.Close()
}

// Supported reports whether the server advertised the extension on a
// protocol revision that defines it.
func (s *EventsSession) Supported() bool {
	res := s.session.InitializeResult()
	if res == nil || res.Capabilities == nil || res.ProtocolVersion < eventsMinProtocolVersion {
		return false
	}
	_, ok := res.Capabilities.Extensions[EventsExtensionID].(map[string]any)
	return ok
}

// List returns the event types of every page. It drops, with a warning,
// a type whose x-mcp-header annotations break the transport's rules.
func (s *EventsSession) List(ctx context.Context) ([]EventType, error) {
	if !s.Supported() {
		return nil, ErrEventsUnsupported
	}
	var types []EventType
	bindings := make(map[string][]paramHeader)
	params := &listEventsParams{}
	for range maxEventListPages {
		res, err := callEvents[*listEventsResult](ctx, s, "events/list", params, nil)
		if err != nil {
			return nil, err
		}
		for _, et := range res.Events {
			headers, err := paramHeaderBindings(et.InputSchema)
			if err != nil {
				s.logger.Warn(ctx, "dropping MCP event type with an invalid x-mcp-header annotation",
					slog.F("event", et.Name), slog.Error(err))
				continue
			}
			bindings[et.Name] = headers
			types = append(types, et)
		}
		if res.NextCursor == "" {
			s.paramHeaders = bindings
			return types, nil
		}
		params = &listEventsParams{Cursor: res.NextCursor}
	}
	return nil, xerrors.Errorf("events/list returned more than %d pages", maxEventListPages)
}

// Poll sends one events/poll request.
func (s *EventsSession) Poll(ctx context.Context, p PollParams) (PollResult, error) {
	p.Arguments = argumentsObject(p.Arguments)
	res, err := callNamed[*PollResult](ctx, s, "events/poll", p.Name, p.Arguments, &p)
	if err != nil {
		return PollResult{}, err
	}
	return *res, nil
}

// Subscribe creates or refreshes a webhook subscription.
func (s *EventsSession) Subscribe(ctx context.Context, p SubscribeParams) (SubscribeResult, error) {
	p.Arguments = argumentsObject(p.Arguments)
	p.Delivery.Mode = DeliveryWebhook
	res, err := callNamed[*SubscribeResult](ctx, s, "events/subscribe", p.Name, p.Arguments, &p)
	if err != nil {
		return SubscribeResult{}, err
	}
	return *res, nil
}

// Unsubscribe ends the webhook subscription keyed by name, arguments and
// callback URL.
func (s *EventsSession) Unsubscribe(ctx context.Context, name string, args map[string]any, url string) error {
	p := &unsubscribeParams{Name: name, Arguments: argumentsObject(args), Delivery: WebhookDelivery{URL: url}}
	_, err := callNamed[*unsubscribeResult](ctx, s, "events/unsubscribe", name, p.Arguments, p)
	return err
}

// callNamed sends an events/* request for one event with its routing
// headers. On HeaderMismatch it lists the event types again and retries
// once, as the extension asks of clients.
func callNamed[R mcp.Result, T any, P interface {
	*T
	mcp.Params
}](ctx context.Context, s *EventsSession, method, name string, args map[string]any, params P) (R, error) {
	for retried := false; ; retried = true {
		headers, err := s.requestHeaders(ctx, name, args)
		if err != nil {
			var zero R
			return zero, err
		}
		res, err := callEvents[R](ctx, s, method, params, headers)
		var eventsErr *EventsError
		if retried || !errors.As(err, &eventsErr) || eventsErr.Code != codeHeaderMismatch {
			return res, err
		}
		s.paramHeaders = nil
	}
}

// requestHeaders returns Mcp-Name and one Mcp-Param-{Name} per annotated
// argument that has a value. A session that has not listed lists first,
// because only events/list carries the annotations.
func (s *EventsSession) requestHeaders(ctx context.Context, name string, args map[string]any) (map[string]string, error) {
	if s.paramHeaders == nil {
		if _, err := s.List(ctx); err != nil {
			return nil, err
		}
	}
	headers := map[string]string{"Mcp-Name": encodeHeaderValue(name)}
	bindings := s.paramHeaders[name]
	if len(bindings) == 0 {
		return headers, nil
	}
	// Read the values from the JSON that goes on the wire, whatever Go
	// types the caller used.
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, xerrors.Errorf("encode event arguments: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var wire any
	if err := dec.Decode(&wire); err != nil {
		return nil, xerrors.Errorf("decode event arguments: %w", err)
	}
	for _, b := range bindings {
		if value, ok := headerArgument(wire, b.path); ok {
			headers["Mcp-Param-"+b.header] = encodeHeaderValue(value)
		}
	}
	return headers, nil
}

// callEvents sends one events/* request with its routing headers and
// returns a JSON-RPC error answer as an *EventsError.
func callEvents[R mcp.Result, T any, P interface {
	*T
	mcp.Params
}](ctx context.Context, s *EventsSession, method string, params P, headers map[string]string) (R, error) {
	ctx = context.WithValue(ctx, eventsHeadersKey{}, headers)
	res, err := mcp.CallCustomMethod[P, R](ctx, s.session, method, params)
	if err != nil {
		var rpcErr *jsonrpc.Error
		if errors.As(err, &rpcErr) {
			err = &EventsError{Code: rpcErr.Code, Message: rpcErr.Message, Data: rpcErr.Data}
		}
		return res, xerrors.Errorf("%s: %w", method, err)
	}
	return res, nil
}

// argumentsObject sends absent arguments as an empty object.
func argumentsObject(args map[string]any) map[string]any {
	if args == nil {
		return map[string]any{}
	}
	return args
}

// paramHeaderBindings returns the x-mcp-header bindings of an
// inputSchema. An annotation is invalid when a chain of properties keys
// from the root does not reach it, its type is not string, integer or
// boolean, its name is not an HTTP token, or it repeats another
// case-insensitively.
func paramHeaderBindings(schema json.RawMessage) ([]paramHeader, error) {
	if len(schema) == 0 {
		return nil, nil
	}
	var root any
	if err := json.Unmarshal(schema, &root); err != nil {
		return nil, xerrors.Errorf("decode inputSchema: %w", err)
	}
	var bindings []paramHeader
	seen := make(map[string]bool)
	var walk func(node any, path []string, reachable bool) error
	walk = func(node any, path []string, reachable bool) error {
		switch node := node.(type) {
		case []any:
			for _, item := range node {
				if err := walk(item, path, false); err != nil {
					return err
				}
			}
		case map[string]any:
			if annotation, ok := node["x-mcp-header"]; ok {
				header, _ := annotation.(string)
				switch typ, _ := node["type"].(string); {
				case !reachable || len(path) == 0:
					return xerrors.New("x-mcp-header is not reachable through properties alone")
				case typ != "string" && typ != "integer" && typ != "boolean":
					return xerrors.Errorf("x-mcp-header on type %q", typ)
				case !httpguts.ValidHeaderFieldName(header):
					return xerrors.Errorf("x-mcp-header %q is not an HTTP token", header)
				case seen[strings.ToLower(header)]:
					return xerrors.Errorf("x-mcp-header %q repeats", header)
				}
				seen[strings.ToLower(header)] = true
				bindings = append(bindings, paramHeader{path: path, header: header})
			}
			for key, child := range node {
				switch key {
				case "properties":
					props, _ := child.(map[string]any)
					for name, prop := range props {
						if err := walk(prop, append(slices.Clone(path), name), reachable); err != nil {
							return err
						}
					}
				case "default", "const", "enum", "examples":
					// Instance values, not schemas.
				default:
					if err := walk(child, path, false); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := walk(root, nil, true); err != nil {
		return nil, err
	}
	return bindings, nil
}

// headerArgument returns the argument at path as a header value: a
// string as is, a boolean as true or false, a safe integer in decimal.
// Any other value, null or an absent argument gives no header.
func headerArgument(value any, path []string) (string, bool) {
	for _, key := range path {
		obj, ok := value.(map[string]any)
		if !ok {
			return "", false
		}
		if value, ok = obj[key]; !ok {
			return "", false
		}
	}
	switch value := value.(type) {
	case string:
		return value, true
	case bool:
		return strconv.FormatBool(value), true
	case json.Number:
		n, err := strconv.ParseInt(value.String(), 10, 64)
		if err != nil || n > maxSafeHeaderInteger || n < -maxSafeHeaderInteger {
			return "", false
		}
		return strconv.FormatInt(n, 10), true
	}
	return "", false
}

// encodeHeaderValue applies the Streamable HTTP value encoding: a value
// that is not printable ASCII, has leading or trailing whitespace, or
// looks like the sentinel is sent as =?base64?...?=.
func encodeHeaderValue(value string) string {
	plain := strings.Trim(value, " \t") == value &&
		(!strings.HasPrefix(value, "=?base64?") || !strings.HasSuffix(value, "?=")) &&
		!strings.ContainsFunc(value, func(r rune) bool { return r < 0x20 || r > 0x7e })
	if plain {
		return value
	}
	return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
}
