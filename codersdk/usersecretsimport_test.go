package codersdk_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

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
		`DQ_ESCAPED_QUOTE="a\"b"`,
		`DQ_EVEN_BACKSLASH_CLOSE="two backslashes\\"`,
		`DQ_CARRIAGE="a\rb"`,
		`DQ_UNKNOWN="x\zy"`,
		`SQUOTED='literal \n no escape'`,
		"EQ_IN_VALUE=a=b=c",
		"HASH=value # kept literal",
		"UNICODE=héllo 世界 café",
		"exportFOO=literal-key",
		"export=literal-export-key",
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
		{Name: "DQ_ESCAPED_QUOTE", EnvName: "DQ_ESCAPED_QUOTE", Value: `a"b`},
		{Name: "DQ_EVEN_BACKSLASH_CLOSE", EnvName: "DQ_EVEN_BACKSLASH_CLOSE", Value: `two backslashes\`},
		{Name: "DQ_CARRIAGE", EnvName: "DQ_CARRIAGE", Value: "a\rb"},
		{Name: "DQ_UNKNOWN", EnvName: "DQ_UNKNOWN", Value: `x\zy`},
		{Name: "SQUOTED", EnvName: "SQUOTED", Value: `literal \n no escape`},
		{Name: "EQ_IN_VALUE", EnvName: "EQ_IN_VALUE", Value: "a=b=c"},
		{Name: "HASH", EnvName: "HASH", Value: "value # kept literal"},
		{Name: "UNICODE", EnvName: "UNICODE", Value: "héllo 世界 café"},
		{Name: "exportFOO", EnvName: "exportFOO", Value: "literal-key"},
		{Name: "export", EnvName: "export", Value: "literal-export-key"},
		{Name: "EQ_ONLY_VALUE", EnvName: "EQ_ONLY_VALUE", Value: "="},
		{Name: "EMPTY_VAL", EnvName: "EMPTY_VAL", Value: ""},
		{Name: "TABBED", EnvName: "TABBED", Value: "tab trimmed"},
		{Name: "TAB_EXPORT", EnvName: "TAB_EXPORT", Value: "via-tab"},
	}
	require.Equal(t, want, reqs)
}

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
		errMsgs []string
	}{
		{name: "NoEquals", content: "OK=value\nNOEQUALS\n", errMsgs: []string{"no '='", "line 2"}},
		{name: "MissingKey", content: "=value", errMsgs: []string{"missing key"}},
		{name: "UnterminatedDouble", content: `KEY="oops`, errMsgs: []string{"missing closing double quote"}},
		{name: "EscapedDoubleQuoteNotClosing", content: `KEY="oops\"`, errMsgs: []string{"missing closing double quote"}},
		{name: "DoubleQuoteTrailingData", content: `KEY="ok" # comment`, errMsgs: []string{"unexpected data after closing double quote"}},
		{name: "UnterminatedSingle", content: `KEY='oops`, errMsgs: []string{"missing closing single quote"}},
		{name: "DuplicateKey", content: "DUP=a\nDUP=b", errMsgs: []string{"duplicate key", "line 2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatEnv, tt.content)
			require.Error(t, err)
			for _, msg := range tt.errMsgs {
				assert.Contains(t, err.Error(), msg)
			}
		})
	}
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
		{name: "InvalidTrailingJSON", content: `{"A":"1"} {`, errMsg: "invalid JSON"},
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
		{name: "BoolKey", content: "true: value", errMsg: "keys must be strings"},
		{name: "IntKey", content: "1: value", errMsg: "keys must be strings"},
		{name: "SequenceKey", content: "? [a, b]\n: value", errMsg: "keys must be strings"},
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

func TestParseSecretsFileYAMLAlias(t *testing.T) {
	t.Parallel()

	_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatYAML, "a: &a \"x\"\nb: *a\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

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

	t.Run("TrailingSeparatorRejected", func(t *testing.T) {
		t.Parallel()
		_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatYAML, "A: \"1\"\n---\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "single document")
	})
}

// FuzzParseSecretsFile checks two invariants: (1) the parser never panics
// regardless of input (the fuzz engine catches panics automatically); (2) on
// success the result is well-formed: at least one entry, at most
// MaxUserSecretsPerUserCount entries, EnvName == Name for every entry,
// and all keys unique. On error the returned slice must be nil/empty.
func FuzzParseSecretsFile(f *testing.F) {
	// env - valid
	f.Add("env", "KEY=value")
	f.Add("env", "export EXPORTED=val\nPLAIN=plain")
	f.Add("env", "\ufeffKEY1=val1\r\nKEY2=val2\r\n")
	f.Add("env", "EMPTY=\nKEY=val")
	f.Add("env", "EQ=a=b=c")
	f.Add("env", `DQUOTED="double quoted"`)
	f.Add("env", `SQUOTED='single quoted'`)
	f.Add("env", "# comment\nKEY=val")
	// env - malformed / tricky quoting
	f.Add("env", `KEY="unterminated`)
	f.Add("env", `KEY='unterminated`)
	f.Add("env", `KEY="escaped\"`)
	f.Add("env", `KEY="two backslashes\\"`)
	f.Add("env", "NOEQUALS")
	f.Add("env", "=value")
	f.Add("env", `KEY="ok" # trailing`)
	f.Add("env", "DUP=a\nDUP=b")
	// json - valid
	f.Add("json", `{"A":"1","B":"two"}`)
	// json - malformed
	f.Add("json", `{"A":`)
	f.Add("json", `["a","b"]`)
	f.Add("json", `"just a string"`)
	f.Add("json", `{"A":1}`)
	f.Add("json", `{"A":true}`)
	f.Add("json", `{"A":null}`)
	f.Add("json", `{"A":{"x":"y"}}`)
	f.Add("json", `{"A":["x"]}`)
	f.Add("json", `{"DUP":"a","DUP":"b"}`)
	f.Add("json", `{"A":"1"} {"B":"2"}`)
	// yaml - valid
	f.Add("yaml", "A: one\nB: \"two\"\n")
	// yaml - malformed
	f.Add("yaml", "A: [unclosed")
	f.Add("yaml", "- a\n- b\n")
	f.Add("yaml", "OUTER:\n  inner: x\n")
	f.Add("yaml", "PORT: 8080\n")
	f.Add("yaml", "FLAG: true\n")
	f.Add("yaml", "a: &a \"x\"\nb: *a\n")
	f.Add("yaml", "A: \"1\"\n---\nB: \"2\"\n")
	// unknown / empty format
	f.Add("", "KEY=value")
	f.Add("toml", "KEY=value")

	f.Fuzz(func(t *testing.T, format string, content string) {
		reqs, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormat(format), content)
		if err != nil {
			require.Empty(t, reqs)
			return
		}

		// On success: at least one entry, within the per-user cap.
		require.NotEmpty(t, reqs)
		require.LessOrEqual(t, len(reqs), codersdk.MaxUserSecretsPerUserCount)

		seen := make(map[string]struct{}, len(reqs))
		for _, req := range reqs {
			require.Equal(t, req.Name, req.EnvName)
			_, dup := seen[req.Name]
			require.False(t, dup, "duplicate key %q in result", req.Name)
			seen[req.Name] = struct{}{}
		}
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

	t.Run("MaxBytesBoundary", func(t *testing.T) {
		t.Parallel()
		value := strings.Repeat("a", codersdk.MaxSecretsFileBytes-len("KEY="))
		reqs, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatEnv, "KEY="+value)
		require.NoError(t, err)
		require.Equal(t, []codersdk.CreateUserSecretRequest{
			{Name: "KEY", EnvName: "KEY", Value: value},
		}, reqs)
	})

	t.Run("Oversized", func(t *testing.T) {
		t.Parallel()
		content := strings.Repeat("a", codersdk.MaxSecretsFileBytes+1)
		_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatEnv, content)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maximum allowed size")
	})

	t.Run("TooManySecrets", func(t *testing.T) {
		t.Parallel()
		lines := make([]string, 0, codersdk.MaxUserSecretsPerUserCount+1)
		for i := 0; i < codersdk.MaxUserSecretsPerUserCount+1; i++ {
			lines = append(lines, fmt.Sprintf("KEY_%d=value", i))
		}
		_, err := codersdk.ParseSecretsFile(codersdk.SecretsFileFormatEnv, strings.Join(lines, "\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds the maximum")
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
