package codersdk_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// TestParseSecretsFileEnv covers the dotenv parsing rules end-to-end:
// comments, blank lines, the export prefix (with a space or a tab),
// single and double quotes, double-quote escapes, '=' inside a value,
// surrounding whitespace and tabs, an inline '#' kept literally,
// non-ASCII values, "export" as part of a key name (not the prefix),
// and a value that is exactly '='. It also asserts the flat mapping
// invariant Name == EnvName == KEY and Value == VALUE.
func TestParseSecretsFileEnv(t *testing.T) {
	t.Parallel()

	content := strings.Join([]string{
		"# full-line comment",
		"   # indented full-line comment",
		"",
		"   ",
		"export EXPORTED=exported-value",
		"PLAIN=plain-value",
		"WITH_SPACES=  trimmed  ",
		`DQUOTED="double quoted"`,
		`DQ_ESCAPES="a\nb\tc\\d\"e"`,
		`SQUOTED='literal \n no escape'`,
		"EQ_IN_VALUE=a=b=c",
		"HASH=value # kept literal",
		"UNICODE=héllo 世界 café",
		"exportFOO=literal-key",
		"EQ_ONLY_VALUE==",
		"EMPTY_VAL=",
		"TABBED=\t tab trimmed \t",
		"export\tTAB_EXPORT=via-tab",
	}, "\n")

	reqs, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatEnv, content)
	require.NoError(t, err)

	want := []codersdk.CreateUserSecretRequest{
		{Name: "EXPORTED", EnvName: "EXPORTED", Value: "exported-value"},
		{Name: "PLAIN", EnvName: "PLAIN", Value: "plain-value"},
		{Name: "WITH_SPACES", EnvName: "WITH_SPACES", Value: "trimmed"},
		{Name: "DQUOTED", EnvName: "DQUOTED", Value: "double quoted"},
		{Name: "DQ_ESCAPES", EnvName: "DQ_ESCAPES", Value: "a\nb\tc\\d\"e"},
		{Name: "SQUOTED", EnvName: "SQUOTED", Value: `literal \n no escape`},
		{Name: "EQ_IN_VALUE", EnvName: "EQ_IN_VALUE", Value: "a=b=c"},
		{Name: "HASH", EnvName: "HASH", Value: "value # kept literal"},
		{Name: "UNICODE", EnvName: "UNICODE", Value: "héllo 世界 café"},
		{Name: "exportFOO", EnvName: "exportFOO", Value: "literal-key"},
		{Name: "EQ_ONLY_VALUE", EnvName: "EQ_ONLY_VALUE", Value: "="},
		{Name: "EMPTY_VAL", EnvName: "EMPTY_VAL", Value: ""},
		{Name: "TABBED", EnvName: "TABBED", Value: "tab trimmed"},
		{Name: "TAB_EXPORT", EnvName: "TAB_EXPORT", Value: "via-tab"},
	}
	require.Equal(t, want, reqs)
}

// TestParseSecretsFileEnvCRLFAndBOM verifies CRLF normalization and BOM
// stripping.
func TestParseSecretsFileEnvCRLFAndBOM(t *testing.T) {
	t.Parallel()

	content := "\ufeffKEY1=val1\r\nKEY2=val2\r\n"
	reqs, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatEnv, content)
	require.NoError(t, err)
	require.Equal(t, []codersdk.CreateUserSecretRequest{
		{Name: "KEY1", EnvName: "KEY1", Value: "val1"},
		{Name: "KEY2", EnvName: "KEY2", Value: "val2"},
	}, reqs)
}

func TestParseSecretsFileEnvErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		errMsg  string
	}{
		{name: "NoEquals", content: "NOEQUALS", errMsg: "no '='"},
		{name: "MissingKey", content: "=value", errMsg: "missing key"},
		{name: "UnterminatedDouble", content: `KEY="oops`, errMsg: "missing closing double quote"},
		{name: "UnterminatedSingle", content: `KEY='oops`, errMsg: "missing closing single quote"},
		{name: "DuplicateKey", content: "DUP=a\nDUP=b", errMsg: "duplicate key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatEnv, tt.content)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

// TestParseSecretsFileEnvDuplicateCitesLine confirms the duplicate-key
// error reports the offending line for the env format.
func TestParseSecretsFileEnvDuplicateCitesLine(t *testing.T) {
	t.Parallel()

	_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatEnv, "DUP=a\nDUP=b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key")
	assert.Contains(t, err.Error(), "line 2")
}

// TestParseSecretsFileEnvMissingEqualsCitesLine confirms the missing
// '=' error reports the offending line for the env format, not just
// line 1.
func TestParseSecretsFileEnvMissingEqualsCitesLine(t *testing.T) {
	t.Parallel()

	_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatEnv, "OK=value\nNOEQUALS\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no '='")
	assert.Contains(t, err.Error(), "line 2")
}

func TestParseSecretsFileJSON(t *testing.T) {
	t.Parallel()

	reqs, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatJSON, `{"A":"1","B":"two","C":"a=b#c"}`)
	require.NoError(t, err)
	require.Equal(t, []codersdk.CreateUserSecretRequest{
		{Name: "A", EnvName: "A", Value: "1"},
		{Name: "B", EnvName: "B", Value: "two"},
		{Name: "C", EnvName: "C", Value: "a=b#c"},
	}, reqs)
}

func TestParseSecretsFileJSONErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		errMsg  string
	}{
		{name: "Malformed", content: `{"A":`, errMsg: "invalid JSON"},
		{name: "NonObjectArray", content: `["a","b"]`, errMsg: "must be an object"},
		{name: "NonObjectScalar", content: `"just a string"`, errMsg: "must be an object"},
		{name: "NumberValue", content: `{"A":1}`, errMsg: "must be a string"},
		{name: "BoolValue", content: `{"A":true}`, errMsg: "must be a string"},
		{name: "NullValue", content: `{"A":null}`, errMsg: "must be a string"},
		{name: "NestedObject", content: `{"A":{"x":"y"}}`, errMsg: "nested object or array"},
		{name: "NestedArray", content: `{"A":["x"]}`, errMsg: "nested object or array"},
		{name: "DuplicateKey", content: `{"DUP":"a","DUP":"b"}`, errMsg: "duplicate key"},
		{name: "TrailingData", content: `{"A":"1"} {"B":"2"}`, errMsg: "trailing data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatJSON, tt.content)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestParseSecretsFileYAML(t *testing.T) {
	t.Parallel()

	content := strings.Join([]string{
		"# a comment",
		"A: one",
		`B: "two"`,
		"C: 'a=b#c'",
	}, "\n")
	reqs, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatYAML, content)
	require.NoError(t, err)
	require.Equal(t, []codersdk.CreateUserSecretRequest{
		{Name: "A", EnvName: "A", Value: "one"},
		{Name: "B", EnvName: "B", Value: "two"},
		{Name: "C", EnvName: "C", Value: "a=b#c"},
	}, reqs)
}

func TestParseSecretsFileYAMLErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		errMsg  string
	}{
		{name: "Malformed", content: "A: [unclosed", errMsg: "invalid YAML"},
		{name: "NonMappingScalar", content: "just a scalar", errMsg: "must be a mapping"},
		{name: "NonMappingSequence", content: "- a\n- b", errMsg: "must be a mapping"},
		{name: "NestedMapping", content: "OUTER:\n  inner: x", errMsg: "nested mapping or sequence"},
		{name: "SequenceValue", content: "LIST:\n  - a\n  - b", errMsg: "nested mapping or sequence"},
		{name: "IntValue", content: "PORT: 8080", errMsg: "must be a string"},
		{name: "BoolValue", content: "FLAG: true", errMsg: "must be a string"},
		{name: "NullValue", content: "KEY: null", errMsg: "must be a string"},
		{name: "DuplicateKey", content: "DUP: a\nDUP: b", errMsg: "duplicate key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatYAML, tt.content)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

// TestParseSecretsFileYAMLAliasBomb is a regression guard against
// YAML alias-expansion ("billion laughs") resource exhaustion. Two
// properties keep an alias bomb cheap: yaml.v3 decodes into a
// yaml.Node without resolving aliases (so nothing expands in memory),
// and the parser only accepts scalar string values, so any sequence,
// mapping, or alias node at the top level is rejected outright. The
// inputs below stay well under MaxSecretsFileBytes yet would expand
// to an enormous structure if aliases were ever resolved; each must
// return a parse error quickly rather than hang or exhaust memory.
func TestParseSecretsFileYAMLAliasBomb(t *testing.T) {
	t.Parallel()

	// Classic nested alias bomb: each anchor references the previous one
	// nine times, so resolving the last alias would expand to 9^9 nodes.
	var bomb strings.Builder
	_, _ = bomb.WriteString("a: &a \"lol\"\n")
	prev := "a"
	for i := 0; i < 9; i++ {
		cur := fmt.Sprintf("l%d", i)
		_, _ = bomb.WriteString(cur + ": &" + cur + " [")
		for j := 0; j < 9; j++ {
			if j > 0 {
				_ = bomb.WriteByte(',')
			}
			_, _ = bomb.WriteString("*" + prev)
		}
		_, _ = bomb.WriteString("]\n")
		prev = cur
	}

	cases := []struct {
		name    string
		content string
	}{
		{name: "NestedSequences", content: bomb.String()},
		// Top-level value is an alias node (not a scalar), which must be
		// rejected even though the anchor it points at is a scalar.
		{name: "AliasToScalar", content: "a: &a \"x\"\nb: *a\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Less(t, len(tc.content), codersdk.MaxSecretsFileBytes)
			_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatYAML, tc.content)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must be a string")
		})
	}
}

// TestParseSecretsFileYAMLMultiDocument verifies that a multi-document
// YAML stream is rejected rather than silently importing only the first
// document and dropping the rest. A bare trailing "---" separator with
// no content is harmless and must still parse.
func TestParseSecretsFileYAMLMultiDocument(t *testing.T) {
	t.Parallel()

	t.Run("SecondMappingRejected", func(t *testing.T) {
		t.Parallel()
		_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatYAML, "A: \"1\"\n---\nB: \"2\"\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "single document")
	})

	t.Run("SecondScalarRejected", func(t *testing.T) {
		t.Parallel()
		_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatYAML, "A: \"1\"\n---\nplain\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "single document")
	})

	t.Run("TrailingSeparatorAllowed", func(t *testing.T) {
		t.Parallel()
		reqs, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatYAML, "A: \"1\"\n---\n")
		require.NoError(t, err)
		require.Equal(t, []codersdk.CreateUserSecretRequest{
			{Name: "A", EnvName: "A", Value: "1"},
		}, reqs)
	})
}

func TestParseSecretsFileGeneralErrors(t *testing.T) {
	t.Parallel()

	t.Run("UnknownFormat", func(t *testing.T) {
		t.Parallel()
		_, err := codersdk.ParseSecretsFile("toml", "A=1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown secrets file format")
	})

	t.Run("EmptyFormat", func(t *testing.T) {
		t.Parallel()
		_, err := codersdk.ParseSecretsFile("", "A=1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "format is required")
	})

	t.Run("Oversized", func(t *testing.T) {
		t.Parallel()
		content := strings.Repeat("a", codersdk.MaxSecretsFileBytes+1)
		_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatEnv, content)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maximum allowed size")
	})

	emptyCases := []struct {
		name    string
		format  codersdk.SecretsFileFormat
		content string
	}{
		{name: "EnvEmpty", format: codersdk.SecretsFileFormatEnv, content: ""},
		{name: "EnvWhitespace", format: codersdk.SecretsFileFormatEnv, content: "   \n\t\n"},
		{name: "EnvAllComments", format: codersdk.SecretsFileFormatEnv, content: "# one\n# two\n"},
		{name: "JSONEmptyObject", format: codersdk.SecretsFileFormatJSON, content: "{}"},
		{name: "YAMLEmpty", format: codersdk.SecretsFileFormatYAML, content: ""},
		{name: "YAMLCommentsOnly", format: codersdk.SecretsFileFormatYAML, content: "# nothing here\n"},
	}
	for _, tt := range emptyCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := codersdk.ParseSecretsFile(tt.format, tt.content)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no secrets found")
		})
	}
}

// TestParseSecretsFileMappingEquivalence asserts the documented flat
// mapping (Name == EnvName == KEY, FilePath empty) holds for every
// format, which is what makes a single duplicate-KEY check cover
// duplicate names, env_names, and file_paths at once.
func TestParseSecretsFileMappingEquivalence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		format  codersdk.SecretsFileFormat
		content string
	}{
		{codersdk.SecretsFileFormatEnv, "FOO=bar"},
		{codersdk.SecretsFileFormatJSON, `{"FOO":"bar"}`},
		{codersdk.SecretsFileFormatYAML, "FOO: bar"},
	}
	for _, tc := range cases {
		reqs, err := codersdk.ParseSecretsFile(tc.format, tc.content)
		require.NoErrorf(t, err, "format %s", tc.format)
		require.Lenf(t, reqs, 1, "format %s", tc.format)
		got := reqs[0]
		assert.Equal(t, "FOO", got.Name)
		assert.Equal(t, "FOO", got.EnvName)
		assert.Equal(t, "bar", got.Value)
		assert.Empty(t, got.FilePath)
		assert.Empty(t, got.Description)
	}
}
