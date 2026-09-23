package chatstructured

import (
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const draft07 = "http://json-schema.org/draft-07/schema#"

// everyKeyword uses every allowed keyword, array items with additionalItems,
// both dependencies forms, and nested boolean schemas.
const everyKeyword = `{"type":"object","enum":[{}],"const":{},"multipleOf":2,"maximum":9,` +
	`"exclusiveMaximum":10,"minimum":0,"exclusiveMinimum":-1,"maxLength":5,"minLength":1,` +
	`"pattern":"^a","format":"email","maxItems":3,"minItems":1,"uniqueItems":true,` +
	`"maxProperties":4,"minProperties":1,"required":["a"],"properties":{"a":true},` +
	`"patternProperties":{"^x-":false},"additionalProperties":{"items":[{},true],` +
	`"additionalItems":false,"contains":{"items":{}}},"dependencies":{"a":["b"],"b":{"required":["a"]}},` +
	`"propertyNames":{"maxLength":8},"if":{},"then":true,"else":false,"allOf":[{}],"anyOf":[true],` +
	`"oneOf":[{}],"not":false}`

// Keyword-shaped literal data that must never be screened.
const (
	refLiteral = `{"$ref":"http://127.0.0.1:1/x","$id":"http://json-schema.org/draft-07/schema#"}`
	literals   = `[` + refLiteral + `,{"format":"regex"},{"x-unknown":1}]`
)

// annotationMembers holds every annotation keyword, with literal values.
const annotationMembers = `"title":"t","description":"d","default":` + literals + `,"examples":` + literals +
	`,"$comment":"c","readOnly":true,"writeOnly":false,"contentMediaType":"text/plain","contentEncoding":"base64"`

const annotated = `{` + annotationMembers + `}`

// schemaPositions wraps @ in every kind of schema position.
var schemaPositions = []string{
	`@`, `{"properties":{"a":@}}`, `{"patternProperties":{"^a":@}}`, `{"dependencies":{"a":@}}`,
	`{"items":@}`, `{"items":[{},@]}`, `{"additionalItems":@}`, `{"additionalProperties":@}`,
	`{"contains":@}`, `{"propertyNames":@}`, `{"allOf":[@]}`, `{"anyOf":[true,@]}`, `{"oneOf":[@]}`,
	`{"not":@}`, `{"if":@}`, `{"then":@}`, `{"else":@}`, `{"properties":{"a":{"items":[{"allOf":[{"not":@}]}]}}}`,
}

// Independent copies of the preflight's lists.
var (
	allowedKeywords = []string{
		"type", "enum", "const", "multipleOf", "maximum", "exclusiveMaximum", "minimum", "exclusiveMinimum",
		"maxLength", "minLength", "pattern", "items", "additionalItems", "maxItems", "minItems", "uniqueItems",
		"contains", "maxProperties", "minProperties", "required", "properties", "patternProperties",
		"additionalProperties", "dependencies", "propertyNames", "if", "then", "else", "allOf", "anyOf",
		"oneOf", "not", "format",
	}
	omittedKeywords = []string{
		"$schema", "title", "description", "default", "examples", "$comment", "readOnly", "writeOnly",
		"contentMediaType", "contentEncoding",
	}
	testFormats = []string{
		"date", "time", "date-time", "hostname", "email", "idn-email", "ipv4", "ipv6", "uri", "uri-reference",
		"iri", "iri-reference", "uri-template", "uuid", "json-pointer", "relative-json-pointer",
	}
)

// repeatJoin joins n copies of format with commas, replacing @ with the index.
func repeatJoin(n int, format string) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = strings.ReplaceAll(format, "@", strconv.Itoa(i))
	}
	return strings.Join(parts, ",")
}

// walkSchemas calls visit on every schema object at a schema position of v,
// children before parents, independently of the preflight's walker.
func walkSchemas(v any, visit func(map[string]any)) {
	obj, ok := v.(map[string]any)
	if !ok {
		return
	}
	for key, val := range obj {
		var subs []any
		switch key {
		case "additionalItems", "additionalProperties", "contains", "propertyNames", "not", "if", "then", "else":
			subs = []any{val}
		case "items":
			subs = []any{val}
			if list, ok := val.([]any); ok {
				subs = list
			}
		case "allOf", "anyOf", "oneOf":
			subs, _ = val.([]any)
		case "properties", "patternProperties", "dependencies":
			members, _ := val.(map[string]any)
			subs = slices.Collect(maps.Values(members))
		}
		for _, sub := range subs {
			walkSchemas(sub, visit)
		}
	}
	visit(obj)
}

// scribble deletes and adds keys in every schema map and applicator map at
// a schema position of v and overwrites every applicator array element, so
// storage shared with another value shows the change.
func scribble(v any) {
	walkSchemas(v, func(obj map[string]any) {
		for key, val := range obj {
			switch sub := val.(type) {
			case map[string]any:
				if key == "properties" || key == "patternProperties" || key == "dependencies" {
					clear(sub)
					sub["x-added"] = true
				}
			case []any:
				if key == "items" || key == "allOf" || key == "anyOf" || key == "oneOf" {
					for i := range sub {
						sub[i] = "x-overwritten"
					}
				}
			}
		}
		clear(obj)
		obj["x-added"] = true
	})
}

type preflightCase struct {
	name    string
	input   string
	wantErr error
	// want is the expected sanitized copy; empty means a copy of input.
	want string
}

func preflightCases() []preflightCase {
	cases := []preflightCase{
		{name: "Empty", input: `{}`},
		{name: "Object", input: `{"type":"object","properties":{"a":{"type":"string"}},"required":["a"],"additionalProperties":false}`},
		{name: "Union", input: `{"type":["string","null"]}`},
		{name: "EveryKeyword", input: everyKeyword},
		{name: "Annotations", input: annotated, want: `{}`},
		{name: "Draft07", input: `{"$schema":"` + draft07 + `","type":"string"}`, want: `{"type":"string"}`},
		{name: "Draft07NoFragment", input: `{"$schema":"http://json-schema.org/draft-07/schema"}`, want: `{}`},
		// Patterns are copied as strings, never compiled.
		{name: "UncompiledPattern", input: `{"pattern":"a{1000}"}`},
		// Property names and literal data are never screened.
		{name: "KeywordPropertyNames", input: `{"properties":{"$ref":{},"$id":{},"definitions":{},"$schema":{}},` +
			`"patternProperties":{"$ref":{}},"required":["$ref","$schema"],"dependencies":{"$id":["$ref"],"definitions":{}}}`},
		{name: "KeywordLiterals", input: `{"const":` + refLiteral + `,"enum":` + literals + `,"properties":{"a":{"const":` + literals + `}}}`},
		{name: "LargeEnum", input: `{"enum":[` + repeatJoin(300, `{"$ref":"#/@","$id":"x"}`) + `]}`},
		{name: "NestedUnknownKeyword", input: `{"properties":{"a":{"x-marker":1}}}`, wantErr: ErrUnsupportedKeyword},
		// Raw JSON rejections pass through unchanged.
		{name: "DuplicateKey", input: `{"type":"string","type":"object"}`, wantErr: ErrDuplicateKey},
		{name: "TrailingData", input: `{} {}`, wantErr: ErrTrailingData},
		{name: "TooLarge", input: `{"title":"` + strings.Repeat("a", 16385-12) + `"}`, wantErr: ErrTooLarge},
		{name: "TooDeep", input: strings.Repeat(`{"not":`, 16) + `{}` + strings.Repeat(`}`, 16), wantErr: ErrTooDeep},
		{name: "NullCharacter", input: `{"title":"\u0000"}`, wantErr: ErrNullCharacter},
		{name: "NumberTooLong", input: `{"minimum":` + strings.Repeat("9", 129) + `}`, wantErr: ErrNumberTooLong},
		{name: "ExponentTooLarge", input: `{"maximum":1e1025}`, wantErr: ErrExponentTooLarge},
	}
	// Malformed shapes are copied unwalked; meta-validation rejects them.
	for _, m := range []string{
		`{"type":5}`, `{"properties":[]}`, `{"required":"a"}`, `{"items":1}`, `{"format":5}`, `{"dependencies":[]}`,
		`{"allOf":{}}`, `{"anyOf":{"$ref":"#"},"not":1,"dependencies":{"a":1},"additionalProperties":"x"}`,
	} {
		cases = append(cases, preflightCase{name: "Malformed" + m, input: m})
	}
	for _, root := range []string{`true`, `[]`, `"marker"`, `1`, `null`} {
		cases = append(cases, preflightCase{name: "Root" + root, input: root, wantErr: ErrInvalidSchema})
	}
	for _, d := range []string{
		`"https://json-schema.org/draft-07/schema#"`, `"http://json-schema.org/draft-04/schema#"`,
		`"http://json-schema.org/draft-06/schema#"`, `"https://json-schema.org/draft/2019-09/schema"`,
		`"https://json-schema.org/draft/2020-12/schema"`, `7`,
	} {
		cases = append(cases, preflightCase{name: "Dialect" + d, input: `{"$schema":` + d + `}`, wantErr: ErrUnsupportedDialect})
	}
	for _, f := range testFormats {
		cases = append(cases, preflightCase{name: "Format" + f, input: `{"items":[{"format":"` + f + `"}]}`})
	}
	for _, f := range []string{"regex", "int32", "duration", "expose"} {
		cases = append(cases, preflightCase{name: "Format" + f, input: `{"items":[{"format":"` + f + `"}]}`, wantErr: ErrUnsupportedFormat})
	}
	// Forbidden and unknown keywords at every schema position. A root
	// $schema that is not a supported URI is a dialect error instead.
	for _, kw := range []string{"$ref", "$id", "id", "$defs", "definitions", "$schema", "nullable"} {
		for _, wrap := range schemaPositions {
			want := ErrUnsupportedKeyword
			if kw == "$schema" && wrap == "@" {
				want = ErrUnsupportedDialect
			}
			input := strings.ReplaceAll(wrap, "@", `{"`+kw+`":{"marker":"#"}}`)
			cases = append(cases, preflightCase{name: kw + wrap, input: input, wantErr: want})
		}
	}
	return cases
}

func TestPreflightSchema(t *testing.T) {
	t.Parallel()

	for _, tt := range preflightCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := preflightSchema([]byte(tt.input))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				// Fixed sentinel text never echoes input.
				require.Equal(t, tt.wantErr.Error(), err.Error())
				return
			}
			require.NoError(t, err)
			want := tt.want
			if want == "" {
				want = tt.input
			}
			wantSanitized, err := ParseSchemaDocument([]byte(want))
			require.NoError(t, err)
			require.Equal(t, wantSanitized, got.sanitized)
			fresh, err := ParseSchemaDocument([]byte(tt.input))
			require.NoError(t, err)
			require.Equal(t, fresh, got.document)
			scribble(got.sanitized)
			require.Equal(t, fresh, got.document)
		})
	}
}

func TestPreflightSchemaSanitizedCopy(t *testing.T) {
	t.Parallel()

	// Annotations are omitted at every schema position.
	for _, wrap := range schemaPositions {
		got, err := preflightSchema([]byte(strings.ReplaceAll(wrap, "@", annotated)))
		require.NoError(t, err, wrap)
		want, err := ParseSchemaDocument([]byte(strings.ReplaceAll(wrap, "@", `{}`)))
		require.NoError(t, err)
		require.Equal(t, want, got.sanitized, wrap)
	}

	raw := []byte(`{"$schema":"` + draft07 + `","title":"t","$comment":"c",` +
		`"properties":{"$ref":{"description":"d","const":` + refLiteral + `},` +
		`"$schema":{"enum":[{"$id":"z","title":"kept"},"$ref"]},"definitions":{"default":{"$ref":"#"},"examples":[{}]}},` +
		`"required":["$ref","$schema"],"dependencies":{"$ref":["$id","definitions"],"$id":{"title":"x","readOnly":true}},` +
		`"items":[{"writeOnly":true,"contentMediaType":"a","contentEncoding":"b"}],` +
		`"allOf":[{"patternProperties":{"^a":{"description":"d","const":{"$ref":"#"}}}}]}`)
	want, err := ParseSchemaDocument([]byte(`{"properties":{"$ref":{"const":` + refLiteral + `},` +
		`"$schema":{"enum":[{"$id":"z","title":"kept"},"$ref"]},"definitions":{}},` +
		`"required":["$ref","$schema"],"dependencies":{"$ref":["$id","definitions"],"$id":{}},` +
		`"items":[{}],"allOf":[{"patternProperties":{"^a":{"const":{"$ref":"#"}}}}]}`))
	require.NoError(t, err)

	got, err := preflightSchema(raw)
	require.NoError(t, err)
	require.Equal(t, want, got.sanitized)
	fresh, err := ParseSchemaDocument(raw)
	require.NoError(t, err)
	require.Equal(t, fresh, got.document)
	scribble(got.sanitized)
	require.Equal(t, fresh, got.document)
}

func FuzzPreflightSchema(f *testing.F) {
	for _, tt := range preflightCases() {
		f.Add([]byte(tt.input))
	}
	f.Add([]byte(`{"properties":{"$ref":` + annotated + `},"allOf":[{"not":{"dependencies":{"a":{"$ref":"#"},"b":["$id"]}}}]}`))
	sentinels := []error{
		ErrInvalidSchema, ErrUnsupportedDialect, ErrUnsupportedKeyword, ErrUnsupportedFormat,
		ErrTooLarge, ErrInvalidUTF8, ErrMalformed, ErrTrailingData, ErrTooDeep, ErrTooManyNodes, ErrArrayTooLong,
		ErrDuplicateKey, ErrNullCharacter, ErrUnpairedSurrogate, ErrNumberTooLong, ErrExponentTooLarge,
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		got, err := preflightSchema(raw)
		if err != nil {
			require.True(t, slices.ContainsFunc(sentinels, func(s error) bool { return errors.Is(err, s) }), err)
			return
		}
		walkSchemas(got.sanitized, func(obj map[string]any) {
			for key, val := range obj {
				require.Contains(t, allowedKeywords, key)
				if format, ok := val.(string); ok && key == "format" {
					require.Contains(t, testFormats, format)
				}
			}
		})
		// A second algorithm: strip the omitted keywords from a fresh parse.
		want, err := ParseSchemaDocument(raw)
		require.NoError(t, err)
		walkSchemas(want, func(obj map[string]any) {
			for _, key := range omittedKeywords {
				delete(obj, key)
			}
		})
		require.Equal(t, want, got.sanitized)
		scribble(got.sanitized)
		fresh, err := ParseSchemaDocument(raw)
		require.NoError(t, err)
		require.Equal(t, fresh, got.document)
	})
}

// BenchmarkPreflightSchema measures an accepted schema near the 16 KiB cap
// that mixes every keyword kind, annotations and literal data.
func BenchmarkPreflightSchema(b *testing.B) {
	member := `"p@":{` + annotationMembers + `,` + everyKeyword[1:]
	build := func(n int) []byte { return []byte(`{"properties":{` + repeatJoin(n, member) + `}}`) }
	n := 1
	for len(build(n+1)) <= 16<<10 {
		n++
	}
	raw := build(n)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		if _, err := preflightSchema(raw); err != nil {
			b.Fatal(err)
		}
	}
}
