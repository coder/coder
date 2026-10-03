// Package chatstructured implements the server-side building blocks for
// structured chat output.
//
// This file is the raw JSON safety boundary every untrusted document
// (inline schemas, finalizer arguments, output values, recovered metadata)
// crosses before schema compilation, validation, or JSONB persistence. It
// parses exactly one value under fixed server-owned caps, keeps numbers as
// json.Number, and rejects what encoding/json silently repairs or JSONB
// cannot store: duplicate keys, invalid UTF-8, unpaired surrogates, U+0000.
package chatstructured

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/xerrors"
)

// Rejection reasons for untrusted input. Error text never contains input
// content because the input is attacker controlled.
var (
	ErrTooLarge          = xerrors.New("json document exceeds the byte limit")
	ErrInvalidUTF8       = xerrors.New("json document is not valid UTF-8")
	ErrMalformed         = xerrors.New("json document is malformed")
	ErrTrailingData      = xerrors.New("json document has data after the first value")
	ErrTooDeep           = xerrors.New("json document exceeds the nesting depth limit")
	ErrTooManyNodes      = xerrors.New("json document exceeds the value count limit")
	ErrArrayTooLong      = xerrors.New("json array exceeds the element limit")
	ErrDuplicateKey      = xerrors.New("json object repeats a key")
	ErrNullCharacter     = xerrors.New("json string contains U+0000")
	ErrUnpairedSurrogate = xerrors.New("json string contains an unpaired surrogate escape")
	ErrNumberTooLong     = xerrors.New("json number exceeds the literal length limit")
	ErrExponentTooLarge  = xerrors.New("json number exponent exceeds the magnitude limit")
)

// Numeric literals are bounded in length and |exponent| so later exact
// arithmetic never materializes huge digit strings or expands 1e999999.
const (
	maxNumberLiteralBytes = 128
	maxNumberExponent     = 1024
)

// limits are fixed server-owned caps for one raw JSON document. A zero
// maxNodes or maxArrayLen leaves that dimension bounded only by maxBytes,
// a real bound because every value and array element costs an input byte.
type limits struct {
	maxBytes    int
	maxDepth    int
	maxNodes    int
	maxArrayLen int
}

var (
	// schemaDocumentLimits bound an inline JSON Schema document. Schema
	// node counting and vocabulary checks belong to the schema compiler.
	schemaDocumentLimits = limits{maxBytes: 16 << 10, maxDepth: 16}
	// outputValueLimits bound an unwrapped structured output value.
	outputValueLimits = limits{maxBytes: 64 << 10, maxDepth: 32, maxNodes: 4096, maxArrayLen: 256}
	// finalizerArgumentLimits bound the raw finalizer tool-call envelope
	// {"output": <value>}: one wrapper level and node above the output caps.
	finalizerArgumentLimits = limits{maxBytes: 80 << 10, maxDepth: 33, maxNodes: 4097, maxArrayLen: 256}
)

// ParseSchemaDocument is ParseOutputValue under the inline schema document caps.
func ParseSchemaDocument(raw []byte) (any, error) {
	return parseJSON(raw, schemaDocumentLimits)
}

// ParseFinalizerArguments is ParseOutputValue under the finalizer envelope caps.
func ParseFinalizerArguments(raw []byte) (any, error) {
	return parseJSON(raw, finalizerArgumentLimits)
}

// ParseOutputValue parses one untrusted unwrapped structured output value.
// The result uses map[string]any, []any, string, json.Number, bool, and
// nil, matching encoding/json with UseNumber; object key order is not
// preserved. raw is never modified. Rejections satisfy errors.Is against
// the Err* values in this package.
func ParseOutputValue(raw []byte) (any, error) {
	return parseJSON(raw, outputValueLimits)
}

// parser walks one document with encoding/json's tokenizer, which owns the
// grammar, while this type owns the budgets and the checks it lacks.
type parser struct {
	raw   []byte
	dec   *json.Decoder
	lim   limits
	nodes int
}

func parseJSON(raw []byte, lim limits) (any, error) {
	// Oversized input costs one comparison. Reject invalid UTF-8 before
	// encoding/json silently replaces it with U+FFFD.
	if len(raw) > lim.maxBytes {
		return nil, ErrTooLarge
	}
	if !utf8.Valid(raw) {
		return nil, ErrInvalidUTF8
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	p := &parser{raw: raw, dec: dec, lim: lim}
	value, err := p.value(0)
	if err != nil {
		return nil, err
	}
	// Exactly one value: a second value or garbage after it is rejected.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, ErrTrailingData
	}
	return value, nil
}

// value parses the next value. depth is the number of containers enclosing
// it: a top-level scalar has depth zero and each container adds one.
func (p *parser) value(depth int) (any, error) {
	start := p.dec.InputOffset()
	tok, err := p.dec.Token()
	if err != nil {
		return nil, ErrMalformed
	}
	// Every scalar and container is one node; keys are not counted.
	p.nodes++
	if p.lim.maxNodes > 0 && p.nodes > p.lim.maxNodes {
		return nil, ErrTooManyNodes
	}
	switch t := tok.(type) {
	case json.Delim:
		// Only an opening delimiter can start a value.
		if depth >= p.lim.maxDepth {
			return nil, ErrTooDeep
		}
		if t == '[' {
			return p.array(depth + 1)
		}
		return p.object(depth + 1)
	case string:
		return t, p.checkString(t, start)
	case json.Number:
		return t, checkNumber(t)
	}
	return tok, nil // bool or nil.
}

func (p *parser) array(depth int) ([]any, error) {
	// Never nil: an empty array must re-encode as [] rather than null.
	out := []any{}
	for p.dec.More() {
		// Checked before parsing so an over-long array stops at the cap.
		if p.lim.maxArrayLen > 0 && len(out) >= p.lim.maxArrayLen {
			return nil, ErrArrayTooLong
		}
		elem, err := p.value(depth)
		if err != nil {
			return nil, err
		}
		out = append(out, elem)
	}
	if _, err := p.dec.Token(); err != nil {
		return nil, ErrMalformed
	}
	return out, nil
}

func (p *parser) object(depth int) (map[string]any, error) {
	out := map[string]any{}
	for p.dec.More() {
		start := p.dec.InputOffset()
		tok, err := p.dec.Token()
		// In key position the tokenizer yields a string or an error.
		key, ok := tok.(string)
		if err != nil || !ok {
			return nil, ErrMalformed
		}
		if err := p.checkString(key, start); err != nil {
			return nil, err
		}
		// encoding/json and JSONB keep one duplicate and erase the evidence.
		// Decoded keys compare equal across escape-equivalent spellings.
		if _, dup := out[key]; dup {
			return nil, ErrDuplicateKey
		}
		val, err := p.value(depth)
		if err != nil {
			return nil, err
		}
		out[key] = val
	}
	if _, err := p.dec.Token(); err != nil {
		return nil, ErrMalformed
	}
	return out, nil
}

// checkString rejects U+0000 and unpaired UTF-16 surrogate escapes, which
// encoding/json silently replaces with U+FFFD. The raw literal starts at
// decoder offset start, possibly with separators (never a backslash), and
// the tokenizer accepted its grammar, so every backslash starts a full escape.
func (p *parser) checkString(decoded string, start int64) error {
	if strings.IndexByte(decoded, 0) >= 0 {
		return ErrNullCharacter
	}
	lit := p.raw[start:p.dec.InputOffset()]
	for i := 0; i < len(lit); i++ {
		if lit[i] != '\\' {
			continue
		}
		r, ok := unicodeEscape(lit[i:])
		if !ok {
			i++ // A simple escape such as \\ or \n: skip its letter.
			continue
		}
		i += 5
		switch {
		case r >= 0xD800 && r < 0xDC00:
			// A high surrogate is valid only directly before a low one.
			low, ok := unicodeEscape(lit[i+1:])
			if !ok || low < 0xDC00 || low >= 0xE000 {
				return ErrUnpairedSurrogate
			}
			i += 6
		case r >= 0xDC00 && r < 0xE000:
			return ErrUnpairedSurrogate
		}
	}
	return nil
}

// unicodeEscape decodes a \uXXXX escape at the start of b.
func unicodeEscape(b []byte) (uint64, bool) {
	if len(b) < 6 || b[0] != '\\' || b[1] != 'u' {
		return 0, false
	}
	v, err := strconv.ParseUint(string(b[2:6]), 16, 16)
	return v, err == nil
}

// checkNumber bounds a literal the tokenizer already accepted as a JSON
// number. The exponent is inspected as text, never computed.
func checkNumber(n json.Number) error {
	if len(n) > maxNumberLiteralBytes {
		return ErrNumberTooLong
	}
	if e := strings.IndexAny(string(n), "eE"); e >= 0 {
		// One optional sign then digits: trimming both leaves the significant
		// digits, the "0" prefix keeps a zero exponent parseable, and Atoi
		// fails cleanly on digit strings beyond the int range.
		v, err := strconv.Atoi("0" + strings.TrimLeft(string(n[e+1:]), "+-0"))
		if err != nil || v > maxNumberExponent {
			return ErrExponentTooLarge
		}
	}
	return nil
}
