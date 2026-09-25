package chatstructured

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
	"golang.org/x/xerrors"
)

func mustCompile(t testing.TB, schema string) *Schema {
	t.Helper()
	s, err := CompileSchema([]byte(schema))
	require.NoError(t, err, schema)
	return s
}

// objectValue is an object with n keys, n+1 nodes.
func objectValue(n int) []byte { return []byte(`{` + repeatJoin(n, `"k@":0`) + `}`) }

// largest returns the largest build(n) that fits the 16 KiB schema cap.
func largest(build func(int) string) string {
	n := 1
	for len(build(n+1)) <= 16<<10 {
		n++
	}
	return build(n)
}

func TestCompileSchema(t *testing.T) {
	t.Parallel()
	rejects := func(want error, inputs ...string) {
		for _, input := range inputs {
			_, err := CompileSchema([]byte(input))
			require.ErrorIs(t, err, want, input)
			require.Equal(t, want.Error(), err.Error())
		}
	}
	// Meta-validation rejects what the preflight copies unchecked.
	rejects(ErrInvalidSchema, `{"type":5}`, `{"pattern":5}`, `{"properties":[]}`, `{"required":"a"}`, `{"items":1}`, `{"format":5}`,
		`{"dependencies":[]}`, `{"allOf":{}}`, `{"not":1}`, `{"title":5}`, `{"required":["a","a"]}`, `{"minLength":-1}`)
	// Numbers beyond float64 would crash the library's meta-validation.
	rejects(ErrNumberOutOfRange, `{"type":1e400}`, `{"type":[1e400]}`, `{"required":[1e400]}`, `{"dependencies":{"a":[1e400]}}`, `{"enum":[1e400]}`, `{"maximum":1.7976931348623159e308}`)
	// Preflight rejections pass through unchanged.
	rejects(ErrUnsupportedKeyword, `{"$ref":"#"}`)
	for _, input := range []string{
		`{}`, `{"type":"object","properties":{"a":{"type":"string"}},"required":["a"]}`, `{"items":[true,false],"not":false}`,
		`{"$schema":"` + draft07 + `"}`, `{"$schema":"http://json-schema.org/draft-07/schema"}`, everyKeyword, capSchema(),
	} {
		mustCompile(t, input)
	}
	// Literal identifiers compile with zero loads and stay data only.
	lit := `{"$ref":"http://127.0.0.1:1/x","$id":"` + draft07 + `","id":"file:///etc/passwd"}`
	s := mustCompile(t, `{"const":`+lit+`,"enum":[`+lit+`],"default":`+lit+`,"examples":[`+lit+`],`+
		`"properties":{"$ref":{"type":"string"},"$id":{"const":"`+draft07+`"},"id":{}}}`)
	_, err := s.Validate([]byte(lit))
	require.NoError(t, err)
	_, err = s.Validate([]byte(`{"$ref":"#"}`))
	require.ErrorIs(t, err, ErrValueMismatch)
	rejects(ErrInvalidSchema, `{"type":5}`)
}

type trapTransport struct{ trips atomic.Int64 }

func (tr *trapTransport) RoundTrip(*http.Request) (*http.Response, error) {
	tr.trips.Add(1)
	return nil, xerrors.New("network access trapped")
}

//nolint:paralleltest // Swaps the process-global http.DefaultTransport.
func TestCompileNeverLoads(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	file := filepath.Join(t.TempDir(), "s.json")
	require.NoError(t, os.WriteFile(file, []byte(`{}`), 0o600))
	// The library's default loader does reach the server.
	_, err := gojsonschema.NewSchema(gojsonschema.NewStringLoader(`{"$ref":"` + srv.URL + `/s.json"}`))
	require.NoError(t, err)
	require.EqualValues(t, 1, hits.Load())
	trap := &trapTransport{}
	orig := http.DefaultTransport
	http.DefaultTransport = trap
	t.Cleanup(func() { http.DefaultTransport = orig })
	// Each reference would load if any loader were reachable.
	for _, input := range []string{
		`{"$ref":"` + srv.URL + `/s.json"}`, `{"properties":{"a":{"$ref":"file://` + file + `"}}}`, `{"$ref":"http://example.com/s.json"}`,
		`{"$id":"` + srv.URL + `/base.json","items":{"$ref":"other.json"}}`, `{"allOf":[{"$ref":"` + draft07 + `"}]}`,
	} {
		doc, err := ParseSchemaDocument([]byte(input))
		require.NoError(t, err)
		_, err = compileDenyAll(doc.(map[string]any))
		require.ErrorIs(t, err, errLoadDenied, input)
		require.ErrorIs(t, err, ErrInvalidSchema, input)
	}
	// A relative reference without an absolute base fails a canonical check before any load.
	_, err = compileDenyAll(map[string]any{"$ref": "other.json"})
	require.ErrorIs(t, err, ErrInvalidSchema)
	require.NotErrorIs(t, err, errLoadDenied)
	_, err = compileMetaSchema()
	require.NoError(t, err)
	for _, schema := range []string{everyKeyword, capSchema(), `{"format":"uri","const":{"$ref":"http://x/"}}`} {
		_, _ = mustCompile(t, schema).Validate([]byte(`{"a":"http://example.com/"}`))
	}
	require.EqualValues(t, 1, hits.Load())
	require.Zero(t, trap.trips.Load())
	// The trap does see the library's default loader.
	_, err = gojsonschema.NewSchema(gojsonschema.NewStringLoader(`{"$ref":"http://example.com/s.json"}`))
	require.Error(t, err)
	require.EqualValues(t, 1, trap.trips.Load())
}

func TestSchemaValidate(t *testing.T) {
	t.Parallel()
	type vcase struct {
		schema         string
		accept, reject []string
	}
	cases := []vcase{
		{`{"type":"object","properties":{"a":{"type":"array","items":{"type":["string","null"]}}},"required":["a"],"additionalProperties":false}`, []string{`{"a":[]}`, `{"a":["x",null]}`}, []string{`{}`, `{"a":[1]}`, `{"a":[],"b":1}`, `null`}},
		{`{"type":"null"}`, []string{`null`}, []string{`0`}},
		{`{"enum":["a",1]}`, []string{`"a"`, `1`}, []string{`"b"`}},
		{`{"const":{"a":[1]}}`, []string{`{"a":[1]}`}, []string{`{"a":[2]}`}},
		{`{"oneOf":[{"type":"string"},{"maxLength":2}]}`, []string{`"abc"`, `5`}, []string{`"ab"`}},
		{`{"if":{"type":"string"},"then":{"minLength":2},"else":{"type":"number"}}`, []string{`"ab"`, `1`}, []string{`"a"`, `true`}},
		{`{"dependencies":{"a":["b"],"b":{"required":["c"]}}}`, []string{`{}`, `{"a":1,"b":1,"c":1}`}, []string{`{"a":1}`, `{"b":1}`}},
		{`{"patternProperties":{"^x-":{"type":"string"}},"additionalProperties":false}`, []string{`{"x-a":"s"}`}, []string{`{"x-a":1}`, `{"y":"s"}`}},
		{`{"multipleOf":0.1}`, []string{`0.3`}, []string{`0.35`}},
		{`{"maximum":9007199254740992,"exclusiveMinimum":0.5}`, []string{`9007199254740992`, `0.5000000000000000001`}, []string{`9007199254740993`, `0.5`}},
		{`{"maximum":1.7976931348623157e308,"minimum":-1e-400}`, []string{`1.7976931348623157e308`, `1e-400`}, []string{`-1`}},
		{`{"const":9007199254740993}`, []string{`9007199254740992`}, []string{`9007199254740995`}}, // float64 precision
	}
	for f, pair := range map[string][2]string{
		"date": {"2024-01-31", "2024-13-01"}, "time": {"12:34:56Z", "25:00:00Z"}, "hostname": {"example.com", "-bad-.com"},
		"date-time": {"2024-01-31T12:34:56Z", "2024-01-31T25:00:00Z"}, "uuid": {"123e4567-e89b-12d3-a456-426614174000", "123e4567"},
		"email": {"a@example.com", "no-at-sign"}, "idn-email": {"a@example.com", "no-at-sign"}, "ipv4": {"192.0.2.1", "256.0.0.1"},
		"uri": {"https://example.com/x", "relative/path"}, "iri": {"https://example.com/x", "relative/path"}, "ipv6": {"2001:db8::1", "192.0.2.1"},
		"uri-reference": {"/x#frag", "%zz"}, "iri-reference": {"/x#frag", "%zz"}, "json-pointer": {"/a/b", "a/b"}, "relative-json-pointer": {"0/a", "/a"},
		"uri-template": {"https://example.com/{id}", "https://example.com/{id"},
	} {
		cases = append(cases, vcase{`{"format":"` + f + `"}`, []string{`"` + pair[0] + `"`}, []string{`"` + pair[1] + `"`}})
	}
	for _, tt := range cases {
		s := mustCompile(t, tt.schema)
		for _, v := range tt.accept {
			raw := []byte(v)
			got, err := s.Validate(raw)
			require.NoError(t, err, "%s accepts %s", tt.schema, v)
			want, _ := ParseOutputValue([]byte(v))
			require.Equal(t, want, got)
			require.Equal(t, v, string(raw))
		}
		for _, v := range tt.reject {
			_, err := s.Validate([]byte(v))
			require.ErrorIs(t, err, ErrValueMismatch, "%s rejects %s", tt.schema, v)
		}
	}
}

func TestSchemaValidateCapsAndFeedback(t *testing.T) {
	t.Parallel()
	// Numbers beyond float64 are rejected before the library converts them.
	s := mustCompile(t, `{"anyOf":[{"const":0},{"enum":[0]},{"uniqueItems":true},{"properties":{"a":{"properties":{"b":{"const":1}}}}}]}`)
	for raw, want := range map[string]error{
		`"` + strings.Repeat("a", 64<<10) + `"`: ErrTooLarge, strings.Repeat("[", 33) + strings.Repeat("]", 33): ErrTooDeep,
		string(objectValue(4096)): ErrTooManyNodes, `[` + repeatJoin(257, "0") + `]`: ErrArrayTooLong, string(objectValue(4095)): nil,
		`1e400`: ErrNumberOutOfRange, `[1e400,1]`: ErrNumberOutOfRange, `{"a":{"b":-1e309}}`: ErrNumberOutOfRange, `1.7976931348623159e308`: ErrNumberOutOfRange,
	} {
		_, err := s.Validate([]byte(raw))
		require.ErrorIs(t, err, want)
	}
	for raw, want := range map[string]error{string(objectValue(255)): nil, string(objectValue(256)): ErrTooManyNodes} {
		_, err := mustCompile(t, `{"patternProperties":{"^k":{}}}`).Validate([]byte(raw))
		require.ErrorIs(t, err, want)
	}

	// Duplicate violations of 6 long multibyte names with a distinctive value.
	raw := []byte(`{` + repeatJoin(6, `"@`+strings.Repeat("é", 300)+`":123456789`) + `}`)
	strict := mustCompile(t, `{"additionalProperties":{"allOf":[{"type":"string"},{"type":"string"}]}}`)
	_, err := strict.Validate(raw)
	var verr *ValidationError
	require.ErrorAs(t, err, &verr)
	require.ErrorIs(t, err, ErrValueMismatch)
	require.Len(t, verr.Issues, 4)
	// Issues are distinct and ordered; paths are cut at a rune boundary within 160 bytes.
	for i, issue := range verr.Issues {
		require.Equal(t, ValidationIssue{Path: fmt.Sprint(i/2) + strings.Repeat("é", 79), Rule: []string{"invalid_type", "number_all_of"}[i%2]}, issue)
	}
	msg := err.Error()
	require.LessOrEqual(t, len(msg), 1024)
	require.True(t, utf8.ValidString(msg))
	require.NotContains(t, msg, "123456789")
	for range 20 {
		_, again := strict.Validate(raw)
		require.Equal(t, msg, again.Error())
	}
	// One compiled Schema and the shared meta-schema serve many goroutines.
	s = mustCompile(t, `{"type":"object","patternProperties":{"^a":{"type":"string","format":"email"}}}`)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			_, err := s.Validate([]byte(`{"a":"x@example.com"}`))
			assert.NoError(t, err)
			_, err = s.Validate([]byte(`{"a":1}`))
			assert.ErrorIs(t, err, ErrValueMismatch)
			_, err = CompileSchema([]byte(everyKeyword))
			assert.NoError(t, err)
		})
	}
	wg.Wait()
}

func FuzzCompileSchemaValidate(f *testing.F) {
	for _, seed := range [][2]string{
		{everyKeyword, `{"a":"x@example.com"}`},
		{`{"$ref":"#"}`, `{}`},
		{`{"patternProperties":{"^k":{"type":"integer"}}}`, `{"k1":1,"k2":"x"}`},
		{`{"items":{"allOf":[{"type":"string"},{"maxLength":2}]}}`, `["abc",1,"ab"]`},
		{`{"const":1e400}`, `[1e400,{"a":-1e309}]`},
	} {
		f.Add([]byte(seed[0]), []byte(seed[1]))
	}
	f.Fuzz(func(t *testing.T, schema, value []byte) {
		s, err := CompileSchema(schema)
		if err != nil {
			return
		}
		got, err := s.Validate(value)
		again, errAgain := s.Validate(value)
		require.Equal(t, got, again)
		require.Equal(t, fmt.Sprint(err), fmt.Sprint(errAgain))
		require.LessOrEqual(t, len(fmt.Sprint(err)), 1024)
	})
}

// BenchmarkSchema measures CompileSchema at every cap padded to 16 KiB and on a
// meta-validation heavy schema (no value), and Validate at the caps: 4096 nodes
// failing all 8 branches, 256 nodes against 4 patterns of 128 instructions, and
// nested uniqueItems arrays.
func BenchmarkSchema(b *testing.B) {
	for _, bc := range []struct {
		name, schema, value string
		want                error
	}{
		{"CompileAtCaps", largest(func(n int) string { return `{"description":"` + strings.Repeat("d", n) + `",` + capSchema()[1:] }), "", nil},
		{"CompileMetaValidation", largest(func(n int) string { return `{"required":[` + repeatJoin(n, `"r@"`) + `]}` }), "", nil},
		{
			"ValidateBranches", `{"items":{"items":{"allOf":[` + repeatJoin(8, `{"type":"string"}`) + `]}}}`,
			`[` + repeatJoin(15, `[`+repeatJoin(255, "0")+`]`) + `,[` + repeatJoin(254, "0") + `]]`, ErrValueMismatch,
		},
		{"ValidatePatternProperties", `{"patternProperties":{"a{126}":{},"b{126}":{},"c{126}":{},"d{126}":{}}}`, string(objectValue(255)), nil},
		{"ValidateUniqueItems", `{"uniqueItems":true,"items":{"uniqueItems":true}}`, `[` + repeatJoin(256, `[@,`+repeatJoin(13, "1000@")+`]`) + `]`, nil},
	} {
		b.Run(bc.name, func(b *testing.B) {
			raw, run := []byte(bc.schema), func(raw []byte) error { _, err := CompileSchema(raw); return err }
			if bc.value != "" {
				s := mustCompile(b, bc.schema)
				raw, run = []byte(bc.value), func(raw []byte) error { _, err := s.Validate(raw); return err }
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			for b.Loop() {
				if err := run(raw); !errors.Is(err, bc.want) {
					b.Fatal(err)
				}
			}
		})
	}
}
