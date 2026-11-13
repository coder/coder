package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSignArgs(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		args      []string
		namespace string
		payload   string
		ok        bool
	}{
		{"LiteralKey", []string{"-Y", "sign", "-n", "git", "-f", "/tmp/key.pub", "-U", "/tmp/buf"}, "git", "/tmp/buf", true},
		{"KeyFile", []string{"-Y", "sign", "-n", "git", "-f", "/tmp/key", "/tmp/buf"}, "git", "/tmp/buf", true},
		{"Verify", []string{"-Y", "verify", "-n", "git", "-f", "allowed", "-I", "x", "-s", "sig"}, "", "", false},
		{"FindPrincipals", []string{"-Y", "find-principals", "-f", "allowed", "-s", "sig"}, "", "", false},
		{"NoNamespace", []string{"-Y", "sign", "-f", "/tmp/key", "/tmp/buf"}, "", "", false},
		{"NoPayload", []string{"-Y", "sign", "-n", "git", "-f", "/tmp/key"}, "", "", false},
		{"TruncatedFlag", []string{"-Y", "sign", "-n"}, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			namespace, payload, ok := parseSignArgs(tc.args)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.namespace, namespace)
			require.Equal(t, tc.payload, payload)
		})
	}
}

func TestGitSignArgs(t *testing.T) {
	t.Parallel()

	require.Equal(t,
		[]string{"coder", "gitsign-ssh", "--", "-Y", "sign", "-n", "git", "buf"},
		gitSignArgs([]string{"coder", "-Y", "sign", "-n", "git", "buf"}),
	)
	require.Equal(t, []string{"coder", "ssh", "-Y"}, gitSignArgs([]string{"coder", "ssh", "-Y"}))
	require.Equal(t, []string{"coder"}, gitSignArgs([]string{"coder"}))
}
