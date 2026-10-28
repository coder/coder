package cli

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIURLIsInsecure(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]bool{
		"https://coder.example.com": false,
		"http://coder.example.com":  true,
		"http://10.0.0.5:3000":      true,
		"http://localhost:3000":     false,
		"http://127.0.0.1:3000":     false,
		"http://[::1]:3000":         false,
	} {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		require.Equal(t, want, apiURLIsInsecure(u), raw)
	}
}

func TestSplitAPIArgs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		wantItems  []string
	}{
		{"PathOnly", []string{"users"}, "", "users", []string{}},
		{"LowercaseMethod", []string{"patch", "users/me", "a=b"}, http.MethodPatch, "users/me", []string{"a=b"}},
		{"EveryMethod", []string{"OPTIONS", "x"}, http.MethodOptions, "x", []string{}},
		{"PathThatIsNotAMethod", []string{"deleted", "a==b"}, "", "deleted", []string{"a==b"}},
		{"MethodWithoutPath", []string{"get"}, http.MethodGet, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			method, path, items := splitAPIArgs(tc.args)
			require.Equal(t, tc.wantMethod, method)
			require.Equal(t, tc.wantPath, path)
			require.Equal(t, tc.wantItems, items)
		})
	}
}

func TestSplitAPIRequestItem(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		arg       string
		wantKey   string
		wantSep   string
		wantValue string
	}{
		{"X-Custom:value", "X-Custom", ":", "value"},
		{"Referer:http://example.com/a=b", "Referer", ":", "http://example.com/a=b"},
		{"q==owner:me", "q", "==", "owner:me"},
		{"name=value", "name", "=", "value"},
		{"email=a=b@example.com", "email", "=", "a=b@example.com"},
		{"count:=5", "count", ":=", "5"},
		{"script=@file.sh", "script", "=@", "file.sh"},
		{"meta:=@file.json", "meta", ":=@", "file.json"},
		{"empty=", "empty", "=", ""},
		// The first matching position wins over a longer separator later.
		{"a=b:=c", "a", "=", "b:=c"},
		{"a:b=c", "a", ":", "b=c"},
		// Escaped separator characters belong to the key.
		{`foo\==bar`, "foo=", "=", "bar"},
		{`foo\:bar:=1`, "foo:bar", ":=", "1"},
		{`a\=\=b==c`, "a==b", "==", "c"},
		{`at\@=x`, "at@", "=", "x"},
		// Escapes also apply in values, and a backslash before any other
		// character is kept.
		{`k=a\=b`, "k", "=", "a=b"},
		{`path\x=y`, `path\x`, "=", "y"},
	} {
		t.Run(tc.arg, func(t *testing.T) {
			t.Parallel()
			key, sep, value, ok := splitAPIRequestItem(tc.arg)
			require.True(t, ok)
			require.Equal(t, tc.wantKey, key)
			require.Equal(t, tc.wantSep, sep)
			require.Equal(t, tc.wantValue, value)
		})
	}
	for _, arg := range []string{"novalue", `escaped\=only`, ""} {
		_, _, _, ok := splitAPIRequestItem(arg)
		require.False(t, ok, arg)
	}
}

func TestParseAPIRequestItems(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	textFile := filepath.Join(dir, "script.sh")
	require.NoError(t, os.WriteFile(textFile, []byte("echo hi\r\n"), 0o600))
	jsonFile := filepath.Join(dir, "meta.json")
	require.NoError(t, os.WriteFile(jsonFile, []byte(`{"a": [1, 2]}`+"\n"), 0o600))
	badJSONFile := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(badJSONFile, []byte(`{"a":`), 0o600))

	for _, tc := range []struct {
		name        string
		items       []string
		stdin       string
		wantHeaders http.Header
		wantQuery   url.Values
		wantBody    string
		wantErr     string
	}{
		{
			name:        "Headers",
			items:       []string{"X-One: a ", "X-One:b", "content-type:text/plain"},
			wantHeaders: http.Header{"X-One": {"a", "b"}, "Content-Type": {"text/plain"}},
		},
		{
			name:      "QueryAllowsRepeats",
			items:     []string{"q==owner:me", "q==status:running", "limit==5"},
			wantQuery: url.Values{"q": {"owner:me", "status:running"}, "limit": {"5"}},
		},
		{
			name: "DataFields",
			items: []string{
				"name=alice", "count:=5", "enabled:=true", "nothing:=null",
				`tags:=["a",2]`, `meta:={"owner":{"name":"bob"}}`, "num=5",
			},
			wantBody: `{"name":"alice","count":5,"enabled":true,"nothing":null,
				"tags":["a",2],"meta":{"owner":{"name":"bob"}},"num":"5"}`,
		},
		{
			name:     "EscapedKey",
			items:    []string{`foo\==bar`},
			wantBody: `{"foo=":"bar"}`,
		},
		{
			name:     "Files",
			items:    []string{"script=@" + textFile, "meta:=@" + jsonFile},
			wantBody: `{"script":"echo hi\r\n","meta":{"a":[1,2]}}`,
		},
		{
			name:     "StringFromStdin",
			items:    []string{"content=@-"},
			stdin:    "line1\nline2\n",
			wantBody: `{"content":"line1\nline2\n"}`,
		},
		{
			name:     "JSONFromStdin",
			items:    []string{"meta:=@-"},
			stdin:    `{"x":true}`,
			wantBody: `{"meta":{"x":true}}`,
		},
		{name: "InvalidJSON", items: []string{"count:=five"}, wantErr: "not valid JSON"},
		{name: "InvalidJSONFile", items: []string{"meta:=@" + badJSONFile}, wantErr: "not valid JSON"},
		{name: "MissingFile", items: []string{"a=@" + filepath.Join(dir, "missing")}, wantErr: "read file"},
		{name: "DuplicateDataKey", items: []string{"a=1", "a:=2"}, wantErr: `duplicate data field "a"`},
		{name: "NoSeparator", items: []string{"novalue"}, wantErr: "invalid request item"},
		{name: "EmptyKey", items: []string{"=value"}, wantErr: "missing name"},
		{name: "TwoStdinItems", items: []string{"a=@-", "b:=@-"}, stdin: "1", wantErr: "only one request item can read stdin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, readStdin, err := parseAPIRequestItems(tc.items, strings.NewReader(tc.stdin))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.stdin != "", readStdin)
			if tc.wantHeaders == nil {
				tc.wantHeaders = http.Header{}
			}
			if tc.wantQuery == nil {
				tc.wantQuery = url.Values{}
			}
			require.Equal(t, tc.wantHeaders, got.headers)
			require.Equal(t, tc.wantQuery, got.query)
			if tc.wantBody == "" {
				require.Empty(t, got.data)
				return
			}
			body, err := json.Marshal(got.data)
			require.NoError(t, err)
			require.JSONEq(t, tc.wantBody, string(body))
		})
	}
}
