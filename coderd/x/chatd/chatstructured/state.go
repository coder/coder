// This file defines the internal structured output metadata stored in chat
// message parts. Payloads decode strictly and fail closed with fixed-text
// errors because they decide what the executor does next, and raw JSON
// values stay raw so numbers and strings never pass through float64.

package chatstructured

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// ErrMalformedStructuredOutputMetadata rejects an invalid request, control
// or outcome payload.
var ErrMalformedStructuredOutputMetadata = xerrors.New("structured output metadata is malformed")

var requestNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// metadataLimits bound a stored payload. Values inside it were bounded when
// recorded, but storage may re-escape them, so only bytes and depth are
// capped: a candidate or outcome value of the output caps nests one level.
var metadataLimits = limits{maxBytes: 512 << 10, maxDepth: 33}

// payloadKeys are the exact keys of each payload object; encoding/json alone
// also matches keys case-insensitively.
var payloadKeys = map[codersdk.ChatMessagePartType][]string{
	codersdk.ChatMessagePartTypeStructuredOutputRequest: {"request_id", "name", "description", "schema"},
	codersdk.ChatMessagePartTypeStructuredOutputControl: {"request_id", "kind", "value"},
	codersdk.ChatMessagePartTypeStructuredOutputOutcome: {"request_id", "status", "value", "error"},
}

// Request asks for a structured final answer. Decoding only screens Schema
// as raw JSON: the executor compiles it when it runs, so reading history
// never depends on the compiler or pays its cost.
type Request struct {
	RequestID   uuid.UUID       `json:"request_id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
}

// ControlKind is the kind of a Control record.
type ControlKind string

const (
	// ControlCandidate records a valid output in Value, possibly JSON null.
	ControlCandidate ControlKind = "candidate"
	// ControlRejection records one rejected finalizer call.
	ControlRejection ControlKind = "rejection"
	// ControlInvalidation clears the current candidate.
	ControlInvalidation ControlKind = "invalidation"
)

// Control records progress on a request. Later kinds reuse these fields.
type Control struct {
	RequestID uuid.UUID       `json:"request_id"`
	Kind      ControlKind     `json:"kind"`
	Value     json.RawMessage `json:"value,omitempty"`
}

// ValidRequestName reports whether name may name a structured output request.
func ValidRequestName(name string) bool { return requestNamePattern.MatchString(name) }

func (r Request) valid() bool {
	if r.RequestID == uuid.Nil || !requestNamePattern.MatchString(r.Name) || !validText(r.Description, true) {
		return false
	}
	doc, err := ParseSchemaDocument(r.Schema)
	return err == nil && doc != nil // A null schema counts as missing.
}

func (c Control) valid() bool {
	switch c.Kind {
	case ControlCandidate:
		return c.RequestID != uuid.Nil && c.Value != nil
	case ControlRejection, ControlInvalidation:
		return c.RequestID != uuid.Nil && c.Value == nil
	}
	return false
}

func validOutcome(o codersdk.ChatStructuredOutput) bool {
	if o.RequestID == uuid.Nil {
		return false
	}
	if o.Status == codersdk.ChatStructuredOutputStatusSucceeded {
		return o.Value != nil && o.Error == nil
	}
	if o.Value != nil || o.Error == nil || !validText(o.Error.Message, false) {
		return false
	}
	switch o.Error.Code {
	case codersdk.ChatStructuredOutputErrorCodeNotProduced, codersdk.ChatStructuredOutputErrorCodeValidationExhausted,
		codersdk.ChatStructuredOutputErrorCodeGenerationFailed, codersdk.ChatStructuredOutputErrorCodeConfigurationError:
		return o.Status == codersdk.ChatStructuredOutputStatusFailed
	case codersdk.ChatStructuredOutputErrorCodeInterrupted, codersdk.ChatStructuredOutputErrorCodeSuperseded,
		codersdk.ChatStructuredOutputErrorCodeQueueDeleted:
		return o.Status == codersdk.ChatStructuredOutputStatusCanceled
	}
	return false
}

func validText(s string, allowEmpty bool) bool {
	return (allowEmpty || s != "") && len(s) <= 1024 && utf8.ValidString(s)
}

// EncodeRequestPart, EncodeControlPart and EncodeOutcomePart build internal
// parts, returning ErrMalformedStructuredOutputMetadata for invalid payloads.
func EncodeRequestPart(r Request) (codersdk.ChatMessagePart, error) {
	part, err := encodePart(codersdk.ChatMessagePartTypeStructuredOutputRequest, r, r.valid)
	// The executor compiles the stored schema, so it must fit the schema caps once stored.
	if stored, _ := DecodeRequestPart(part); err == nil && !fitsStorage(stored.Schema, schemaDocumentLimits.maxBytes) {
		return codersdk.ChatMessagePart{}, ErrMalformedStructuredOutputMetadata
	}
	return part, err
}

func EncodeControlPart(c Control) (codersdk.ChatMessagePart, error) {
	return encodePart(codersdk.ChatMessagePartTypeStructuredOutputControl, c, c.valid)
}

func EncodeOutcomePart(o codersdk.ChatStructuredOutput) (codersdk.ChatMessagePart, error) {
	return encodePart(codersdk.ChatMessagePartTypeStructuredOutputOutcome, o, func() bool { return validOutcome(o) })
}

// encodePart marshals a payload and decodes the result with the strict
// decoder, which also applies the validity rules, so every part it returns
// decodes: marshaling can re-escape a raw value past the byte cap, and a raw
// value can nest past the depth cap, hold what the raw screen rejects, or
// outgrow the caps once stored. valid checks the original payload: marshaling
// would replace invalid UTF-8.
func encodePart(typ codersdk.ChatMessagePartType, payload any, valid func() bool) (codersdk.ChatMessagePart, error) {
	data, err := json.Marshal(payload)
	part := codersdk.ChatMessagePart{Type: typ, StructuredOutputData: data}
	if !valid() || err != nil {
		return codersdk.ChatMessagePart{}, ErrMalformedStructuredOutputMetadata
	}
	if _, err := decodePart(part); err != nil || !fitsStorage(data, metadataLimits.maxBytes) {
		return codersdk.ChatMessagePart{}, ErrMalformedStructuredOutputMetadata
	}
	return part, nil
}

// fitsStorage bounds strictly decoded compact data as PostgreSQL JSONB
// renders it: a space after each separator (one byte per token) and plain
// decimal numbers, so 1e308 takes 309 bytes (a number with an exponent is
// charged |exponent|+2 extra bytes). Strings never grow: encoding/json
// escapes at least what JSONB escapes.
func fitsStorage(data []byte, maxBytes int) bool {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	size := len(data)
	for tok, err := dec.Token(); err == nil; tok, err = dec.Token() {
		size++
		n, _ := tok.(json.Number)
		if e := strings.IndexAny(string(n), "eE"); e >= 0 {
			exp, expErr := strconv.Atoi(strings.TrimPrefix(string(n[e+1:]), "-"))
			if expErr != nil || len(n)+exp+2 > maxNumberLiteralBytes {
				return false
			}
			size += exp + 2
		}
	}
	return size <= maxBytes
}

// DecodeRequestPart, DecodeControlPart and DecodeOutcomePart strictly decode
// a part of the matching type: exact keys, exactly one JSON value, and the
// same validity rules as encoding.
func DecodeRequestPart(part codersdk.ChatMessagePart) (Request, error) {
	var r Request
	if !decodeStrict(part, codersdk.ChatMessagePartTypeStructuredOutputRequest, &r) || !r.valid() {
		return Request{}, ErrMalformedStructuredOutputMetadata
	}
	return r, nil
}

func DecodeControlPart(part codersdk.ChatMessagePart) (Control, error) {
	var c Control
	if !decodeStrict(part, codersdk.ChatMessagePartTypeStructuredOutputControl, &c) || !c.valid() {
		return Control{}, ErrMalformedStructuredOutputMetadata
	}
	return c, nil
}

func DecodeOutcomePart(part codersdk.ChatMessagePart) (codersdk.ChatStructuredOutput, error) {
	var o codersdk.ChatStructuredOutput
	if !decodeStrict(part, codersdk.ChatMessagePartTypeStructuredOutputOutcome, &o) || !validOutcome(o) {
		return codersdk.ChatStructuredOutput{}, ErrMalformedStructuredOutputMetadata
	}
	return o, nil
}

// decodePart decodes a part of any metadata type.
func decodePart(part codersdk.ChatMessagePart) (any, error) {
	switch part.Type {
	case codersdk.ChatMessagePartTypeStructuredOutputRequest:
		return DecodeRequestPart(part)
	case codersdk.ChatMessagePartTypeStructuredOutputControl:
		return DecodeControlPart(part)
	}
	return DecodeOutcomePart(part)
}

func decodeStrict(part codersdk.ChatMessagePart, typ codersdk.ChatMessagePartType, payload any) bool {
	// The raw screen rejects duplicate keys and trailing data.
	doc, err := parseJSON(part.StructuredOutputData, metadataLimits)
	obj, ok := doc.(map[string]any)
	if part.Type != typ || err != nil || !ok || !exactKeys(obj, payloadKeys[typ]) {
		return false
	}
	if e, has := obj["error"]; has {
		errObj, ok := e.(map[string]any)
		if !ok || !exactKeys(errObj, []string{"code", "message"}) {
			return false
		}
	}
	dec := json.NewDecoder(bytes.NewReader(part.StructuredOutputData))
	dec.DisallowUnknownFields()
	return dec.Decode(payload) == nil
}

func exactKeys(obj map[string]any, allowed []string) bool {
	for key := range obj {
		if !slices.Contains(allowed, key) {
			return false
		}
	}
	return true
}
