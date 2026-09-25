package chatstructured_test

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
)

// nested returns n arrays nested inside each other: depth n, n nodes.
func nested(n int) string {
	return strings.Repeat("[", n) + strings.Repeat("]", n)
}

// object returns an object with members members holding 0: members+1 nodes.
func object(members int) string {
	parts := make([]string, members)
	for i := range parts {
		parts[i] = `"k` + strconv.Itoa(i) + `":0`
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// array returns an array of elems zeros: elems+1 nodes.
func array(elems int) string {
	return "[" + strings.TrimSuffix(strings.Repeat("0,", elems), ",") + "]"
}

func TestParseOutputValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    any
		wantErr error
	}{
		{name: "Object", input: `{"a":1}`, want: map[string]any{"a": json.Number("1")}},
		{name: "Array", input: `[1,"x",true,null]`, want: []any{json.Number("1"), "x", true, nil}},
		{name: "EmptyArray", input: `[]`, want: []any{}},
		{name: "EmptyObject", input: `{}`, want: map[string]any{}},
		{name: "Null", input: `null`, want: nil},
		{name: "Whitespace", input: " \n{\"a\" : [ 1 , 2 ] }\t\n", want: map[string]any{"a": []any{json.Number("1"), json.Number("2")}}},
		{name: "EmptyKey", input: `{"":1}`, want: map[string]any{"": json.Number("1")}},
		{name: "CaseDistinctKeys", input: `{"a":1,"A":2}`, want: map[string]any{"a": json.Number("1"), "A": json.Number("2")}},
		// Numbers keep their literal text instead of rounding through float64.
		{name: "BeyondFloat64Integer", input: `9007199254740993`, want: json.Number("9007199254740993")},
		{name: "FractionalPrecision", input: `-0.10000000000000001`, want: json.Number("-0.10000000000000001")},
		{name: "ExponentBeyondFloat64", input: `1.50E+400`, want: json.Number("1.50E+400")},
		// Exact numeric caps: 128 literal bytes and |exponent| 1024.
		{name: "MaxLengthInteger", input: strings.Repeat("9", 128), want: json.Number(strings.Repeat("9", 128))},
		{name: "MaxLengthNegative", input: "-" + strings.Repeat("9", 127), want: json.Number("-" + strings.Repeat("9", 127))},
		{name: "MaxExponent", input: `1e1024`, want: json.Number("1e1024")},
		{name: "MaxNegativeExponent", input: `-1.5E-1024`, want: json.Number("-1.5E-1024")},
		{name: "ExponentLeadingZeros", input: "1e+" + strings.Repeat("0", 100) + "1024", want: json.Number("1e+" + strings.Repeat("0", 100) + "1024")},
		// Unicode that must stay valid.
		{name: "SurrogatePairEscape", input: `"\ud83d\ude00"`, want: "\U0001F600"},
		{name: "LiteralAstral", input: "\"\U0001F600\"", want: "\U0001F600"},
		{name: "EscapedReplacementCharacter", input: `"\ufffd"`, want: "\uFFFD"},
		{name: "LiteralReplacementCharacter", input: "\"\uFFFD\"", want: "\uFFFD"},
		{name: "EscapedBackslashBeforeNul", input: `"\\u0000"`, want: `\u0000`},
		{name: "EscapedBackslashBeforeSurrogate", input: `["\\ud800"]`, want: []any{`\ud800`}},
		// Schema vocabulary is ordinary data at this boundary.
		{name: "SchemaKeywordsAsData", input: `{"$ref":"#/x","$id":"s","enum":["$schema"]}`, want: map[string]any{"$ref": "#/x", "$id": "s", "enum": []any{"$schema"}}},
		// Rejections. Inputs containing "marker" prove error text never echoes
		// attacker-controlled content.
		{name: "Empty", input: ``, wantErr: chatstructured.ErrMalformed},
		{name: "WhitespaceOnly", input: " \n\t", wantErr: chatstructured.ErrMalformed},
		{name: "TrailingComma", input: `{"marker":1,}`, wantErr: chatstructured.ErrMalformed},
		{name: "RawControlCharacter", input: "\"a\tb\"", wantErr: chatstructured.ErrMalformed},
		{name: "RawNulByte", input: "\"a\x00b\"", wantErr: chatstructured.ErrMalformed},
		{name: "NaN", input: `NaN`, wantErr: chatstructured.ErrMalformed},
		{name: "NegativeInfinity", input: `-Infinity`, wantErr: chatstructured.ErrMalformed},
		{name: "DanglingExponent", input: `1e+`, wantErr: chatstructured.ErrMalformed},
		{name: "SecondValue", input: `{} {}`, wantErr: chatstructured.ErrTrailingData},
		{name: "TrailingGarbage", input: `["marker"] x`, wantErr: chatstructured.ErrTrailingData},
		// The tokenizer reads the leading 0 as a complete number.
		{name: "Hex", input: `0x10`, wantErr: chatstructured.ErrTrailingData},
		{name: "LeadingZero", input: `01`, wantErr: chatstructured.ErrTrailingData},
		{name: "InvalidUTF8InString", input: "\"\xff\"", wantErr: chatstructured.ErrInvalidUTF8},
		{name: "InvalidUTF8OutsideString", input: "[\xc3]", wantErr: chatstructured.ErrInvalidUTF8},
		{name: "DuplicateKey", input: `{"marker":1,"marker":2}`, wantErr: chatstructured.ErrDuplicateKey},
		{name: "NestedDuplicateKey", input: `{"x":[{"a":1,"b":2,"a":3}]}`, wantErr: chatstructured.ErrDuplicateKey},
		{name: "EscapeEquivalentDuplicateKey", input: `{"a":1,"\u0061":2}`, wantErr: chatstructured.ErrDuplicateKey},
		{name: "SurrogateEquivalentDuplicateKey", input: "{\"\\ud83d\\ude00\":1,\"\U0001F600\":2}", wantErr: chatstructured.ErrDuplicateKey},
		{name: "EscapedNul", input: `"\u0000"`, wantErr: chatstructured.ErrNullCharacter},
		{name: "EscapedNulInKey", input: `{"\u0000":1}`, wantErr: chatstructured.ErrNullCharacter},
		{name: "LoneHighSurrogate", input: `"marker\ud800"`, wantErr: chatstructured.ErrUnpairedSurrogate},
		{name: "LoneLowSurrogate", input: `"\udc00"`, wantErr: chatstructured.ErrUnpairedSurrogate},
		{name: "HighSurrogateThenOtherEscape", input: `["\ud800\u0041"]`, wantErr: chatstructured.ErrUnpairedSurrogate},
		{name: "ReversedSurrogates", input: `"\ude00\ud83d"`, wantErr: chatstructured.ErrUnpairedSurrogate},
		{name: "LoneSurrogateInKey", input: `{"\ud800":1}`, wantErr: chatstructured.ErrUnpairedSurrogate},
		// Just over the numeric caps, in both signs and nested positions.
		{name: "LiteralTooLong", input: strings.Repeat("9", 129), wantErr: chatstructured.ErrNumberTooLong},
		{name: "NegativeLiteralTooLong", input: "[-" + strings.Repeat("9", 128) + "]", wantErr: chatstructured.ErrNumberTooLong},
		{name: "ExponentTooLarge", input: `1e1025`, wantErr: chatstructured.ErrExponentTooLarge},
		{name: "NegativeExponentTooLarge", input: `{"a":-1.5E-1025}`, wantErr: chatstructured.ErrExponentTooLarge},
		{name: "ExponentLeadingZerosTooLarge", input: `1e+01025`, wantErr: chatstructured.ErrExponentTooLarge},
		// Exponents far beyond the int range are rejected without overflow.
		{name: "HugeExponent", input: "1e" + strings.Repeat("9", 100), wantErr: chatstructured.ErrExponentTooLarge},
		{name: "HugeNegativeExponent", input: "1e-" + strings.Repeat("9", 100), wantErr: chatstructured.ErrExponentTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			raw := []byte(tt.input)
			before := bytes.Clone(raw)
			got, err := chatstructured.ParseOutputValue(raw)
			require.Equal(t, before, raw, "input must not be mutated")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Nil(t, got)
				require.NotContains(t, err.Error(), "marker")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseLimits(t *testing.T) {
	t.Parallel()

	// Expected caps are stated independently of the implementation. A zero
	// node or array cap means only the byte cap bounds that dimension.
	profiles := []struct {
		name        string
		parse       func([]byte) (any, error)
		maxBytes    int
		maxDepth    int
		maxNodes    int
		maxArrayLen int
	}{
		{name: "SchemaDocument", parse: chatstructured.ParseSchemaDocument, maxBytes: 16384, maxDepth: 16},
		{name: "FinalizerArguments", parse: chatstructured.ParseFinalizerArguments, maxBytes: 81920, maxDepth: 33, maxNodes: 4097, maxArrayLen: 256},
		{name: "OutputValue", parse: chatstructured.ParseOutputValue, maxBytes: 65536, maxDepth: 32, maxNodes: 4096, maxArrayLen: 256},
	}
	for _, p := range profiles {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()

			accept := func(input string) {
				_, err := p.parse([]byte(input))
				require.NoError(t, err)
			}
			reject := func(input string, want error) {
				_, err := p.parse([]byte(input))
				require.ErrorIs(t, err, want)
			}

			str := func(n int) string { return `"` + strings.Repeat("a", n-2) + `"` }
			accept(str(p.maxBytes))
			reject(str(p.maxBytes+1), chatstructured.ErrTooLarge)
			// The byte cap is checked before anything else, even UTF-8.
			reject(strings.Repeat("\xff", p.maxBytes+1), chatstructured.ErrTooLarge)

			accept(nested(p.maxDepth))
			reject(nested(p.maxDepth+1), chatstructured.ErrTooDeep)
			// Depth counts every container kind; keys add nothing.
			deep := strings.Repeat(`{"a":`, p.maxDepth) + `1` + strings.Repeat("}", p.maxDepth)
			accept(deep)
			reject(`{"a":`+deep+`}`, chatstructured.ErrTooDeep)

			if p.maxNodes > 0 {
				// The object itself is one node; each member value is another.
				accept(object(p.maxNodes - 1))
				reject(object(p.maxNodes), chatstructured.ErrTooManyNodes)
			} else {
				accept(array(5000))
			}
			if p.maxArrayLen > 0 {
				accept(array(p.maxArrayLen))
				reject(array(p.maxArrayLen+1), chatstructured.ErrArrayTooLong)
				reject("[[],"+array(p.maxArrayLen+1)+"]", chatstructured.ErrArrayTooLong)
			}
		})
	}
}

func FuzzParseOutputValue(f *testing.F) {
	for _, seed := range []string{
		`{"a":[1,"x",true,null],"b":{"c":"\ud83d\ude00"}}`, `{"a":1,}`, `[1 2]`, ``, `{} {}`, `NaN`, `01`,
		`"\u0041\\\"\n\ud800"`, `"\udc00"`, `"\u0000"`, `{"a":1,"\u0061":2}`, "[\"\xff\"]", `-0.0e-0`,
		nested(32), nested(33), object(4095), array(257), `9007199254740993`, `1e1024`, `1e1025`, strings.Repeat("9", 129),
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		before := bytes.Clone(raw)
		got, err := chatstructured.ParseOutputValue(raw)
		require.Equal(t, before, raw, "input must not be mutated")
		if err != nil {
			return
		}
		// Accepted input is valid JSON within the byte cap that decodes to the
		// same value through encoding/json's own precision-preserving path.
		require.LessOrEqual(t, len(raw), 65536)
		require.True(t, json.Valid(raw))
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var want any
		require.NoError(t, dec.Decode(&want))
		require.Equal(t, want, got)
	})
}

// BenchmarkParseOutputValue measures inputs shaped to be the most expensive
// the output profile accepts, so the caps can be qualified natively.
func BenchmarkParseOutputValue(b *testing.B) {
	number := "1e" + strings.Repeat("0", 125) + "1" // 128 bytes, exponent 1.
	inputs := []struct {
		name  string
		input string
	}{
		{name: "MaxNodesObject", input: object(4095)},
		{name: "MaxBytesSurrogatePairs", input: `"` + strings.Repeat(`\ud83d\ude00`, (65536-2)/12) + `"`},
		{name: "MaxLengthNumbers", input: "[" + strings.Repeat(number+",", 255) + number + "]"},
	}
	for _, in := range inputs {
		b.Run(in.name, func(b *testing.B) {
			raw := []byte(in.input)
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			for b.Loop() {
				if _, err := chatstructured.ParseOutputValue(raw); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
