// This file screens the vocabulary of an inline JSON Schema in one linear
// walk over its schema positions before any validator library sees it. It
// is NOT full Draft-07 validity checking and imposes no resource limits
// beyond the raw JSON caps of ParseSchemaDocument. Later layers add schema
// resource limits, regexp screening, trusted meta-validation and
// compilation; meta-validation rejects what this walk copies unchecked,
// such as malformed keyword values. The preflight never decides whether an
// instance matches a schema.

package chatstructured

import (
	"maps"

	"golang.org/x/xerrors"
)

// Schema rejection reasons. Raw JSON rejections from ParseSchemaDocument
// are returned unchanged.
var (
	ErrInvalidSchema      = xerrors.New("json schema is invalid")
	ErrUnsupportedDialect = xerrors.New("json schema dialect is not supported")
	ErrUnsupportedKeyword = xerrors.New("json schema uses an unsupported keyword")
	ErrUnsupportedFormat  = xerrors.New("json schema uses an unsupported format")
)

// screenedSchema is the result of preflightSchema.
type screenedSchema struct {
	document  map[string]any // the parsed original document, meta-validated later as data
	sanitized map[string]any // a copy for compilation: annotations and $schema removed
}

// preflightSchema parses raw with the ParseSchemaDocument caps and screens
// the vocabulary at schema positions. The depth cap bounds the recursion.
func preflightSchema(raw []byte) (screenedSchema, error) {
	doc, err := ParseSchemaDocument(raw)
	if err != nil {
		return screenedSchema{}, err
	}
	root, ok := doc.(map[string]any)
	if !ok {
		return screenedSchema{}, ErrInvalidSchema
	}
	// An absent $schema means Draft-07. A value of another type never
	// equals a string constant.
	if d, ok := root["$schema"]; ok && d != "http://json-schema.org/draft-07/schema#" && d != "http://json-schema.org/draft-07/schema" {
		return screenedSchema{}, ErrUnsupportedDialect
	}
	// $schema declares the dialect only at the root; the walk rejects it
	// anywhere else.
	top := maps.Clone(root)
	delete(top, "$schema")
	sanitized, err := screenObject(top)
	if err != nil {
		return screenedSchema{}, err
	}
	return screenedSchema{document: root, sanitized: sanitized}, nil
}

// screenSchema screens the value at a schema position. Booleans are
// complete schemas, and a value that is not a schema is returned unchanged
// for meta-validation to reject.
func screenSchema(v any) (any, error) {
	obj, ok := v.(map[string]any)
	if !ok {
		return v, nil
	}
	return screenObject(obj)
}

// screenObject builds the sanitized copy of one schema object in a fresh
// map. The input is never modified; literal values are shared read-only.
func screenObject(obj map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(obj))
	for key, val := range obj {
		var err error
		switch key {
		case "title", "description", "default", "examples", "$comment", "readOnly", "writeOnly",
			"contentMediaType", "contentEncoding":
			// Inert annotations are omitted so their data never reaches the library.
		case "type", "enum", "const", "multipleOf", "maximum", "exclusiveMaximum", "minimum",
			"exclusiveMinimum", "maxLength", "minLength", "pattern", "maxItems", "minItems",
			"uniqueItems", "maxProperties", "minProperties", "required":
			// Literal data, never inspected. Patterns are not compiled here.
			out[key] = val
		case "format":
			if f, ok := val.(string); ok && !supportedFormat(f) {
				return nil, ErrUnsupportedFormat
			}
			out[key] = val
		case "additionalItems", "additionalProperties", "contains", "propertyNames",
			"not", "if", "then", "else":
			out[key], err = screenSchema(val)
		case "items":
			if list, ok := val.([]any); ok {
				out[key], err = screenList(list)
			} else {
				out[key], err = screenSchema(val)
			}
		case "allOf", "anyOf", "oneOf":
			out[key] = val
			if list, ok := val.([]any); ok {
				out[key], err = screenList(list)
			}
		case "properties", "patternProperties", "dependencies":
			out[key] = val
			if members, ok := val.(map[string]any); ok {
				out[key], err = screenMembers(members)
			}
		default:
			// Includes $ref, $id, id, $defs, definitions and a nested
			// $schema, so no reference can reach a loader.
			return nil, ErrUnsupportedKeyword
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// screenList screens an array of schemas into a fresh array.
func screenList(list []any) ([]any, error) {
	out := make([]any, len(list))
	for i, v := range list {
		sub, err := screenSchema(v)
		if err != nil {
			return nil, err
		}
		out[i] = sub
	}
	return out, nil
}

// screenMembers screens the values of properties, patternProperties or
// dependencies into a fresh map. Member names are literal data, and
// array-form dependencies pass through screenSchema unchanged.
func screenMembers(members map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(members))
	for name, v := range members {
		sub, err := screenSchema(v)
		if err != nil {
			return nil, err
		}
		out[name] = sub
	}
	return out, nil
}

// supportedFormat reports whether f is a format the validator may check.
// regex is excluded: the pinned gojsonschema v1.2.0 compiles every instance
// string under it, which the selected resource budget does not constrain
// adequately.
func supportedFormat(f string) bool {
	switch f {
	case "date", "time", "date-time", "hostname", "email", "idn-email", "ipv4", "ipv6", "uri",
		"uri-reference", "iri", "iri-reference", "uri-template", "uuid", "json-pointer",
		"relative-json-pointer":
		return true
	}
	return false
}
