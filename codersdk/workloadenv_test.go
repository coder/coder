package codersdk_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestWorkloadEnvValidation(t *testing.T) {
	t.Parallel()
	variables := func(count int) map[string]string {
		env := make(map[string]string, count)
		for i := range count {
			env[fmt.Sprintf("KEY_%d", i)] = "private-value"
		}
		return env
	}
	for _, tc := range []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{name: "Omitted"},
		{name: "Empty", env: map[string]string{}},
		{name: "ExactValues", env: map[string]string{
			"EMPTY": "", "_": "private-value", "Mixed_Case123": "\nspaces = a=b,c\n\t雪",
		}},
		{name: "MaxCount", env: variables(256)},
		{name: "TooMany", env: variables(257), wantErr: "env must contain at most 256 variables"},
		{name: "MaxBytes", env: map[string]string{"KEY": strings.Repeat("v", (64<<10)-3)}},
		{name: "TooManyBytes", env: map[string]string{"KEY": strings.Repeat("v", (64<<10)-2)}, wantErr: "env keys and values must total at most 64KiB"},
		{name: "TotalBytes", env: map[string]string{"A": strings.Repeat("v", 32<<10), "B": strings.Repeat("v", 32<<10)}, wantErr: "env keys and values must total at most 64KiB"},
		{name: "MaxKeyBytes", env: map[string]string{strings.Repeat("K", 64<<10): ""}},
		{name: "TooManyKeyBytes", env: map[string]string{strings.Repeat("K", (64<<10)+1): ""}, wantErr: "env keys and values must total at most 64KiB"},
		{name: "UTF8Bytes", env: map[string]string{"A": strings.Repeat("é", 32<<10)}, wantErr: "env keys and values must total at most 64KiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := codersdk.CreateWorkloadRequest{
				ID: uuid.New(), TemplateID: uuid.New(), CPU: 1, MemoryBytes: 16 << 20, Command: "true", Env: tc.env,
			}
			if tc.wantErr == "" {
				require.NoError(t, req.Validate())
			} else {
				require.EqualError(t, req.Validate(), tc.wantErr)
			}
		})
	}
	for _, tc := range []struct {
		name  string
		key   string
		value string
	}{
		{name: "EmptyKey", value: "private-value"},
		{name: "LeadingDigit", key: "1KEY", value: "private-value"},
		{name: "EqualsKey", key: "KEY=secret", value: "private-value"},
		{name: "HyphenKey", key: "KEY-NAME", value: "private-value"},
		{name: "SpaceKey", key: "KEY NAME", value: "private-value"},
		{name: "NewlineKey", key: "KEY\n", value: "private-value"},
		{name: "NULKey", key: "KEY\x00", value: "private-value"},
		{name: "UnicodeKey", key: "KÉY", value: "private-value"},
		{name: "NULValue", key: "KEY", value: "private-value\x00suffix"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := codersdk.CreateWorkloadRequest{
				ID: uuid.New(), TemplateID: uuid.New(), CPU: 1, MemoryBytes: 16 << 20, Command: "true",
				Env: map[string]string{tc.key: tc.value},
			}
			require.EqualError(t, req.Validate(), "env keys must be portable variable names and values must contain no NUL")
		})
	}
}
