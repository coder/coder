package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi/httpapiconstraints"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

var Validate *validator.Validate

// This init is used to create a validator and register validation-specific
// functionality for the HTTP API.
//
// A single validator instance is used, because it caches struct parsing.
func init() {
	Validate = validator.New()
	Validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	nameValidator := func(fl validator.FieldLevel) bool {
		f := fl.Field().Interface()
		str, ok := f.(string)
		if !ok {
			return false
		}
		valid := codersdk.NameValid(str)
		return valid == nil
	}
	for _, tag := range []string{"username", "organization_name", "template_name", "workspace_name", "oauth2_app_name"} {
		err := Validate.RegisterValidation(tag, nameValidator)
		if err != nil {
			panic(err)
		}
	}

	displayNameValidator := func(fl validator.FieldLevel) bool {
		f := fl.Field().Interface()
		str, ok := f.(string)
		if !ok {
			return false
		}
		valid := codersdk.DisplayNameValid(str)
		return valid == nil
	}
	for _, displayNameTag := range []string{"organization_display_name", "template_display_name", "group_display_name"} {
		err := Validate.RegisterValidation(displayNameTag, displayNameValidator)
		if err != nil {
			panic(err)
		}
	}

	templateVersionNameValidator := func(fl validator.FieldLevel) bool {
		f := fl.Field().Interface()
		str, ok := f.(string)
		if !ok {
			return false
		}
		valid := codersdk.TemplateVersionNameValid(str)
		return valid == nil
	}
	err := Validate.RegisterValidation("template_version_name", templateVersionNameValidator)
	if err != nil {
		panic(err)
	}

	userRealNameValidator := func(fl validator.FieldLevel) bool {
		f := fl.Field().Interface()
		str, ok := f.(string)
		if !ok {
			return false
		}
		valid := codersdk.UserRealNameValid(str)
		return valid == nil
	}
	err = Validate.RegisterValidation("user_real_name", userRealNameValidator)
	if err != nil {
		panic(err)
	}

	groupNameValidator := func(fl validator.FieldLevel) bool {
		f := fl.Field().Interface()
		str, ok := f.(string)
		if !ok {
			return false
		}
		valid := codersdk.GroupNameValid(str)
		return valid == nil
	}
	err = Validate.RegisterValidation("group_name", groupNameValidator)
	if err != nil {
		panic(err)
	}
}

// Is404Error returns true if the given error should return a 404 status code.
// Both actual 404s and unauthorized errors should return 404s to not leak
// information about the existence of resources.
func Is404Error(err error) bool {
	if err == nil {
		return false
	}

	// This tests for dbauthz.IsNotAuthorizedError and rbac.IsUnauthorizedError.
	if IsUnauthorizedError(err) {
		return true
	}
	return xerrors.Is(err, sql.ErrNoRows)
}

func IsUnauthorizedError(err error) bool {
	if err == nil {
		return false
	}

	// This tests for dbauthz.IsNotAuthorizedError and rbac.IsUnauthorizedError.
	var unauthorized httpapiconstraints.IsUnauthorizedError
	if errors.As(err, &unauthorized) && unauthorized.IsUnauthorized() {
		return true
	}
	return false
}

// Convenience error functions don't take contexts since their responses are
// static, it doesn't make much sense to trace them.

var ResourceNotFoundResponse = codersdk.Response{Message: "Resource not found or you do not have access to this resource"}

// ResourceNotFound is intentionally vague. All 404 responses should be identical
// to prevent leaking existence of resources.
func ResourceNotFound(rw http.ResponseWriter) {
	Write(context.Background(), rw, http.StatusNotFound, ResourceNotFoundResponse)
}

var ResourceForbiddenResponse = codersdk.Response{
	Message: "Forbidden.",
	Detail:  "You don't have permission to view this content. If you believe this is a mistake, please contact your administrator or try signing in with different credentials.",
}

func Forbidden(rw http.ResponseWriter) {
	Write(context.Background(), rw, http.StatusForbidden, ResourceForbiddenResponse)
}

func InternalServerError(rw http.ResponseWriter, err error) {
	var details string
	if err != nil {
		details = err.Error()
	}

	Write(context.Background(), rw, http.StatusInternalServerError, codersdk.Response{
		Message: "An internal server error occurred.",
		Detail:  details,
	})
}

func RouteNotFound(rw http.ResponseWriter) {
	Write(context.Background(), rw, http.StatusNotFound, codersdk.Response{
		Message: "Route not found.",
	})
}

// Write outputs a standardized format to an HTTP response body. ctx is used for
// tracing and can be nil for tracing to be disabled. Tracing this function is
// helpful because JSON marshaling can sometimes take a non-insignificant amount
// of time, and could help us catch outliers. Additionally, we can enrich span
// data a bit more since we have access to the actual interface{} we're
// marshaling, such as the number of elements in an array, which could help us
// spot routes that need to be paginated.
func Write(ctx context.Context, rw http.ResponseWriter, status int, response interface{}) {
	// Pretty up JSON when testing.
	if flag.Lookup("test.v") != nil {
		WriteIndent(ctx, rw, status, response)
		return
	}

	_, span := tracing.StartSpan(ctx)
	defer span.End()

	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	rw.WriteHeader(status)

	enc := json.NewEncoder(rw)
	enc.SetEscapeHTML(true)

	// We can't really do much about these errors, it's probably due to a
	// dropped connection.
	_ = enc.Encode(response)
}

func WriteIndent(ctx context.Context, rw http.ResponseWriter, status int, response interface{}) {
	_, span := tracing.StartSpan(ctx)
	defer span.End()

	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	rw.WriteHeader(status)

	enc := json.NewEncoder(rw)
	enc.SetEscapeHTML(true)
	enc.SetIndent("", "\t")

	// We can't really do much about these errors, it's probably due to a
	// dropped connection.
	_ = enc.Encode(response)
}

// Read decodes JSON from the HTTP request into the value provided. It uses
// go-validator to validate the incoming request body. ctx is used for tracing
// and can be nil. Although tracing this function isn't likely too helpful, it
// was done to be consistent with Write.
func Read(ctx context.Context, rw http.ResponseWriter, r *http.Request, value interface{}) bool {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	err := json.NewDecoder(r.Body).Decode(value)
	if err != nil {
		Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return false
	}
	err = Validate.Struct(value)
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		apiErrors := make([]codersdk.ValidationError, 0, len(validationErrors))
		for _, validationError := range validationErrors {
			apiErrors = append(apiErrors, codersdk.ValidationError{
				Field:  validationError.Field(),
				Detail: fmt.Sprintf("Validation failed for tag %q with value: \"%v\"", validationError.Tag(), validationError.Value()),
			})
		}
		Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Validation failed.",
			Validations: apiErrors,
		})
		return false
	}
	if err != nil {
		Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error validating request body payload.",
			Detail:  err.Error(),
		})
		return false
	}
	return true
}

const websocketCloseMaxLen = 123

// WebsocketCloseSprintf formats a websocket close message and ensures it is
// truncated to the maximum allowed length.
func WebsocketCloseSprintf(format string, vars ...any) string {
	msg := fmt.Sprintf(format, vars...)

	// Cap msg length at 123 bytes. coder/websocket only allows close messages
	// of this length.
	if len(msg) > websocketCloseMaxLen {
		// Trim the string to 123 bytes. If we accidentally cut in the middle of
		// a UTF-8 character, remove it from the string.
		return strings.ToValidUTF8(msg[:websocketCloseMaxLen], "")
	}

	return msg
}

// ServerSentEventSender establishes a Server-Sent Event connection and allows
// the consumer to send messages to the client.
//
// As much as possible, this function should be avoided in favor of using the
// OneWayWebSocket function. See OneWayWebSocket for more context.
func ServerSentEventSender(rw http.ResponseWriter, r *http.Request) (
	sendEvent func(ctx context.Context, sse codersdk.ServerSentEvent) error,
	closed chan struct{},
	err error,
) {
	h := rw.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")

	f, ok := rw.(http.Flusher)
	if !ok {
		panic("http.ResponseWriter is not http.Flusher")
	}

	closed = make(chan struct{})
	type sseEvent struct {
		payload []byte
		errC    chan error
	}
	eventC := make(chan sseEvent)

	// Synchronized handling of events (no guarantee of order).
	go func() {
		defer close(closed)

		// Send a heartbeat every 15 seconds to avoid the connection being killed.
		ticker := time.NewTicker(time.Second * 15)
		defer ticker.Stop()

		for {
			var event sseEvent

			select {
			case <-r.Context().Done():
				return
			case event = <-eventC:
			case <-ticker.C:
				event = sseEvent{
					payload: []byte(fmt.Sprintf("event: %s\n\n", codersdk.ServerSentEventTypePing)),
				}
			}

			_, err := rw.Write(event.payload)
			if event.errC != nil {
				event.errC <- err
			}
			if err != nil {
				return
			}
			f.Flush()
		}
	}()

	sendEvent = func(ctx context.Context, sse codersdk.ServerSentEvent) error {
		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)

		_, err := buf.WriteString(fmt.Sprintf("event: %s\n", sse.Type))
		if err != nil {
			return err
		}

		if sse.Data != nil {
			_, err = buf.WriteString("data: ")
			if err != nil {
				return err
			}
			err = enc.Encode(sse.Data)
			if err != nil {
				return err
			}
		}

		err = buf.WriteByte('\n')
		if err != nil {
			return err
		}

		event := sseEvent{
			payload: buf.Bytes(),
			errC:    make(chan error, 1), // Buffered to prevent deadlock.
		}

		select {
		case <-r.Context().Done():
			return r.Context().Err()
		case <-ctx.Done():
			return ctx.Err()
		case <-closed:
			return xerrors.New("server sent event sender closed")
		case eventC <- event:
			// Re-check closure signals after sending the event to allow
			// for early exit. We don't check closed here because it
			// can't happen while processing the event.
			select {
			case <-r.Context().Done():
				return r.Context().Err()
			case <-ctx.Done():
				return ctx.Err()
			case err := <-event.errC:
				return err
			}
		}
	}

	return sendEvent, closed, nil
}

// OneWayWebSocket establishes a new WebSocket connection that enforces one-way
// communication from the server to the client.
//
// We must use an approach like this instead of Server-Sent Events for the
// browser, because on HTTP/1.1 connections, browsers are locked to no more than
// six HTTP connections for a domain total, across all tabs. If a user were to
// open a workspace in multiple tabs, the entire UI can start to lock up.
// WebSockets have no such limitation, no matter what HTTP protocol was used to
// establish the connection.
func OneWayWebSocket[JsonSerializable any](rw http.ResponseWriter, r *http.Request) (
	sendEvent func(event JsonSerializable) error,
	closed chan struct{},
	err error,
) {
	ctx, cancel := context.WithCancel(r.Context())
	r = r.WithContext(ctx)
	socket, err := websocket.Accept(rw, r, nil)
	if err != nil {
		cancel()
		return nil, nil, xerrors.Errorf("cannot establish connection: %w", err)
	}
	go Heartbeat(ctx, socket)

	type SocketError struct {
		Code   websocket.StatusCode
		Reason string
	}
	eventC := make(chan JsonSerializable)
	socketErrC := make(chan SocketError, 1)
	closed = make(chan struct{})
	go func() {
		defer cancel()
		defer close(closed)

		for {
			select {
			case event := <-eventC:
				err := wsjson.Write(ctx, socket, event)
				if err == nil {
					continue
				}
				_ = socket.Close(websocket.StatusInternalError, "Unable to send newest message")
			case err := <-socketErrC:
				_ = socket.Close(err.Code, err.Reason)
			case <-ctx.Done():
				_ = socket.Close(websocket.StatusNormalClosure, "Connection closed")
			}
			return
		}
	}()

	// We have some tools in the UI code to help enforce one-way WebSocket
	// connections, but there's still the possibility that the client could send
	// a message when it's not supposed to. If that happens, the client likely
	// forgot to use those tools, and communication probably can't be trusted.
	// Better to just close the socket and force the UI to fix its mess
	go func() {
		_, _, err := socket.Read(ctx)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			socketErrC <- SocketError{
				Code:   websocket.StatusInternalError,
				Reason: "Unable to process invalid message from client",
			}
			return
		}
		socketErrC <- SocketError{
			Code:   websocket.StatusProtocolError,
			Reason: "Clients cannot send messages for one-way WebSockets",
		}
	}()

	sendEvent = func(event JsonSerializable) error {
		select {
		case eventC <- event:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}

	return sendEvent, closed, nil
}
