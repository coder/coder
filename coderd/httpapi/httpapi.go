package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/go-playground/validator/v10"

	"github.com/coder/coder/coderd/tracing"
	"github.com/coder/coder/codersdk"
)

var validate *validator.Validate

// This init is used to create a validator and register validation-specific
// functionality for the HTTP API.
//
// A single validator instance is used, because it caches struct parsing.
func init() {
	validate = validator.New()
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
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
		return UsernameValid(str)
	}
	for _, tag := range []string{"username", "template_name", "workspace_name"} {
		err := validate.RegisterValidation(tag, nameValidator)
		if err != nil {
			panic(err)
		}
	}
}

// Convenience error functions don't take contexts since their responses are
// static, it doesn't make much sense to trace them.

// ResourceNotFound is intentionally vague. All 404 responses should be identical
// to prevent leaking existence of resources.
func ResourceNotFound(rw http.ResponseWriter) {
	Write(context.Background(), rw, http.StatusNotFound, codersdk.Response{
		Message: "Resource not found or you do not have access to this resource",
	})
}

func Forbidden(rw http.ResponseWriter) {
	Write(context.Background(), rw, http.StatusForbidden, codersdk.Response{
		Message: "Forbidden.",
	})
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
	_, span := tracing.StartSpan(ctx)
	defer span.End()

	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(true)
	err := enc.Encode(response)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	rw.WriteHeader(status)
	_, err = rw.Write(buf.Bytes())
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
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
	err = validate.Struct(value)
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

	// Cap msg length at 123 bytes. nhooyr/websocket only allows close messages
	// of this length.
	if len(msg) > websocketCloseMaxLen {
		// Trim the string to 123 bytes. If we accidentally cut in the middle of
		// a UTF-8 character, remove it from the string.
		return strings.ToValidUTF8(string(msg[123]), "")
	}

	return msg
}

func ServerSentEventSender(rw http.ResponseWriter, r *http.Request) (func(ctx context.Context, sse codersdk.ServerSentEvent) error, error) {
	var mu sync.Mutex
	h := rw.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")

	f, ok := rw.(http.Flusher)
	if !ok {
		panic("http.ResponseWriter is not http.Flusher")
	}

	// Send a heartbeat every 15 seconds to avoid the connection being killed.
	go func() {
		ticker := time.NewTicker(time.Second * 15)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				mu.Lock()
				_, err := io.WriteString(rw, fmt.Sprintf("event: %s\n\n", codersdk.ServerSentEventTypePing))
				if err != nil {
					mu.Unlock()
					return
				}
				f.Flush()
				mu.Unlock()
			}
		}
	}()

	sendEvent := func(ctx context.Context, sse codersdk.ServerSentEvent) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)

		_, err := buf.Write([]byte(fmt.Sprintf("event: %s\ndata: ", sse.Type)))
		if err != nil {
			return err
		}

		err = enc.Encode(sse.Data)
		if err != nil {
			return err
		}

		err = buf.WriteByte('\n')
		if err != nil {
			return err
		}

		mu.Lock()
		defer mu.Unlock()
		_, err = rw.Write(buf.Bytes())
		if err != nil {
			return err
		}
		f.Flush()

		return nil
	}

	return sendEvent, nil
}
