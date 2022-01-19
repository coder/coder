package xjson

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"reflect"
	"runtime"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/go-playground/validator/v10"
	"golang.org/x/xerrors"

	"github.com/coder/coder/buildmode"
	"github.com/coder/coder/validate"

	"cdr.dev/slog"
)

// This contains the raw mark-up for our dynamic server-side error page.
// Reasons for using a string literal:
//
// 1. It's a small/simple amount of markup
// 2. We avoid possible file-path errors from files moving around
// 3. More performant than doing file i/o
// 4. Development will be easier because it becomes hot-swappable
//go:embed error_page.html
var errPageMarkup string

// m is a a helper struct for marshaling arbitrary json.
type m map[string]interface{}

// SuccessMsg contains a single 'msg' field
// with an arbitrary value indicating success.
type SuccessMsg struct {
	Msg string `json:"msg"`
} // @name SuccessMsg

// Write writes a json response.
func Write(w http.ResponseWriter, status int, body interface{}) {
	if body == nil {
		w.WriteHeader(status)
		return
	}

	if strBody, ok := body.(string); ok {
		body = SuccessMsg{
			Msg: strBody,
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	err := encodeBody(w, body)
	if err != nil {
		// We can't write to hijacked connections. Don't panic in that
		// case.
		if xerrors.Is(err, http.ErrHijacked) {
			return
		}

		panic(err)
	}
}

func encodeBody(w io.Writer, body interface{}) error {
	enc := json.NewEncoder(w)
	// Format the response nicely.
	enc.SetIndent("", "\t")
	enc.SetEscapeHTML(false)
	return enc.Encode(body)
}

// ErrPage is used for writing error templates
// to the response writer using WriteErrPage.
type ErrPage struct {
	DevURL    string
	AccessURL string
	Msg       string
	Code      int
	Err       error
}

// WriteErrPage writes error templates to w after dynamically constructing it
// based on the contents of p.
//
// If p.Code == 0:
// The status code will default to http.StatusInternalServerError.
//
// If p.Msg == "":
// The error message will default to the status text of the status code.

// If p.Err == nil:
// The error will not render. It is optional to provide a value for p.Err since
// p.Msg is rendered as the public-facing error that the user will see. p.Err
// can be used for development debugging purposes.
//
// If p.AccessURL == "":
// The Back to Site button on the page won't work.  AccessURL should have it's
// value set to a database.ConfigGeneral.AccessURL.URL.
//
// If p.DevURL == "":
// The retry button linking back to the dev url will not appear on the rendered page.
func WriteErrPage(w http.ResponseWriter, p ErrPage) {
	if p.Code == 0 {
		p.Code = http.StatusInternalServerError
	}

	if p.Msg == "" {
		p.Msg = http.StatusText(p.Code)
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(p.Code)

	t, err := template.New("").Funcs(
		template.FuncMap{
			"status": func(p ErrPage) string {
				return fmt.Sprintf("%d - %s", p.Code, http.StatusText(p.Code))
			},
		},
	).Parse(errPageMarkup)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Load the contents of p into the template then write the template to w.
	if err := t.ExecuteTemplate(w, "", p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// ErrUnauthorized is an error returned when a user tries to access a resource without
// having sufficient permissions.
var ErrUnauthorized = xerrors.New("Insufficient permissions to access resource")

// WriteUnauthorizedError writes out an error formatted in JSON
// about the user not having sufficient permissions to access a resource.
func WriteUnauthorizedError(w http.ResponseWriter) {
	WriteUnauthorized(w, ErrUnauthorized.Error())
}

// DatabaseError writes 500 with a database error message to the response writer,
// and logs details about the error.
func DatabaseError(ctx context.Context, log slog.Logger, w http.ResponseWriter, err error) {
	// To maintain backwards-compatible behavior we do not write the database error to
	// the details.
	WriteCustomInternalServerError(w, "A database error occurred.", nil)
	slog.Helper()
	log.Error(ctx, "A database error occurred.", slog.Error(err))
}

// ServerError writes a 500 with the message to the response writer, and logs details
// about the error.
func ServerError(ctx context.Context, log slog.Logger, w http.ResponseWriter, err error, msg string) {
	WriteInternalServerError(w, nil)
	slog.Helper()
	log.Error(ctx, "server error",
		slog.F("msg", msg),
		slog.Error(err),
	)
}

// validatorErrorMessage constructs a human readable message from a validation error.
func validatorErrorMessage(err govalidator.Error) string {
	switch {
	case err.Validator == "required":
		return fmt.Sprintf("Field %q is required.", err.Name)
	default:
		return fmt.Sprintf("Field %q is invalid (%v).", err.Name, err.Err.Error())
	}
}

// convertValidationErrors converts govalidator errors into structured JSON.
// Each entry of the returned []m has at least `msg` set.
func convertValidationErrors(errs govalidator.Errors) []m {
	var r []m

	for _, err := range errs {
		switch e := err.(type) { // nolint: errorlint
		case govalidator.Errors:
			// For some reason govalidator nests another Errors sometimes.
			// Let's just flatten and append it.
			r = append(r, convertValidationErrors(e)...)
		case govalidator.Error:
			r = append(r, m{
				"msg":       validatorErrorMessage(e),
				"field":     e.Name,
				"error":     e.Err.Error(),
				"validator": e.Validator,
			})
		default:
			r = append(r, m{
				"msg": err,
				// type is provided to aid in debugging. It offers no contract.
				"type": reflect.TypeOf(err).String(),
			})
		}
	}

	return r
}

// ReadBody reads a json object from the request body. If the read fails, a 400
// is sent back to the client, and this will return false.
//
// To ensure proper validation during development, this function will fatal if
// the current build mode is "dev", if there's at least one field with the
// "validate" tag, and there's additional fields on the struct that are not
// validated (in accordance to `validate.FieldsMissingValidation`).
func ReadBody(log slog.Logger, w http.ResponseWriter, r *http.Request, v interface{}) bool {
	err := json.NewDecoder(r.Body).Decode(v)
	if err != nil {
		log.Warn(r.Context(), "failed to read body", slog.Error(err))
		WriteError(w, http.StatusBadRequest, "Failed to read body.", err)
		return false
	}

	if buildmode.Dev() {
		mustConsistentlyValidate(r.Context(), log, v)
	}

	return Validate(w, v)
}

func mustConsistentlyValidate(ctx context.Context, log slog.Logger, v interface{}) {
	// Only make the logger if we need it.
	logger := func() slog.Logger {
		// Add caller and object type to know where to find struct that is failing.
		_, file, line, _ := runtime.Caller(3)
		return log.With(
			slog.F("v", v),
			slog.F("type", reflect.TypeOf(v).String()),
			slog.F("caller", fmt.Sprintf("%s:%d", file, line)),
		)
	}

	// Get explicitly tagged fields.
	explicit, err := validate.FieldsWithValidation(v)
	// Errors only if `v` isn't a struct.
	if err != nil {
		logger().Debug(ctx, "failed to check for fields with validation", slog.Error(err))
		return
	}
	if len(explicit) > 0 {
		notValidated, err := validate.FieldsMissingValidation(v)
		if err != nil {
			logger().Debug(ctx, "failed to check for fields missing validation", slog.Error(err))
			return
		}
		if len(notValidated) > 0 {
			logger().Fatal(ctx, "some fields missing validation",
				slog.F("explicitly_validated", explicit),
				slog.F("not_validated", notValidated),
			)
		}
	}
}

func summarizeValidationErrors(subErrors []m) string {
	var sb strings.Builder
	_, _ = fmt.Fprint(&sb, "Input validation failed.")
	for _, v := range subErrors {
		_, _ = fmt.Fprint(&sb, "\n", "• ", v["msg"])
	}
	return sb.String()
}

// summarizeFieldErrors formats an error string suitable for displaying directly
// to the user from field validation errors.
func summarizeFieldErrors(errs []validator.FieldError) string {
	var sb strings.Builder
	for i, err := range errs {
		// Error is decent enough. Will produce a string in the form of:
		// "Key: '%s' Error:Field validation for '%s' failed on the '%s' tag"
		_, _ = sb.WriteString(err.Error())
		if i != len(errs)-1 {
			_, _ = sb.WriteString(", ")
		}
	}

	return sb.String()
}

// convertFieldErrors converts field errors into structured JSON.  Each entry of
// the returned []m has at least `msg` set.
func convertFieldErrors(errs []validator.FieldError) []m {
	ms := make([]m, len(errs))
	for i, err := range errs {
		ms[i] = m{
			"msg": err.Error(),
		}
	}

	return ms
}

// Validate will call `Check` on the provided value, and write an appropriate
// response if validation fails.
func Validate(w http.ResponseWriter, v interface{}) bool {
	if err := Check(v); err != nil {
		WriteError(w, http.StatusBadRequest, "Request failed to validate.", err)
		return false
	}
	return true
}

type checkError struct {
	Message string `json:"msg"`
	Errors  []m    `json:"validation_errors,omitempty"`
}

func (e *checkError) Error() string {
	return fmt.Sprintf("%s: [%s]", e.Message, summarizeValidationErrors(e.Errors))
}

// Check runs validation on fields of a struct. If the type passed in is not a
// struct, no validation will be done.
//
// If a struct field has a "valid" tag, asaskevich/govalidator will be used for
// validation. If a struct field has a "validate" tag, go-playground/validator
// will be used for validation.
func Check(v interface{}) error {
	// govalidator returns an error if the type isn't struct or *struct.
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	// Validate using go-playground/validator first.
	if err := validate.Validator().Struct(v); err != nil {
		var vErrs validator.ValidationErrors
		if xerrors.As(err, &vErrs) {
			return &checkError{
				Message: summarizeFieldErrors(vErrs),
				Errors:  convertFieldErrors(vErrs),
			}
		}
		return &checkError{Message: fmt.Sprintf("input validation: %s", err.Error())}
	}

	// Validate using asaskevich/govalidator after the above. Eventually this
	// should be removed when all validation is switched over.
	if ok, err := govalidator.ValidateStruct(v); err != nil {
		var gve govalidator.Errors
		if xerrors.As(err, &gve) {
			verrs := convertValidationErrors(gve)

			return &checkError{
				Message: summarizeValidationErrors(verrs),
				Errors:  verrs,
			}
		}

		return &checkError{Message: fmt.Sprintf("input validation: %s", err.Error())}
	} else if !ok {
		return &checkError{Message: "input validation failed"}
	}

	return nil
}
