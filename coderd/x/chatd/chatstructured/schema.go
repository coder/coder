// This file screens the vocabulary and resource use of an inline JSON
// Schema in one linear walk over its schema positions before any validator
// library sees it. It is NOT full Draft-07 validity checking: trusted
// meta-validation and compilation run later and reject what this walk
// copies unchecked, such as malformed keyword values. The preflight never
// decides whether an instance matches a schema.
//
// The pinned gojsonschema v1.2.0 recompiles every patternProperties pattern
// for every object key on every validation and keeps every validation error
// (about 1 KiB each) without failing fast. Measured on a development host
// (cumulative allocation): 2 small patterns over 4095 keys took 0.26 s and
// 192 MiB, 60 patterns over 4000 keys 23.7 s; 32 failing allOf branches over
// 4000 keys produced 128k errors and 131 MiB, and 120 schema-form
// dependencies over 4000 keys 480k errors and 465 MiB. Fixed limits on
// schema nodes, branches, branch nesting, patterns and patternProperties
// therefore hold before the library sees a schema.

package chatstructured

import (
	"maps"
	"regexp/syntax"
	"slices"

	"golang.org/x/xerrors"
)

// Schema rejection reasons. Raw JSON rejections from ParseSchemaDocument
// are returned unchanged.
var (
	ErrInvalidSchema      = xerrors.New("json schema is invalid")
	ErrUnsupportedDialect = xerrors.New("json schema dialect is not supported")
	ErrUnsupportedKeyword = xerrors.New("json schema uses an unsupported keyword")
	ErrUnsupportedFormat  = xerrors.New("json schema uses an unsupported format")
	ErrSchemaTooComplex   = xerrors.New("json schema exceeds a complexity limit")
)

// Fixed server-owned schema limits. Every branch applies another schema to
// the same instance and multiplies the errors the library keeps.
const (
	maxSchemaNodes       = 256
	maxSchemaBranches    = 8
	maxBranchNesting     = 4
	maxPatternBytes      = 256
	maxPatternInsts      = 128
	maxPatternEstimate   = 1024
	maxPatternProperties = 4 // patterns summed over the whole document
)

// screenedSchema is the result of preflightSchema.
type screenedSchema struct {
	document  map[string]any // the parsed original document, meta-validated later as data
	sanitized map[string]any // a copy for compilation: annotations and $schema removed
	// patternProperties is true when any schema position uses
	// patternProperties, so outputs are validated under a reduced profile.
	patternProperties bool
}

// preflightSchema parses raw with the ParseSchemaDocument caps and screens
// the vocabulary and resource limits at schema positions. The depth cap
// bounds the recursion.
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
	w := schemaWalker{nodes: 1} // The root.
	sanitized, err := w.screenObject(top, 0)
	if err != nil {
		return screenedSchema{}, err
	}
	return screenedSchema{document: root, sanitized: sanitized, patternProperties: w.patternProperties}, nil
}

// schemaWalker carries the document-wide counts of one walk. Levels count
// branch nesting from the root at level zero. Literal data never counts.
type schemaWalker struct {
	nodes, branches, patterns int
	patternProperties         bool
}

// isSchema reports whether v is an object or boolean schema.
func isSchema(v any) bool {
	_, isBool := v.(bool)
	_, isObject := v.(map[string]any)
	return isBool || isObject
}

// screenSchema screens the value at a schema position. Booleans are
// complete schemas, and a value that is not a schema is returned unchanged
// for meta-validation to reject.
func (w *schemaWalker) screenSchema(v any, level int) (any, error) {
	if !isSchema(v) {
		return v, nil
	}
	w.nodes++
	if w.nodes > maxSchemaNodes || level > maxBranchNesting {
		return nil, ErrSchemaTooComplex
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return v, nil
	}
	return w.screenObject(obj, level)
}

// screenBranch screens a subschema that applies to the same instance one
// nesting level deeper. A malformed branch still counts.
func (w *schemaWalker) screenBranch(v any, level int) (any, error) {
	w.branches++
	if w.branches > maxSchemaBranches {
		return nil, ErrSchemaTooComplex
	}
	return w.screenSchema(v, level+1)
}

// screenObject builds the sanitized copy of one schema object in a fresh
// map. The input is never modified; literal values are shared read-only.
// Rejection is deterministic because keys are walked in sorted order.
func (w *schemaWalker) screenObject(obj map[string]any, level int) (map[string]any, error) {
	out := make(map[string]any, len(obj))
	for _, key := range slices.Sorted(maps.Keys(obj)) {
		val := obj[key]
		var err error
		switch key {
		case "title", "description", "default", "examples", "$comment", "readOnly", "writeOnly",
			"contentMediaType", "contentEncoding":
			// Inert annotations are omitted so their data never reaches the library.
		case "type", "enum", "const", "multipleOf", "maximum", "exclusiveMaximum", "minimum",
			"exclusiveMinimum", "maxLength", "minLength", "maxItems", "minItems",
			"uniqueItems", "maxProperties", "minProperties", "required":
			// Literal data, never inspected.
			out[key] = val
		case "pattern":
			out[key] = val
			if p, ok := val.(string); ok {
				err = checkPattern(p)
			}
		case "format":
			if f, ok := val.(string); ok && !supportedFormat(f) {
				return nil, ErrUnsupportedFormat
			}
			out[key] = val
		case "additionalItems", "additionalProperties", "contains", "propertyNames":
			out[key], err = w.screenSchema(val, level)
		case "not", "if", "then", "else":
			out[key], err = w.screenBranch(val, level)
		case "items":
			if list, ok := val.([]any); ok {
				out[key], err = screenList(list, level, w.screenSchema)
			} else {
				out[key], err = w.screenSchema(val, level)
			}
		case "allOf", "anyOf", "oneOf":
			out[key] = val
			if list, ok := val.([]any); ok {
				out[key], err = screenList(list, level, w.screenBranch)
			}
		case "properties", "patternProperties", "dependencies":
			w.patternProperties = w.patternProperties || key == "patternProperties"
			out[key] = val
			if members, ok := val.(map[string]any); ok {
				out[key], err = w.screenMembers(key, members, level)
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
func screenList(list []any, level int, screen func(any, int) (any, error)) ([]any, error) {
	out := make([]any, len(list))
	for i, v := range list {
		sub, err := screen(v, level)
		if err != nil {
			return nil, err
		}
		out[i] = sub
	}
	return out, nil
}

// screenMembers screens the values of properties, patternProperties or
// dependencies into a fresh map. Member names are literal data, except
// patternProperties keys, which are patterns. Only schema-form dependencies
// are branches; array-form dependencies pass through unchanged.
func (w *schemaWalker) screenMembers(key string, members map[string]any, level int) (map[string]any, error) {
	out := make(map[string]any, len(members))
	for _, name := range slices.Sorted(maps.Keys(members)) {
		v := members[name]
		screen := w.screenSchema
		if key == "dependencies" && isSchema(v) {
			screen = w.screenBranch
		}
		if key == "patternProperties" {
			w.patterns++
			if w.patterns > maxPatternProperties {
				return nil, ErrSchemaTooComplex
			}
			if err := checkPattern(name); err != nil {
				return nil, err
			}
		}
		sub, err := screen(v, level)
		if err != nil {
			return nil, err
		}
		out[name] = sub
	}
	return out, nil
}

// checkPattern bounds one regular expression. Counted repetitions expand
// at compile time (a{1000} is about 1000 instructions), so a cheap estimate
// rejects large programs before the exact count pays for compiling one.
// The program is counted and discarded, never cached or run.
func checkPattern(p string) error {
	if len(p) > maxPatternBytes {
		return ErrSchemaTooComplex
	}
	// The flags of regexp.Compile, so both agree on syntax.
	re, err := syntax.Parse(p, syntax.Perl)
	if err != nil {
		return ErrInvalidSchema
	}
	if estimateInsts(re) > maxPatternEstimate {
		return ErrSchemaTooComplex
	}
	prog, err := syntax.Compile(re.Simplify())
	if err != nil || len(prog.Inst) > maxPatternInsts {
		return ErrSchemaTooComplex
	}
	return nil
}

// estimateInsts returns an upper bound on the instruction count of
// syntax.Compile(re.Simplify()), saturated above maxPatternEstimate. The
// program adds a fail and a match instruction to its body.
func estimateInsts(re *syntax.Regexp) int {
	return min(estimateNode(re)+2, maxPatternEstimate+1)
}

// estimateNode mirrors regexp/syntax: one instruction per rune, class or
// empty-width assertion, two per capture, at most two per star, plus or
// quest, one per alternative, and Simplify expands x{n,m} into m copies of
// x plus at most m-n quests. Saturating every node keeps the products small
// because parsed repeat counts are at most 1000.
func estimateNode(re *syntax.Regexp) int {
	n := 1
	switch re.Op {
	case syntax.OpLiteral:
		n = max(1, len(re.Rune))
	case syntax.OpCapture, syntax.OpStar, syntax.OpPlus, syntax.OpQuest:
		n = estimateNode(re.Sub[0]) + 2
	case syntax.OpRepeat:
		count := re.Max
		if count < 0 { // x{n,} becomes n-1 copies of x and x+.
			count = re.Min + 1
		}
		n = count*(estimateNode(re.Sub[0])+1) + 2
	case syntax.OpConcat, syntax.OpAlternate:
		n += len(re.Sub)
		for _, sub := range re.Sub {
			n = min(n+estimateNode(sub), maxPatternEstimate+1)
		}
	}
	return min(n, maxPatternEstimate+1)
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
