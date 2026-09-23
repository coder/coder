package chatstructured

import (
	"errors"
	"maps"
	"regexp/syntax"
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

// exactInsts counts the program regexp.Compile builds for p, independently
// of the preflight's estimate.
func exactInsts(t testing.TB, p string) int {
	t.Helper()
	re, err := syntax.Parse(p, syntax.Perl)
	require.NoError(t, err, p)
	prog, err := syntax.Compile(re.Simplify())
	require.NoError(t, err, p)
	return len(prog.Inst)
}

func estimate(t testing.TB, p string) int {
	t.Helper()
	re, err := syntax.Parse(p, syntax.Perl)
	require.NoError(t, err, p)
	return estimateInsts(re)
}

// capSchema sits at every schema cap: 256 nodes, 8 branches, nesting 4, and
// 4 patternProperties patterns, with 128-instruction patterns throughout.
func capSchema() string {
	return `{"not":{"not":{"not":{"not":{}}}},"anyOf":[{},{},{},{}],` +
		`"patternProperties":{"a{126}":{},"b{126}":{},"c{126}":{},"d{126}":{}},"properties":{` +
		repeatJoin(243, `"p@":{"pattern":"a{126}"}`) + `}}`
}

// literalOnly holds keyword-shaped literal data and property names that
// would exceed every limit if they were counted.
var literalOnly = `{"enum":[` + repeatJoin(300, `{"allOf":[@],"patternProperties":{"(":1}}`) + `],` +
	`"const":{"anyOf":[{},{},{},{},{},{},{},{},{}]},"examples":[{"patternProperties":{"a{1000}":{}}}],` +
	`"default":{"not":{"not":{"not":{"not":{"not":{"pattern":"a{1000}"}}}}}},` +
	`"properties":{"allOf":{},"patternProperties":{},"not":{}},"required":["patternProperties"]}`

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
		// Counted repetitions expand when compiled.
		{name: "ExpandingPattern", input: `{"pattern":"a{1000}"}`, wantErr: ErrSchemaTooComplex},
		// Property names and literal data are never screened.
		{name: "KeywordPropertyNames", input: `{"properties":{"$ref":{},"$id":{},"definitions":{},"$schema":{}},` +
			`"patternProperties":{"$ref":{}},"required":["$ref","$schema"],"dependencies":{"$id":["$ref"],"definitions":{}}}`},
		{name: "KeywordLiterals", input: `{"const":` + refLiteral + `,"enum":` + literals + `,"properties":{"a":{"const":` + literals + `}}}`},
		{name: "LargeEnum", input: `{"enum":[` + repeatJoin(300, `{"$ref":"#/@","$id":"x"}`) + `]}`},
		{name: "NestedUnknownKeyword", input: `{"properties":{"a":{"x-marker":1}}}`, wantErr: ErrUnsupportedKeyword},
		{name: "SortedKeys", input: `{"$ref":"#","format":"regex"}`, wantErr: ErrUnsupportedKeyword},
		{name: "SortedMembers", input: `{"properties":{"a":{"format":"regex"},"b":{"$ref":"#"}}}`, wantErr: ErrUnsupportedFormat},
		{name: "SortedBranches", input: `{"allOf":[{"format":"regex"},{"$ref":"#"}],"anyOf":[{"x":1}]}`, wantErr: ErrUnsupportedFormat},
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
				for range 100 {
					_, again := preflightSchema([]byte(tt.input))
					require.Equal(t, err, again)
				}
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

func TestPreflightSchemaLimits(t *testing.T) {
	t.Parallel()

	// Each pair trips only the intended limit; expectations are literal.
	check := func(input string, wantErr error) {
		t.Helper()
		_, err := preflightSchema([]byte(input))
		if wantErr == nil {
			require.NoError(t, err, input)
			return
		}
		require.ErrorIs(t, err, wantErr, input)
		require.Equal(t, wantErr.Error(), err.Error())
	}
	tooComplex := ErrSchemaTooComplex
	check(capSchema(), nil)
	check(literalOnly, nil)

	// The root plus n object or boolean schemas.
	check(`{"properties":{`+repeatJoin(255, `"p@":{}`)+`}}`, nil)
	check(`{"properties":{`+repeatJoin(256, `"p@":{}`)+`}}`, tooComplex)
	check(`{"items":[`+repeatJoin(255, `true`)+`]}`, nil)
	check(`{"items":[`+repeatJoin(256, `false`)+`]}`, tooComplex)

	anyOf := func(n int, elem string) string { return `{"anyOf":[` + repeatJoin(n, elem) + `]}` }
	check(anyOf(8, `{}`), nil)
	check(anyOf(9, `{}`), tooComplex)
	// Malformed branches count but are not walked.
	check(anyOf(8, `1`), nil)
	check(anyOf(9, `1`), tooComplex)
	// Array-form dependencies are literal data and do not count.
	mixed := `{"allOf":[{},{}],"oneOf":[true],"not":{},"if":{},"then":{},"else":false,"dependencies":{"a":{},"b":["a"]`
	check(mixed+`}}`, nil)
	check(mixed+`,"c":true}}`, tooComplex)

	notChain := func(n int) string { return strings.Repeat(`{"not":`, n) + `{}` + strings.Repeat(`}`, n) }
	check(notChain(4), nil)
	check(notChain(5), tooComplex)
	// Other keywords keep the level.
	check(strings.Replace(notChain(4), `{}`, `{"properties":{"a":{"items":[{"additionalProperties":{}}]}}}`, 1), nil)
	// allOf, dependencies, if and anyOf add one level; properties keeps it.
	chain := `{"allOf":[{"dependencies":{"a":{"if":{"properties":{"p":{"anyOf":[@]}}}}}}]}`
	check(strings.Replace(chain, "@", `{}`, 1), nil)
	check(strings.Replace(chain, "@", `{"not":{}}`, 1), tooComplex)

	// A class compiles to one instruction, so only the length trips.
	class := func(body string) string { return `[` + body + `]` }
	require.Len(t, class(strings.Repeat("é", 127)), 256)
	for _, p := range []string{class(strings.Repeat("a", 254)), class(strings.Repeat("é", 127))} {
		check(`{"pattern":"`+p+`"}`, nil)
		check(`{"patternProperties":{"`+p+`":{}}}`, nil)
	}
	for _, p := range []string{class(strings.Repeat("a", 255)), class("a" + strings.Repeat("é", 127))} {
		require.Len(t, p, 257)
		check(`{"pattern":"`+p+`"}`, tooComplex)
		check(`{"patternProperties":{"`+p+`":{}}}`, tooComplex)
	}

	require.Equal(t, 128, exactInsts(t, "a{126}"))
	require.Equal(t, 129, exactInsts(t, "a{127}"))
	// The estimate admits both, so the exact count decides.
	require.LessOrEqual(t, estimate(t, "a{127}"), 1024)
	check(`{"pattern":"a{126}"}`, nil)
	check(`{"pattern":"a{127}"}`, tooComplex)
	check(`{"patternProperties":{"a{126}":{}}}`, nil)
	check(`{"patternProperties":{"a{127}":{}}}`, tooComplex)
	// The estimate rejects before compiling.
	require.Greater(t, estimate(t, "(a{1000})"), 1024)
	check(`{"pattern":"(a{1000})"}`, tooComplex)
	check(`{"patternProperties":{"(a{1000})":{}}}`, tooComplex)

	check(`{"pattern":"("}`, ErrInvalidSchema)
	check(`{"patternProperties":{"(":{}}}`, ErrInvalidSchema)
	check(`{"pattern":"((a{1000}){1000}){1000}"}`, ErrInvalidSchema)

	pp := `{"patternProperties":{"^a":{"patternProperties":{"^b":{}}}},"properties":{"x":{"patternProperties":{"^c":{}}}},` +
		`"anyOf":[{"patternProperties":{"^d":{}}}]`
	check(pp+`}`, nil)
	check(pp+`,"items":{"patternProperties":{"^e":{}}}}`, tooComplex)
}

func TestPreflightSchemaPatternPropertiesFlag(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]bool{
		`{}`:                       false,
		`{"patternProperties":{}}`: true,
		`{"items":{"patternProperties":{"^a":{}}}}`:                                          true,
		`{"items":[{},{"patternProperties":{}}]}`:                                            true,
		`{"allOf":[{"not":{"patternProperties":{}}}]}`:                                       true,
		`{"dependencies":{"a":{"patternProperties":{}}}}`:                                    true,
		`{"properties":{"patternProperties":{}},"dependencies":{"patternProperties":["a"]}}`: false,
		literalOnly: false,
	} {
		got, err := preflightSchema([]byte(input))
		require.NoError(t, err, input)
		require.Equal(t, want, got.patternProperties, input)
	}
}

func TestEstimateInsts(t *testing.T) {
	t.Parallel()

	for _, p := range []string{`^[a-z0-9_-]{1,40}$`, `^\d{3}-\d{4}$`, `^x-[a-z]+$`, `^[A-Z]{2}$`, `^\S+@\S+$`} {
		require.NoError(t, checkPattern(p), p)
	}
	corpus := []string{
		"", "a", `\bfoo\B`, "(?i)héllo|wörld", "日本{3,5}", `[^\p{Greek}]+?`, "(a|bc|d)*", "(a*)*", "(|a)+",
		"(a?){3,}", "a{0}", "a{0,}", "a{1000}", "(?:(?:x|yz){1,3}z){2}", "((a{2}){3}){4}", "(?:(a)|b){2,7}?",
		"^(?:[0-9a-f]{2}:){5}[0-9a-f]{2}$", "(?s).{2,}(?m)^$",
	}
	quantifiers := []string{"", "*", "+?", "?", "{3}", "{2,}", "{1,4}", "{0,2}", "{0}"}
	for _, atom := range []string{"a", "[ab]", "(x|yz)", "(?:)", "é", "(a*)", "^"} {
		for _, q1 := range quantifiers {
			for _, q2 := range quantifiers {
				corpus = append(corpus, "(?:"+atom+q1+"b)"+q2, "("+atom+q1+"|c)"+q2+"$")
			}
		}
	}
	checked := 0
	for _, p := range corpus {
		if est := estimate(t, p); est <= 1024 {
			require.GreaterOrEqual(t, est, exactInsts(t, p), p)
			checked++
		}
	}
	require.Greater(t, checked, len(corpus)/2)
}

// recount recounts limits over a sanitized copy independently of the
// preflight's walker. schema reports whether v is a schema.
type recount struct {
	nodes, branches, nesting, patterns int
	patternProperties                  bool
}

func (c *recount) schema(t *testing.T, v any, level int) bool {
	obj, isObject := v.(map[string]any)
	if _, isBool := v.(bool); !isObject && !isBool {
		return false
	}
	c.nodes++
	c.nesting = max(c.nesting, level)
	for key, val := range obj {
		// subs holds the values at schema positions below key; every one
		// counts when branch is 1.
		subs, branch := []any{}, 0
		switch key {
		case "pattern":
			if p, ok := val.(string); ok {
				c.pattern(t, p)
			}
		case "not", "if", "then", "else":
			subs, branch = []any{val}, 1
		case "allOf", "anyOf", "oneOf":
			subs, _ = val.([]any)
			branch = 1
		case "items", "additionalItems", "additionalProperties", "contains", "propertyNames":
			if list, ok := val.([]any); ok && key == "items" {
				subs = list
			} else {
				subs = []any{val}
			}
		case "properties", "patternProperties", "dependencies":
			c.patternProperties = c.patternProperties || key == "patternProperties"
			members, _ := val.(map[string]any)
			for name, sub := range members {
				if key == "patternProperties" {
					c.patterns++
					c.pattern(t, name)
				}
				if key != "dependencies" {
					subs = append(subs, sub)
				} else if c.schema(t, sub, level+1) {
					c.branches++ // Only schema-form dependencies branch.
				}
			}
		}
		c.branches += branch * len(subs)
		for _, sub := range subs {
			c.schema(t, sub, level+branch)
		}
	}
	return true
}

func (*recount) pattern(t *testing.T, p string) {
	require.LessOrEqual(t, len(p), 256)
	require.LessOrEqual(t, exactInsts(t, p), 128)
}

func FuzzPreflightSchema(f *testing.F) {
	for _, tt := range preflightCases() {
		f.Add([]byte(tt.input))
	}
	for _, seed := range []string{
		`{"properties":{"$ref":` + annotated + `},"allOf":[{"not":{"dependencies":{"a":{"$ref":"#"},"b":["$id"]}}}]}`,
		capSchema(), literalOnly, `{"pattern":"a{127}"}`, `{"pattern":"((a{2}){3}|[\\p{L}]+?){1,4}"}`, `{"pattern":"(a{1000})"}`,
		`{"patternProperties":{"(?i)^x-[a-z]{1,40}$":{"not":{"dependencies":{"a":{"items":[true,{"pattern":"^\\d+$"}]}}}}}}`,
	} {
		f.Add([]byte(seed))
	}
	sentinels := []error{
		ErrInvalidSchema, ErrUnsupportedDialect, ErrUnsupportedKeyword, ErrUnsupportedFormat, ErrSchemaTooComplex,
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
		var c recount
		c.schema(t, got.sanitized, 0)
		require.LessOrEqual(t, c.nodes, 256)
		require.LessOrEqual(t, c.branches, 8)
		require.LessOrEqual(t, c.nesting, 4)
		require.LessOrEqual(t, c.patterns, 4)
		require.Equal(t, c.patternProperties, got.patternProperties)
		scribble(got.sanitized)
		fresh, err := ParseSchemaDocument(raw)
		require.NoError(t, err)
		require.Equal(t, fresh, got.document)
	})
}

// BenchmarkPreflightSchema measures an accepted schema near the 16 KiB cap
// that mixes every keyword kind, annotations and literal data, the accepted
// schema at every cap, a maximal pattern that the estimate rejects before
// compiling, and a pattern that the exact count rejects after compiling.
func BenchmarkPreflightSchema(b *testing.B) {
	build := func(n int) string {
		return `{"properties":{"k":` + everyKeyword + `,` + repeatJoin(n, `"p@":`+annotated) + `}}`
	}
	n := 1
	for len(build(n+1)) <= 16<<10 {
		n++
	}
	for _, bc := range []struct {
		name  string
		input string
		want  error
	}{
		{name: "Vocabulary", input: build(n)},
		{name: "AtCaps", input: capSchema()},
		{name: "EstimateRejection", input: `{"pattern":"(?:` + strings.Repeat("[a-z]", 49) + `){1000}"}`, want: ErrSchemaTooComplex},
		{name: "CompileRejection", input: `{"pattern":"a{500}"}`, want: ErrSchemaTooComplex},
	} {
		b.Run(bc.name, func(b *testing.B) {
			raw := []byte(bc.input)
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			for b.Loop() {
				if _, err := preflightSchema(raw); !errors.Is(err, bc.want) {
					b.Fatal(err)
				}
			}
		})
	}
}
