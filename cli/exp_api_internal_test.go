package cli

import (
	"net/url"
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

func TestParseAPIFieldKey(t *testing.T) {
	t.Parallel()
	for key, want := range map[string][]string{
		"name":            {"name"},
		"tags[]":          {"tags", ""},
		"meta[a][b]":      {"meta", "a", "b"},
		"meta[a][list][]": {"meta", "a", "list", ""},
	} {
		got, err := parseAPIFieldKey(key)
		require.NoError(t, err, key)
		require.Equal(t, want, got, key)
	}
	for _, key := range []string{"", "[a]", "a[b", "a[b]c", "a[][b]"} {
		_, err := parseAPIFieldKey(key)
		require.Error(t, err, key)
	}
}
