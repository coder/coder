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
	"golang.org/x/xerrors"

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

// ResourceNotFound is intentionally vague. All 404 responses should be identical
// to prevent leaking existence of resources.
func ResourceNotFound(rw http.ResponseWriter) {
	Write(rw, http.StatusNotFound, codersdk.Response{
		Message: "Resource not found or you do not have access to this resource",
	})
}

func Forbidden(rw http.ResponseWriter) {
	Write(rw, http.StatusForbidden, codersdk.Response{
		Message: "Forbidden.",
	})
}

func InternalServerError(rw http.ResponseWriter, err error) {
	var details string
	if err != nil {
		details = err.Error()
	}

	Write(rw, http.StatusInternalServerError, codersdk.Response{
		Message: "An internal server error occurred.",
		Detail:  details,
	})
}

// Write outputs a standardized format to an HTTP response body.
func Write(rw http.ResponseWriter, status int, response interface{}) {
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

// Read decodes JSON from the HTTP request into the value provided.
// It uses go-validator to validate the incoming request body.
func Read(rw http.ResponseWriter, r *http.Request, value interface{}) bool {
	err := json.NewDecoder(r.Body).Decode(value)
	if err != nil {
		Write(rw, http.StatusBadRequest, codersdk.Response{
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
		Write(rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Validation failed.",
			Validations: apiErrors,
		})
		return false
	}
	if err != nil {
		Write(rw, http.StatusInternalServerError, codersdk.Response{
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

func ServerSideEventSender(rw http.ResponseWriter, r *http.Request) (func(ctx context.Context, sse codersdk.ServerSideEvent) error, error) {
	var mu sync.Mutex
	h := rw.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")

	sw, ok := rw.(*StatusWriter)
	if !ok {
		return nil, xerrors.New("http.ResponseWriter is not StatusWriter")
	}

	f, ok := sw.ResponseWriter.(http.Flusher)
	if !ok {
		return nil, xerrors.New("http.ResponseWriter is not http.Flusher")
	}

	pingMsg := fmt.Sprintf("event: %s\n\n", codersdk.EventTypePing)
	_, err := io.WriteString(rw, pingMsg)
	if err != nil {
		return nil, err
	}
	f.Flush()

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
				_, err := io.WriteString(rw, pingMsg)
				if err != nil {
					mu.Unlock()
					return
				}
				f.Flush()
				mu.Unlock()
			}
		}
	}()

	sendEvent := func(ctx context.Context, sse codersdk.ServerSideEvent) error {
		if r.Context().Err() != nil {
			return err
		}

		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(true)

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
