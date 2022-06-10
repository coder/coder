package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
)

var (
	validate      *validator.Validate
	usernameRegex = regexp.MustCompile("^[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*$")
)

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
	err := validate.RegisterValidation("username", func(fl validator.FieldLevel) bool {
		f := fl.Field().Interface()
		str, ok := f.(string)
		if !ok {
			return false
		}
		if len(str) > 32 {
			return false
		}
		if len(str) < 1 {
			return false
		}
		return usernameRegex.MatchString(str)
	})
	if err != nil {
		panic(err)
	}
}

// Response represents a generic HTTP response.
type Response struct {
	// Message is an actionable message that depicts actions the request took.
	// These messages should be fully formed sentences with proper punctuation.
	// Examples:
	// - "A user has been created."
	// - "Failed to create a user."
	Message string `json:"message"`
	// Detail is a debug message that provides further insight into why the
	// action failed. This information can be technical and a regular golang
	// err.Error() text.
	// - "database: too many open connections"
	// - "stat: too many open files"
	Detail string `json:"detail,omitempty"`
	// Validations are form field-specific friendly error messages. They will be
	// shown on a form field in the UI. These can also be used to add additional
	// context if there is a set of errors in the primary 'Message'.
	Validations []Error `json:"validations,omitempty"`
}

// Error represents a scoped error to a user input.
type Error struct {
	Field  string `json:"field" validate:"required"`
	Detail string `json:"detail" validate:"required"`
}

// ResourceNotFound is intentionally vague. All 404 responses should be identical
// to prevent leaking existence of resources.
func ResourceNotFound(rw http.ResponseWriter) {
	Write(rw, http.StatusNotFound, Response{
		Message: "Resource not found or you do not have access to this resource",
	})
}

func Forbidden(rw http.ResponseWriter) {
	Write(rw, http.StatusForbidden, Response{
		Message: "Forbidden.",
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
		Write(rw, http.StatusBadRequest, Response{
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return false
	}
	err = validate.Struct(value)
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		apiErrors := make([]Error, 0, len(validationErrors))
		for _, validationError := range validationErrors {
			apiErrors = append(apiErrors, Error{
				Field:  validationError.Field(),
				Detail: fmt.Sprintf("Validation failed for tag %q with value: \"%v\"", validationError.Tag(), validationError.Value()),
			})
		}
		Write(rw, http.StatusBadRequest, Response{
			Message:     "Validation failed.",
			Validations: apiErrors,
		})
		return false
	}
	if err != nil {
		Write(rw, http.StatusInternalServerError, Response{
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
